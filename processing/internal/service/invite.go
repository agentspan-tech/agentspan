package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/agentspan/processing/internal/crypto"
	"github.com/agentspan/processing/internal/db"
	"github.com/agentspan/processing/internal/email"
	"github.com/agentspan/processing/internal/txutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InviteResult is returned by InviteService.CreateInvite.
type InviteResult struct {
	InviteID  uuid.UUID `json:"invite_id"`
	InviteURL string    `json:"invite_url,omitempty"`
}

// AcceptInviteResult is returned by InviteService.AcceptInvite.
type AcceptInviteResult struct {
	OrganizationID uuid.UUID `json:"organization_id"`
}

// InviteService handles invite creation, acceptance, listing, and revocation.
type InviteService struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	mailer  email.Mailer
}

// NewInviteService creates a new InviteService.
func NewInviteService(queries *db.Queries, pool *pgxpool.Pool, mailer email.Mailer) *InviteService {
	return &InviteService{
		queries: queries,
		pool:    pool,
		mailer:  mailer,
	}
}

// CreateInvite creates or re-issues an invite for the given email address.
//
// Free plan enforcement (ORG-12): returns 403 if orgPlan == "free".
// Invite limits (ORG-05): max 50 pending invites per org.
// Re-send: if a pending invite already exists for the same email+org, the old invite is deleted and a new one is issued.
func (s *InviteService) CreateInvite(ctx context.Context, orgID, inviterID uuid.UUID, emailAddr, role, orgName, inviterName, orgPlan, locale string) (*InviteResult, error) {
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))

	// Validate email format.
	if _, err := mail.ParseAddress(emailAddr); err != nil {
		return nil, &ServiceError{Code: "invalid_email", Status: 400, Message: "Invalid email address"}
	}

	// Free plan enforcement.
	if orgPlan == "free" {
		return nil, &ServiceError{Code: "free_plan_no_invites", Status: 403, Message: "Free plan does not support team invites"}
	}

	// Validate role.
	validRoles := map[string]bool{"admin": true, "member": true, "viewer": true}
	if !validRoles[role] {
		return nil, &ServiceError{Code: "invalid_role", Status: 400, Message: "Role must be one of: admin, member, viewer"}
	}

	// Silently check if invitee is already a member. Use a generic error message
	// to prevent email enumeration — the caller cannot distinguish "user doesn't exist"
	// from "user exists but isn't a member".
	invitee, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err == nil {
		_, memberErr := s.queries.GetMembership(ctx, db.GetMembershipParams{
			OrganizationID: orgID,
			UserID:         invitee.ID,
		})
		if memberErr == nil {
			return nil, &ServiceError{Code: "invite_not_possible", Status: 409, Message: "Cannot invite this email address"}
		}
	}

	// Delete old invite (if re-send) + check limit + create new invite — atomically.
	raw, hash, err := crypto.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate invite token: %w", err)
	}

	var invite db.Invite
	err = txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		pendingCount, err := q.CountPendingInvitesByOrg(ctx, orgID)
		if err != nil {
			return fmt.Errorf("count pending invites: %w", err)
		}

		// Check for existing invite for same email+org (re-send flow).
		existing, err := q.GetInviteByEmailAndOrg(ctx, db.GetInviteByEmailAndOrgParams{
			Email:          &emailAddr,
			OrganizationID: orgID,
		})
		if err == nil {
			if delErr := q.DeleteInvite(ctx, db.DeleteInviteParams{
				ID:             existing.ID,
				OrganizationID: orgID,
			}); delErr != nil {
				return fmt.Errorf("delete old invite: %w", delErr)
			}
			pendingCount--
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing invite: %w", err)
		}

		if pendingCount >= 50 {
			return &ServiceError{Code: "invite_limit_reached", Status: 409, Message: "Maximum pending invites (50) reached for this organization"}
		}

		invite, err = q.CreateInvite(ctx, db.CreateInviteParams{
			OrganizationID: orgID,
			InvitedBy:      inviterID,
			Email:          &emailAddr,
			TokenHash:      hash,
			Role:           role,
			ExpiresAt:      time.Now().Add(7 * 24 * time.Hour),
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	// Send invite email (or generate link if SMTP is disabled).
	inviteURL, err := s.mailer.SendInvite(emailAddr, orgName, inviterName, raw, role, locale)
	if err != nil {
		// Log but don't fail — invite record is already created.
		inviteURL = ""
	}

	result := &InviteResult{
		InviteID: invite.ID,
	}
	if !s.mailer.IsSMTP() {
		result.InviteURL = inviteURL
	}
	return result, nil
}

// AcceptInvite validates a token and creates the membership for the authenticated user.
// The user's email must match the invite's target email (prevents token hijacking).
// Uses a transaction to prevent race conditions with concurrent accept requests.
func (s *InviteService) AcceptInvite(ctx context.Context, token string, userID uuid.UUID, userEmail string) (*AcceptInviteResult, error) {
	hash, err := crypto.HashToken(token)
	if err != nil {
		return nil, &ServiceError{Code: "invalid_token", Status: 400, Message: "Invalid invite token"}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept invite: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is no-op after commit

	txQueries := s.queries.WithTx(tx)

	invite, err := txQueries.GetInviteByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ServiceError{Code: "invalid_token", Status: 400, Message: "Invite token not found or expired"}
		}
		return nil, fmt.Errorf("get invite: %w", err)
	}

	// Verify the accepting user's email matches the invite target (case-insensitive).
	if invite.Email != nil && strings.ToLower(*invite.Email) != strings.ToLower(userEmail) {
		return nil, &ServiceError{Code: "email_mismatch", Status: 403, Message: "This invite was sent to a different email address"}
	}

	// Check user is not already a member.
	_, memberErr := txQueries.GetMembership(ctx, db.GetMembershipParams{
		OrganizationID: invite.OrganizationID,
		UserID:         userID,
	})
	if memberErr == nil {
		return nil, &ServiceError{Code: "already_member", Status: 409, Message: "You are already a member of this organization"}
	}

	// Create membership — handle unique constraint violation as 409.
	_, err = txQueries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: invite.OrganizationID,
		UserID:         userID,
		Role:           invite.Role,
	})
	if err != nil {
		if isDuplicateError(err) {
			return nil, &ServiceError{Code: "already_member", Status: 409, Message: "You are already a member of this organization"}
		}
		return nil, fmt.Errorf("create membership: %w", err)
	}

	// Mark invite as accepted.
	if err := txQueries.AcceptInvite(ctx, invite.ID); err != nil {
		return nil, fmt.Errorf("accept invite: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("accept invite: commit: %w", err)
	}

	return &AcceptInviteResult{OrganizationID: invite.OrganizationID}, nil
}

// ListPendingInvites returns all pending (unaccepted, non-expired) invites for an org.
func (s *InviteService) ListPendingInvites(ctx context.Context, orgID uuid.UUID) ([]db.Invite, error) {
	invites, err := s.queries.ListPendingInvitesByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return invites, nil
}

// RevokeInvite deletes a pending invite.
func (s *InviteService) RevokeInvite(ctx context.Context, orgID, inviteID uuid.UUID) error {
	if err := s.queries.DeleteInvite(ctx, db.DeleteInviteParams{
		ID:             inviteID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	return nil
}
