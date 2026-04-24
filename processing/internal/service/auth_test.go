//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

func TestAuthService_Register(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	result, err := svc.Register(context.Background(), "test@example.com", "Test User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if result.UserID.String() == "" {
		t.Error("expected non-empty user ID")
	}
	if result.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", result.Email)
	}
	if len(mailer.Calls) != 1 {
		t.Errorf("expected 1 mailer call, got %d", len(mailer.Calls))
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	_, err := svc.Register(context.Background(), "dup@example.com", "User 1", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// Second registration with same email should return 409 error
	result, err := svc.Register(context.Background(), "dup@example.com", "User 2", "Password2", "en", "Test Org")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result for duplicate email, got %+v", result)
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if svcErr.Status != 409 {
		t.Errorf("expected status 409, got %d", svcErr.Status)
	}
	if svcErr.Code != "email_exists" {
		t.Errorf("expected code 'email_exists', got %q", svcErr.Code)
	}
	// No new user was created, so only 1 mailer call from first registration
	if len(mailer.Calls) != 1 {
		t.Errorf("expected 1 mailer call (no verification sent for duplicate), got %d", len(mailer.Calls))
	}
}

// TestAuthService_Register_RollsBackOnMailerFailure verifies that when the
// verification email fails to send, the user row is not persisted — so the
// same email can be retried instead of getting locked out with a 409.
func TestAuthService_Register_RollsBackOnMailerFailure(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{VerificationErr: errors.New("smtp 500: Subject is not ASCII")}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	_, err := svc.Register(ctx, "rollback@example.com", "Rollback User", "Password1", "ru", "Test Org")
	if err == nil {
		t.Fatal("expected error when mailer fails, got nil")
	}

	// User row must not exist — mailer failure should roll back the whole transaction.
	if _, err := queries.GetUserByEmail(ctx, "rollback@example.com"); err == nil {
		t.Fatal("expected user to be rolled back after mailer failure, but user still exists")
	}

	// Retry with a working mailer must succeed (no 409 email_exists).
	mailer.VerificationErr = nil
	result, err := svc.Register(ctx, "rollback@example.com", "Rollback User", "Password1", "ru", "Test Org")
	if err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
	if result.UserID.String() == "" {
		t.Error("expected non-empty user ID on retry")
	}
}

func TestAuthService_Login(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "login@example.com", "Login User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Verify email first (required for login)
	err = queries.SetUserEmailVerified(ctx, result.UserID)
	if err != nil {
		t.Fatalf("verify email failed: %v", err)
	}

	loginResult, err := svc.Login(ctx, "login@example.com", "Password1")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginResult.Token == "" {
		t.Error("expected non-empty JWT token")
	}
	if loginResult.ExpiresAt.Before(time.Now()) {
		t.Error("expected expires_at to be in the future")
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "wrong@example.com", "Wrong User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_ = queries.SetUserEmailVerified(ctx, result.UserID)

	_, err = svc.Login(ctx, "wrong@example.com", "WrongPassword1")
	if err == nil {
		t.Fatal("expected error on wrong password, got nil")
	}
}

func TestAuthService_Login_RateLimited(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "rate@example.com", "Rate User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_ = queries.SetUserEmailVerified(ctx, result.UserID)

	// Exhaust rate limit (5 failed attempts)
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, "rate@example.com", "WrongPassword1")
	}

	// 6th attempt should be rate limited
	_, err = svc.Login(ctx, "rate@example.com", "Password1")
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "too_many_attempts" {
		t.Errorf("expected code too_many_attempts, got %s", svcErr.Code)
	}
}

func TestAuthService_VerifyEmail(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "verify@example.com", "Verify User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// The verification URL is returned by MockMailer (non-SMTP)
	if result.VerificationURL == "" {
		t.Fatal("expected verification URL from non-SMTP mailer")
	}

	// Extract token from URL -- the MockMailer returns http://test/verify?token=<token>
	token := result.VerificationURL[len("http://test/verify?token="):]
	err = svc.VerifyEmail(ctx, token)
	if err != nil {
		t.Fatalf("verify email failed: %v", err)
	}

	// Login should now work
	loginResult, err := svc.Login(ctx, "verify@example.com", "Password1")
	if err != nil {
		t.Fatalf("login after verify failed: %v", err)
	}
	if loginResult.Token == "" {
		t.Error("expected non-empty JWT token after verification")
	}
}

func TestAuthService_ResetPassword(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "reset@example.com", "Reset User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_ = queries.SetUserEmailVerified(ctx, result.UserID)

	// Request reset
	resetResult, err := svc.RequestPasswordReset(ctx, "reset@example.com", "en")
	if err != nil {
		t.Fatalf("request password reset failed: %v", err)
	}
	if resetResult.ResetURL == "" {
		t.Fatal("expected reset URL from non-SMTP mailer")
	}

	// Extract token
	token := resetResult.ResetURL[len("http://test/reset?token="):]
	err = svc.ResetPassword(ctx, token, "NewPassword1")
	if err != nil {
		t.Fatalf("reset password failed: %v", err)
	}

	// Login with new password should work
	loginResult, err := svc.Login(ctx, "reset@example.com", "NewPassword1")
	if err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
	if loginResult.Token == "" {
		t.Error("expected non-empty JWT token")
	}

	// Login with old password should fail
	_, err = svc.Login(ctx, "reset@example.com", "Password1")
	if err == nil {
		t.Fatal("expected error with old password, got nil")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "change@example.com", "Change User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_ = queries.SetUserEmailVerified(ctx, result.UserID)

	// Change password with correct current password
	err = svc.ChangePassword(ctx, result.UserID, "Password1", "NewPassword1")
	if err != nil {
		t.Fatalf("change password failed: %v", err)
	}

	// Login with new password should work
	loginResult, err := svc.Login(ctx, "change@example.com", "NewPassword1")
	if err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
	if loginResult.Token == "" {
		t.Error("expected non-empty JWT token")
	}

	// Login with old password should fail
	_, err = svc.Login(ctx, "change@example.com", "Password1")
	if err == nil {
		t.Fatal("expected error with old password, got nil")
	}

	// Change with wrong current password should fail
	err = svc.ChangePassword(ctx, result.UserID, "WrongPassword1", "AnotherPassword1")
	if err == nil {
		t.Fatal("expected error with wrong current password, got nil")
	}
}

func TestAuthService_Register_UserAndTokenAtomic(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
	ctx := context.Background()

	result, err := svc.Register(ctx, "atomic@example.com", "Atomic User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify both user and email verification token exist
	user, err := queries.GetUserByEmail(ctx, "atomic@example.com")
	if err != nil {
		t.Fatalf("expected user to exist: %v", err)
	}
	if user.ID != result.UserID {
		t.Errorf("user ID mismatch: %s vs %s", user.ID, result.UserID)
	}

	// Verify token exists by using the verification URL
	if result.VerificationURL == "" {
		t.Fatal("expected verification URL from non-SMTP mailer")
	}
	token := result.VerificationURL[len("http://test/verify?token="):]
	err = svc.VerifyEmail(ctx, token)
	if err != nil {
		t.Fatalf("verify email with generated token failed: %v", err)
	}
}

func TestAuthService_VerifyEmail_AtomicCleanup(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
	ctx := context.Background()

	result, err := svc.Register(ctx, "cleanup@example.com", "Cleanup User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	token := result.VerificationURL[len("http://test/verify?token="):]
	err = svc.VerifyEmail(ctx, token)
	if err != nil {
		t.Fatalf("verify email failed: %v", err)
	}

	// User should be verified
	user, err := queries.GetUserByEmail(ctx, "cleanup@example.com")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !user.EmailVerifiedAt.Valid {
		t.Error("expected email to be verified")
	}

	// Re-verification with same token should fail (token was deleted)
	err = svc.VerifyEmail(ctx, token)
	if err == nil {
		t.Fatal("expected error re-verifying with deleted token")
	}
}

// Register must provision an organization + owner-membership atomically with the
// user row so new signups land on the dashboard instead of a second "create org"
// step after email verification.
func TestAuthService_Register_CreatesOrganization(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
	ctx := context.Background()

	result, err := svc.Register(ctx, "orgowner@example.com", "Org Owner", "Password1", "en", "Acme Inc")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if result.OrganizationID == nil {
		t.Fatal("expected RegisterResult.OrganizationID to be set")
	}

	org, err := queries.GetOrganizationByID(ctx, *result.OrganizationID)
	if err != nil {
		t.Fatalf("expected organization to exist: %v", err)
	}
	if org.Name != "Acme Inc" {
		t.Errorf("expected org name 'Acme Inc', got %q", org.Name)
	}

	orgs, err := queries.GetOrganizationsByUserID(ctx, result.UserID)
	if err != nil {
		t.Fatalf("list user orgs: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected exactly 1 org for user, got %d", len(orgs))
	}
	if orgs[0].ID != *result.OrganizationID {
		t.Errorf("expected user to own organization %s, got %s", *result.OrganizationID, orgs[0].ID)
	}
}

// Register must reject empty organization names — previously the frontend stashed
// the org name in sessionStorage and re-prompted after email verification,
// which broke when users opened the verification link on a different device.
func TestAuthService_Register_RequiresOrganizationName(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
	ctx := context.Background()

	_, err := svc.Register(ctx, "noorg@example.com", "No Org", "Password1", "en", "   ")
	if err == nil {
		t.Fatal("expected error for empty organization name, got nil")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 400 || svcErr.Code != "invalid_org_name" {
		t.Errorf("expected 400/invalid_org_name, got %d/%s", svcErr.Status, svcErr.Code)
	}

	// The user row must not exist — validation must happen before any DB work.
	if _, err := queries.GetUserByEmail(ctx, "noorg@example.com"); err == nil {
		t.Error("expected user creation to be skipped when orgName is invalid")
	}
}

// Register in self-host mode must also provision the organization for the first
// user (auto-login path) — otherwise the post-verify re-prompt re-appears.
func TestAuthService_Register_SelfHost_CreatesOrganization(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "self_host", 24*time.Hour, false)
	ctx := context.Background()

	result, err := svc.Register(ctx, "admin@example.com", "Admin", "Password1", "en", "Self-Host HQ")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if !result.AutoLogin {
		t.Error("expected AutoLogin=true for self-host first user")
	}
	if result.OrganizationID == nil {
		t.Fatal("expected OrganizationID to be set for self-host first user")
	}
	org, err := queries.GetOrganizationByID(ctx, *result.OrganizationID)
	if err != nil {
		t.Fatalf("expected organization to exist: %v", err)
	}
	if org.Plan != "self_host" {
		t.Errorf("expected plan 'self_host', got %q", org.Plan)
	}
}

// Register with skipEmailVerification (cloud) also creates the org atomically.
func TestAuthService_Register_SkipVerification_CreatesOrganization(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, true)
	ctx := context.Background()

	result, err := svc.Register(ctx, "skip@example.com", "Skip User", "Password1", "en", "Skip Org")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if !result.AutoLogin {
		t.Error("expected AutoLogin=true when email verification is skipped")
	}
	if result.OrganizationID == nil {
		t.Fatal("expected OrganizationID to be set")
	}
	orgs, err := queries.GetOrganizationsByUserID(ctx, result.UserID)
	if err != nil {
		t.Fatalf("list user orgs: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}
}

// The user + org + owner membership must all commit or all roll back together.
// If the mailer fails, nothing should be left behind — particularly no orphan
// organization rows.
func TestAuthService_Register_RollsBackOrganizationOnMailerFailure(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{VerificationErr: errors.New("smtp 500")}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
	ctx := context.Background()

	_, err := svc.Register(ctx, "rbf@example.com", "Rollback", "Password1", "en", "Doomed Org")
	if err == nil {
		t.Fatal("expected error when mailer fails, got nil")
	}

	if _, err := queries.GetUserByEmail(ctx, "rbf@example.com"); err == nil {
		t.Error("user should have been rolled back")
	}
	if _, err := queries.GetOrganizationBySlug(ctx, "doomed-org"); err == nil {
		t.Error("organization should have been rolled back")
	}
}

func TestAuthService_SetupStatus(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	ctx := context.Background()

	// Self-host: setup not complete when no users
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "self_host", 24*time.Hour, false)
	status, err := svc.SetupStatus(ctx)
	if err != nil {
		t.Fatalf("setup status failed: %v", err)
	}
	if status.SetupComplete {
		t.Error("expected setup not complete when no users")
	}
	if !status.RegistrationOpen {
		t.Error("expected registration open when no users in self-host")
	}

	// Register first user
	_, err = svc.Register(ctx, "first@example.com", "First User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register first user failed: %v", err)
	}

	// Now setup should be complete
	status, err = svc.SetupStatus(ctx)
	if err != nil {
		t.Fatalf("setup status failed: %v", err)
	}
	if !status.SetupComplete {
		t.Error("expected setup complete after first user registration")
	}
	if status.RegistrationOpen {
		t.Error("expected registration closed after first user in self-host")
	}
}
