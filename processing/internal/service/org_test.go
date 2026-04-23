//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

// createTestUser is a helper to create a user and verify their email.
func createTestUser(t *testing.T, ctx context.Context, queries *db.Queries, email, name string) db.User {
	t.Helper()
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		Name:         name,
		PasswordHash: "$2a$10$dummyhashfortest000000000000000000000000000000000000",
	})
	if err != nil {
		t.Fatalf("create test user %s: %v", email, err)
	}
	_ = queries.SetUserEmailVerified(ctx, user.ID)
	return user
}

func TestOrgService_Create(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "org-create@example.com", "Org Creator")

	org, err := svc.CreateOrganization(ctx, user.ID, "Test Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	if org.Name != "Test Org" {
		t.Errorf("expected name 'Test Org', got '%s'", org.Name)
	}
	if org.Plan != "free" {
		t.Errorf("expected plan 'free', got '%s'", org.Plan)
	}

	// Verify owner membership
	members, err := queries.ListMembershipsByOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].Role != "owner" {
		t.Errorf("expected role 'owner', got '%s'", members[0].Role)
	}
}

func TestOrgService_Get(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "org-get@example.com", "Org Getter")
	created, err := svc.CreateOrganization(ctx, user.ID, "Get Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	fetched, err := svc.GetOrganization(ctx, created.ID)
	if err != nil {
		t.Fatalf("get org failed: %v", err)
	}
	if fetched.Name != "Get Org" {
		t.Errorf("expected name 'Get Org', got '%s'", fetched.Name)
	}
}

func TestOrgService_UpdateSettings(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "org-settings@example.com", "Settings User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Settings Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	err = svc.UpdateSettings(ctx, org.ID, "ru", 120)
	if err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	updated, err := svc.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org failed: %v", err)
	}
	if updated.Locale != "ru" {
		t.Errorf("expected locale 'ru', got '%s'", updated.Locale)
	}
	if updated.SessionTimeoutSeconds != 120 {
		t.Errorf("expected session timeout 120, got %d", updated.SessionTimeoutSeconds)
	}
}

func TestOrgService_SoftDelete(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "org-delete@example.com", "Delete User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Delete Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	scheduledAt, err := svc.InitiateDeletion(ctx, org.ID)
	if err != nil {
		t.Fatalf("initiate deletion failed: %v", err)
	}
	if scheduledAt.IsZero() {
		t.Error("expected non-zero scheduled deletion time")
	}

	updated, err := svc.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org failed: %v", err)
	}
	if updated.Status != "pending_deletion" {
		t.Errorf("expected status 'pending_deletion', got '%s'", updated.Status)
	}
}

func TestOrgService_CancelDeletion(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "org-cancel@example.com", "Cancel User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Cancel Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	_, err = svc.InitiateDeletion(ctx, org.ID)
	if err != nil {
		t.Fatalf("initiate deletion failed: %v", err)
	}

	err = svc.CancelDeletion(ctx, org.ID)
	if err != nil {
		t.Fatalf("cancel deletion failed: %v", err)
	}

	restored, err := svc.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org failed: %v", err)
	}
	if restored.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", restored.Status)
	}
}

func TestOrgService_TransferOwnership(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	owner := createTestUser(t, ctx, queries, "org-owner@example.com", "Owner")
	newOwner := createTestUser(t, ctx, queries, "org-new-owner@example.com", "New Owner")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Transfer Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	// Add new owner as member first
	_, err = queries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: org.ID,
		UserID:         newOwner.ID,
		Role:           "admin",
	})
	if err != nil {
		t.Fatalf("create membership failed: %v", err)
	}

	err = svc.TransferOwnership(ctx, org.ID, newOwner.ID)
	if err != nil {
		t.Fatalf("transfer ownership failed: %v", err)
	}

	// Verify new owner has owner role
	membership, err := queries.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
		OrganizationID: org.ID,
		UserID:         newOwner.ID,
	})
	if err != nil {
		t.Fatalf("get new owner membership failed: %v", err)
	}
	if membership.Role != "owner" {
		t.Errorf("expected new owner role 'owner', got '%s'", membership.Role)
	}

	// Verify old owner is now admin
	oldMembership, err := queries.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
		OrganizationID: org.ID,
		UserID:         owner.ID,
	})
	if err != nil {
		t.Fatalf("get old owner membership failed: %v", err)
	}
	if oldMembership.Role != "admin" {
		t.Errorf("expected old owner role 'admin', got '%s'", oldMembership.Role)
	}
}

func TestOrgService_UpdateSettings_LocaleTrimmed(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "locale-trim@example.com", "Locale User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Locale Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Update with whitespace-padded locale.
	err = svc.UpdateSettings(ctx, org.ID, "  en  ", 60)
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}

	updated, err := svc.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if updated.Locale != "en" {
		t.Errorf("expected trimmed locale 'en', got %q", updated.Locale)
	}
}

func TestOrgService_LeaveOrganization(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	owner := createTestUser(t, ctx, queries, "org-leave-owner@example.com", "Owner")
	member := createTestUser(t, ctx, queries, "org-leave-member@example.com", "Member")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Leave Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = queries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: org.ID,
		UserID:         member.ID,
		Role:           "member",
	})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	// Owner cannot leave
	err = svc.LeaveOrganization(ctx, org.ID, owner.ID)
	if err == nil {
		t.Fatal("expected error when owner tries to leave")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "owner_cannot_leave" {
		t.Errorf("expected code 'owner_cannot_leave', got %q", svcErr.Code)
	}

	// Member can leave
	err = svc.LeaveOrganization(ctx, org.ID, member.ID)
	if err != nil {
		t.Fatalf("member leave: %v", err)
	}
}

func TestOrgService_RemoveMember(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	owner := createTestUser(t, ctx, queries, "rm-owner@example.com", "Owner")
	member := createTestUser(t, ctx, queries, "rm-member@example.com", "Member")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Remove Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	membership, err := queries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: org.ID,
		UserID:         member.ID,
		Role:           "member",
	})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	// Cannot remove owner
	ownerMembership, err := queries.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
		OrganizationID: org.ID,
		UserID:         owner.ID,
	})
	if err != nil {
		t.Fatalf("get owner membership: %v", err)
	}
	err = svc.RemoveMember(ctx, org.ID, ownerMembership.ID)
	if err == nil {
		t.Fatal("expected error when removing owner")
	}

	// Can remove regular member
	err = svc.RemoveMember(ctx, org.ID, membership.ID)
	if err != nil {
		t.Fatalf("remove member: %v", err)
	}
}

func TestOrgService_UpdateMemberRole(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	owner := createTestUser(t, ctx, queries, "role-owner@example.com", "Owner")
	member := createTestUser(t, ctx, queries, "role-member@example.com", "Member")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Role Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	membership, err := queries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: org.ID,
		UserID:         member.ID,
		Role:           "member",
	})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	// Invalid role
	err = svc.UpdateMemberRole(ctx, org.ID, membership.ID, "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}

	// Cannot change owner role
	ownerMembership, err := queries.GetMembershipByOrgAndUser(ctx, db.GetMembershipByOrgAndUserParams{
		OrganizationID: org.ID,
		UserID:         owner.ID,
	})
	if err != nil {
		t.Fatalf("get owner membership: %v", err)
	}
	err = svc.UpdateMemberRole(ctx, org.ID, ownerMembership.ID, "admin")
	if err == nil {
		t.Fatal("expected error when changing owner role")
	}

	// Valid role change
	err = svc.UpdateMemberRole(ctx, org.ID, membership.ID, "admin")
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
}

func TestOrgService_ListMembers(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	owner := createTestUser(t, ctx, queries, "list-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "List Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	members, err := svc.ListMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
}

func TestOrgService_CancelDeletion_ActiveOrg(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "cancel-active@example.com", "User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Active Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Cancel deletion on active org is idempotent — should not error.
	err = svc.CancelDeletion(ctx, org.ID)
	if err != nil {
		t.Fatalf("cancel deletion on active org: %v", err)
	}

	// Verify org is still active.
	updated, err := svc.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if updated.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", updated.Status)
	}
}

func TestOrgService_CreateOrganization_NameTooLong(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(queries, pool, mailer, "cloud")
	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "longname@example.com", "Long Name User")

	longName := ""
	for len(longName) <= 200 {
		longName += "a"
	}

	_, err := svc.CreateOrganization(ctx, user.ID, longName)
	if err == nil {
		t.Fatal("expected error for name > 200 chars, got nil")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if svcErr.Status != 400 {
		t.Errorf("expected status 400, got %d", svcErr.Status)
	}
}
