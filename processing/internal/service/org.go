package service

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
	"github.com/agentorbit-tech/agentorbit/processing/internal/txutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrgService handles organization lifecycle, membership management, and related operations.
type OrgService struct {
	queries        *db.Queries
	pool           *pgxpool.Pool
	mailer         email.Mailer
	deploymentMode string // "cloud", "self_host"
}

// NewOrgService creates a new OrgService.
func NewOrgService(queries *db.Queries, pool *pgxpool.Pool, mailer email.Mailer, deploymentMode string) *OrgService {
	return &OrgService{
		queries:        queries,
		pool:           pool,
		mailer:         mailer,
		deploymentMode: deploymentMode,
	}
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// generateSlug converts an org name to a URL-safe slug.
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = nonAlphanumRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "org"
	}
	return slug
}

// CreateOrganization creates a new organization with the given user as owner.
// If the generated slug collides, a 4-char random suffix is appended.
func (s *OrgService) CreateOrganization(ctx context.Context, userID uuid.UUID, name string) (*db.Organization, error) {
	if err := validateOrgName(name); err != nil {
		return nil, err
	}

	var org db.Organization
	err := txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		created, err := createOrganizationWithOwnerTx(ctx, q, name, s.deploymentMode, userID)
		if err != nil {
			return err
		}
		org = *created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// validateOrgName checks that the organization name is non-empty and within length limits.
func validateOrgName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ServiceError{Code: "invalid_org_name", Status: 400, Message: "Organization name is required"}
	}
	if len(name) > 200 {
		return &ServiceError{Code: "invalid_org_name", Status: 400, Message: "Organization name must not exceed 200 characters"}
	}
	return nil
}

// createOrganizationWithOwnerTx creates an organization and its owner membership
// inside an existing transaction. Resolves slug collisions by appending a random
// hex suffix. Shared between OrgService.CreateOrganization and AuthService.Register
// so signup can create the org atomically with the user row.
func createOrganizationWithOwnerTx(ctx context.Context, q *db.Queries, name, deploymentMode string, ownerID uuid.UUID) (*db.Organization, error) {
	candidateSlug := generateSlug(name)

	if _, err := q.GetOrganizationBySlug(ctx, candidateSlug); err == nil {
		suffix, err := randomHex(2)
		if err != nil {
			return nil, fmt.Errorf("generate slug suffix: %w", err)
		}
		candidateSlug = candidateSlug + "-" + suffix
		if len(candidateSlug) > 50 {
			candidateSlug = candidateSlug[:50]
			candidateSlug = strings.TrimRight(candidateSlug, "-")
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check slug: %w", err)
	}

	plan := "free"
	if deploymentMode == "self_host" {
		plan = "self_host"
	}

	created, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name: name,
		Slug: candidateSlug,
		Plan: plan,
	})
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}

	if _, err := q.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: created.ID,
		UserID:         ownerID,
		Role:           "owner",
	}); err != nil {
		return nil, fmt.Errorf("create membership: %w", err)
	}

	return &created, nil
}

// GetOrganization returns an organization by ID.
func (s *OrgService) GetOrganization(ctx context.Context, orgID uuid.UUID) (*db.Organization, error) {
	org, err := s.queries.GetOrganizationByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ServiceError{Code: "not_found", Status: 404, Message: "Organization not found"}
		}
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &org, nil
}

// ListUserOrganizations returns all organizations the user is a member of.
func (s *OrgService) ListUserOrganizations(ctx context.Context, userID uuid.UUID) ([]db.Organization, error) {
	orgs, err := s.queries.GetOrganizationsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	return orgs, nil
}

// supportedLocales is the set of allowed locale values.
var supportedLocales = map[string]bool{
	"en": true, "es": true, "fr": true, "de": true, "pt": true,
	"ru": true, "ja": true, "ko": true, "zh": true, "it": true,
}

// UpdateSettings updates the locale and session timeout for an organization.
func (s *OrgService) UpdateSettings(ctx context.Context, orgID uuid.UUID, locale string, sessionTimeout int32) error {
	locale = strings.TrimSpace(locale)
	if !supportedLocales[locale] {
		return &ServiceError{Code: "invalid_locale", Status: 400, Message: "Unsupported locale"}
	}
	if sessionTimeout < 10 || sessionTimeout > 3600 {
		return &ServiceError{Code: "invalid_timeout", Status: 400, Message: "session_timeout_seconds must be between 10 and 3600"}
	}
	err := s.queries.UpdateOrganizationSettings(ctx, db.UpdateOrganizationSettingsParams{
		ID:                    orgID,
		Locale:                locale,
		SessionTimeoutSeconds: sessionTimeout,
	})
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// InitiateDeletion schedules an organization for hard deletion in 14 days.
// All API keys are deactivated and members receive email notices.
func (s *OrgService) InitiateDeletion(ctx context.Context, orgID uuid.UUID) (time.Time, error) {
	scheduledAt := time.Now().Add(14 * 24 * time.Hour)

	err := s.queries.SetOrganizationPendingDeletion(ctx, db.SetOrganizationPendingDeletionParams{
		ID:                  orgID,
		DeletionScheduledAt: sql.NullTime{Time: scheduledAt, Valid: true},
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("set pending deletion: %w", err)
	}

	if err := s.queries.DeactivateApiKeysByOrg(ctx, orgID); err != nil {
		return time.Time{}, fmt.Errorf("deactivate api keys: %w", err)
	}

	// Send deletion notices to all members (best effort — don't fail on email errors).
	org, err := s.queries.GetOrganizationByID(ctx, orgID)
	if err == nil {
		members, err := s.queries.ListMembershipsByOrg(ctx, orgID)
		if err == nil {
			for _, m := range members {
				_ = s.mailer.SendDeletionNotice(m.Email, m.UserName, org.Name, org.Locale, scheduledAt)
			}
		}
	}

	return scheduledAt, nil
}

// CancelDeletion restores an organization from pending_deletion and reactivates API keys.
func (s *OrgService) CancelDeletion(ctx context.Context, orgID uuid.UUID) error {
	if err := s.queries.RestoreOrganization(ctx, orgID); err != nil {
		return fmt.Errorf("restore organization: %w", err)
	}
	if err := s.queries.ReactivateApiKeysByOrg(ctx, orgID); err != nil {
		return fmt.Errorf("reactivate api keys: %w", err)
	}
	return nil
}

// RunHardDeleteCron deletes all organizations that are past their deletion date.
// Each deletion cascades to all related data via DB foreign keys.
func (s *OrgService) RunHardDeleteCron(ctx context.Context) error {
	orgs, err := s.queries.GetOrganizationsDueForDeletion(ctx)
	if err != nil {
		return fmt.Errorf("get organizations due for deletion: %w", err)
	}
	for _, org := range orgs {
		if err := s.queries.DeleteOrganization(ctx, org.ID); err != nil {
			// Log but continue — don't abort entire cron on a single failure.
			// The record remains and will be retried next run.
			slog.Error("hard-delete cron: failed to delete org", "org", org.ID, "error", err)
			continue
		}
	}
	return nil
}

// TransferOwnership makes newOwnerUserID the owner and demotes the current owner to admin.
func (s *OrgService) TransferOwnership(ctx context.Context, orgID, newOwnerUserID uuid.UUID) error {
	return txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		// Get current owner.
		oldOwner, err := q.GetOwnerMembership(ctx, orgID)
		if err != nil {
			return fmt.Errorf("get owner membership: %w", err)
		}

		// Verify new owner is a member.
		newOwner, err := q.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
			OrganizationID: orgID,
			UserID:         newOwnerUserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &ServiceError{Code: "not_a_member", Status: 404, Message: "User is not a member of this organization"}
			}
			return fmt.Errorf("get new owner membership: %w", err)
		}

		// Demote old owner to admin.
		if err := q.UpdateMembershipRole(ctx, db.UpdateMembershipRoleParams{
			Role:           "admin",
			ID:             oldOwner.ID,
			OrganizationID: orgID,
		}); err != nil {
			return fmt.Errorf("demote old owner: %w", err)
		}

		// Promote new owner.
		if err := q.UpdateMembershipRole(ctx, db.UpdateMembershipRoleParams{
			Role:           "owner",
			ID:             newOwner.ID,
			OrganizationID: orgID,
		}); err != nil {
			return fmt.Errorf("promote new owner: %w", err)
		}

		return nil
	})
}

// LeaveOrganization removes the user from the organization.
// Returns an error if the user is the owner (must transfer or delete first).
func (s *OrgService) LeaveOrganization(ctx context.Context, orgID, userID uuid.UUID) error {
	membership, err := s.queries.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ServiceError{Code: "not_a_member", Status: 404, Message: "Not a member of this organization"}
		}
		return fmt.Errorf("get membership: %w", err)
	}

	if membership.Role == "owner" {
		return &ServiceError{Code: "owner_cannot_leave", Status: 403, Message: "Owner cannot leave the organization — transfer ownership or delete the organization first"}
	}

	if err := s.queries.DeleteMembership(ctx, db.DeleteMembershipParams{
		ID:             membership.ID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}

// RemoveMember removes a membership by ID. Cannot remove the owner.
func (s *OrgService) RemoveMember(ctx context.Context, orgID, membershipID uuid.UUID) error {
	target, err := s.queries.GetMembershipByID(ctx, db.GetMembershipByIDParams{
		ID:             membershipID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ServiceError{Code: "not_found", Status: 404, Message: "Membership not found"}
		}
		return fmt.Errorf("get membership: %w", err)
	}
	if target.Role == "owner" {
		return &ServiceError{Code: "cannot_remove_owner", Status: 403, Message: "Cannot remove the organization owner"}
	}

	if err := s.queries.DeleteMembership(ctx, db.DeleteMembershipParams{
		ID:             membershipID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}

// UpdateMemberRole changes the role of a membership. Cannot be used to assign "owner".
func (s *OrgService) UpdateMemberRole(ctx context.Context, orgID, membershipID uuid.UUID, newRole string) error {
	validRoles := map[string]bool{"admin": true, "member": true, "viewer": true}
	if !validRoles[newRole] {
		return &ServiceError{Code: "invalid_role", Status: 400, Message: "Role must be one of: admin, member, viewer"}
	}

	target, err := s.queries.GetMembershipByID(ctx, db.GetMembershipByIDParams{
		ID:             membershipID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ServiceError{Code: "not_found", Status: 404, Message: "Membership not found"}
		}
		return fmt.Errorf("get membership: %w", err)
	}
	if target.Role == "owner" {
		return &ServiceError{Code: "cannot_demote_owner", Status: 403, Message: "Cannot change owner role — use transfer ownership instead"}
	}

	if err := s.queries.UpdateMembershipRole(ctx, db.UpdateMembershipRoleParams{
		Role:           newRole,
		ID:             membershipID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("update membership role: %w", err)
	}
	return nil
}

// ListMembers returns all members of an organization with user info.
func (s *OrgService) ListMembers(ctx context.Context, orgID uuid.UUID) ([]db.ListMembershipsByOrgRow, error) {
	members, err := s.queries.ListMembershipsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// GetSpanMaskingMaps returns the masking map entries for a given span (D-22).
func (s *OrgService) GetSpanMaskingMaps(ctx context.Context, spanID uuid.UUID) ([]db.SpanMaskingMap, error) {
	maps, err := s.queries.GetSpanMaskingMaps(ctx, spanID)
	if err != nil {
		return nil, fmt.Errorf("get span masking maps: %w", err)
	}
	return maps, nil
}

// PrivacySettingsResponse is the response for GET /privacy-settings.
type PrivacySettingsResponse struct {
	StoreSpanContent bool            `json:"store_span_content"`
	MaskingConfig    json.RawMessage `json:"masking_config"`
}

// GetPrivacySettings returns the current privacy settings for an organization.
func (s *OrgService) GetPrivacySettings(ctx context.Context, orgID uuid.UUID) (*PrivacySettingsResponse, error) {
	row, err := s.queries.GetOrganizationPrivacySettings(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("get privacy settings: %w", err)
	}
	return &PrivacySettingsResponse{
		StoreSpanContent: row.StoreSpanContent,
		MaskingConfig:    json.RawMessage(row.MaskingConfig),
	}, nil
}

// maskingConfigPayload is the expected shape of masking_config JSON.
type maskingConfigPayload struct {
	Mode  string `json:"mode"`
	Rules []struct {
		Name    string `json:"name"`
		Pattern string `json:"pattern"`
		Builtin bool   `json:"builtin"`
	} `json:"rules"`
}

// UpdatePrivacySettings validates and persists privacy settings for an organization.
// Per D-03: if metadata-only (store_span_content=false), masking mode is forced to "off".
func (s *OrgService) UpdatePrivacySettings(ctx context.Context, orgID uuid.UUID, storeSpanContent bool, maskingConfig json.RawMessage) error {
	if maskingConfig != nil && len(maskingConfig) > 0 {
		var cfg maskingConfigPayload
		if err := json.Unmarshal(maskingConfig, &cfg); err != nil {
			return &ServiceError{Code: "invalid_masking_config", Status: 400, Message: "invalid masking_config JSON"}
		}
		validModes := map[string]bool{"off": true, "llm_only": true, "llm_storage": true}
		if cfg.Mode != "" && !validModes[cfg.Mode] {
			return &ServiceError{Code: "invalid_masking_mode", Status: 400, Message: "mode must be off, llm_only, or llm_storage"}
		}
		if len(cfg.Rules) > 20 {
			return &ServiceError{Code: "too_many_rules", Status: 400, Message: "maximum 20 masking rules allowed"}
		}
		for i, rule := range cfg.Rules {
			if rule.Name == "" {
				return &ServiceError{Code: "invalid_rule", Status: 400, Message: fmt.Sprintf("rule %d: name is required", i)}
			}
			if rule.Pattern == "" {
				return &ServiceError{Code: "invalid_rule", Status: 400, Message: fmt.Sprintf("rule %q: pattern is required", rule.Name)}
			}
			if len(rule.Pattern) > 512 {
				return &ServiceError{Code: "invalid_rule_pattern", Status: 400, Message: fmt.Sprintf("rule %q: pattern too long (max 512 chars)", rule.Name)}
			}
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				return &ServiceError{Code: "invalid_rule_pattern", Status: 400, Message: fmt.Sprintf("rule %q: invalid regex: %s", rule.Name, err.Error())}
			}
		}
	}

	// Per D-03: if metadata-only, force masking to off.
	if !storeSpanContent && maskingConfig != nil {
		maskingConfig = json.RawMessage(`{"mode":"off","rules":[]}`)
	}

	return s.queries.UpdateOrganizationPrivacySettings(ctx, db.UpdateOrganizationPrivacySettingsParams{
		ID:               orgID,
		StoreSpanContent: storeSpanContent,
		MaskingConfig:    []byte(maskingConfig),
	})
}

// randomHex generates n random bytes and returns them as a hex string.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
