//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

func TestInviteService_CreateAndAccept(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-owner@example.com", "Owner")
	org, err := orgSvc.CreateOrganization(ctx, owner.ID, "Invite Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create invite
	result, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "invitee@example.com", "member", org.Name, "Owner", org.Plan, "en")
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if result.InviteURL == "" {
		t.Error("expected invite URL (non-SMTP mode)")
	}

	// List pending invites
	invites, err := inviteSvc.ListPendingInvites(ctx, org.ID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 pending invite, got %d", len(invites))
	}

	// Create invitee user and accept
	invitee := createTestUser(t, ctx, queries, "invitee@example.com", "Invitee")
	// Extract token from URL
	url := result.InviteURL
	token := url[len("http://test/invite?token="):]
	acceptResult, err := inviteSvc.AcceptInvite(ctx, token, invitee.ID, invitee.Email)
	if err != nil {
		t.Fatalf("accept invite: %v", err)
	}
	if acceptResult.OrganizationID != org.ID {
		t.Errorf("expected org ID %s, got %s", org.ID, acceptResult.OrganizationID)
	}

	// Verify membership
	members, err := queries.ListMembershipsByOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members (owner + invitee), got %d", len(members))
	}
}

func TestInviteService_FreePlanBlocked(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")

	owner := createTestUser(t, ctx, queries, "inv-free@example.com", "Free Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Free Org")

	_, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "x@example.com", "member", org.Name, "Owner", org.Plan, "en")
	if err == nil {
		t.Fatal("expected error for free plan invite")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok || svcErr.Status != 403 {
		t.Errorf("expected 403 ServiceError, got %v", err)
	}
}

func TestInviteService_InvalidRole(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-role@example.com", "Role Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Role Org")

	_, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "x@example.com", "superadmin", org.Name, "Owner", org.Plan, "en")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestInviteService_AlreadyMember(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-dup@example.com", "Dup Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Dup Org")

	// Try to invite the owner (already a member)
	_, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "inv-dup@example.com", "member", org.Name, "Owner", org.Plan, "en")
	if err == nil {
		t.Fatal("expected error for inviting existing member")
	}
}

func TestInviteService_Revoke(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-revoke@example.com", "Revoke Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Revoke Org")

	result, _ := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "rev@example.com", "member", org.Name, "Owner", org.Plan, "en")

	err := inviteSvc.RevokeInvite(ctx, org.ID, result.InviteID)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// List should be empty
	invites, _ := inviteSvc.ListPendingInvites(ctx, org.ID)
	if len(invites) != 0 {
		t.Errorf("expected 0 pending invites after revoke, got %d", len(invites))
	}
}

func TestInviteService_ResendReplacesPrevious(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-resend@example.com", "Resend Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Resend Org")

	// Send first invite
	_, _ = inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "resend@example.com", "member", org.Name, "Owner", org.Plan, "en")

	// Send second invite to same email — should replace
	_, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "resend@example.com", "admin", org.Name, "Owner", org.Plan, "en")
	if err != nil {
		t.Fatalf("resend invite: %v", err)
	}

	// Should still have only 1 pending invite
	invites, _ := inviteSvc.ListPendingInvites(ctx, org.ID)
	if len(invites) != 1 {
		t.Errorf("expected 1 pending invite after resend, got %d", len(invites))
	}
}

func TestInviteService_AcceptInvite_InvalidToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)

	user := createTestUser(t, ctx, queries, "inv-bad@example.com", "Bad")

	_, err := inviteSvc.AcceptInvite(ctx, "invalid-token-12345", user.ID, user.Email)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestInviteService_AcceptInvite_WrongEmail(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-wrong@example.com", "Wrong Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Wrong Org")

	result, _ := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "target@example.com", "member", org.Name, "Owner", org.Plan, "en")
	token := result.InviteURL[len("http://test/invite?token="):]

	// Different user tries to accept
	wrongUser := createTestUser(t, ctx, queries, "wrong-user@example.com", "Wrong")
	_, err := inviteSvc.AcceptInvite(ctx, token, wrongUser.ID, wrongUser.Email)
	if err == nil {
		t.Fatal("expected error for wrong email accepting invite")
	}
}

func TestInviteService_EmailCaseInsensitive(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUser(t, ctx, queries, "inv-case@example.com", "Case Owner")
	org, _ := orgSvc.CreateOrganization(ctx, owner.ID, "Case Org")

	// Create invite with mixed-case email
	result, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "Invitee@Example.COM", "member", org.Name, "Owner", org.Plan, "en")
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	// Create user with lowercase email and accept
	invitee := createTestUser(t, ctx, queries, "invitee@example.com", "Invitee")
	token := result.InviteURL[len("http://test/invite?token="):]
	_, err = inviteSvc.AcceptInvite(ctx, token, invitee.ID, invitee.Email)
	if err != nil {
		t.Fatalf("expected case-insensitive email match to succeed, got: %v", err)
	}
}

func TestInviteService_CreateInvite_InvalidEmail(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUserInvite(t, ctx, queries, "inv-email-owner@example.com", "Owner")
	org, err := orgSvc.CreateOrganization(ctx, owner.ID, "Email Validate Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	badEmails := []string{"not-an-email", "missing@", "@missing.com", ""}
	for _, email := range badEmails {
		_, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, email, "member", org.Name, "Owner", org.Plan, "en")
		if err == nil {
			t.Errorf("expected error for email %q, got nil", email)
			continue
		}
		svcErr, ok := err.(*service.ServiceError)
		if !ok {
			t.Errorf("expected ServiceError for email %q, got %T: %v", email, err, err)
			continue
		}
		if svcErr.Status != 400 {
			t.Errorf("expected status 400 for email %q, got %d", email, svcErr.Status)
		}
	}
}

func TestInviteService_Resend_AtomicReplace(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries
	mailer := &testutil.MockMailer{}

	inviteSvc := service.NewInviteService(queries, pool, mailer)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")

	owner := createTestUserInvite(t, ctx, queries, "resend-owner@example.com", "Resend Owner")
	org, err := orgSvc.CreateOrganization(ctx, owner.ID, "Resend Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create first invite.
	result1, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "resend-target@example.com", "member", org.Name, "Owner", org.Plan, "en")
	if err != nil {
		t.Fatalf("create first invite: %v", err)
	}
	oldInviteID := result1.InviteID

	// Re-send: create invite for same email.
	result2, err := inviteSvc.CreateInvite(ctx, org.ID, owner.ID, "resend-target@example.com", "member", org.Name, "Owner", org.Plan, "en")
	if err != nil {
		t.Fatalf("resend invite: %v", err)
	}

	if result2.InviteID == oldInviteID {
		t.Error("expected new invite ID after re-send, got same ID")
	}

	// Only one pending invite should exist for this email.
	invites, err := inviteSvc.ListPendingInvites(ctx, org.ID)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	count := 0
	for _, inv := range invites {
		if inv.Email != nil && *inv.Email == "resend-target@example.com" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 invite for target email, got %d", count)
	}
}

// Helper for creating test users (duplicated from org_test.go).
func createTestUserInvite(t *testing.T, ctx context.Context, queries *db.Queries, email, name string) db.User {
	t.Helper()
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: email, Name: name,
		PasswordHash: "$2a$10$dummyhashfortest000000000000000000000000000000000000",
	})
	if err != nil {
		t.Fatalf("create test user %s: %v", email, err)
	}
	_ = queries.SetUserEmailVerified(ctx, user.ID)
	return user
}
