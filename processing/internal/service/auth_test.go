//go:build integration

package service_test

import (
	"context"
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

	result, err := svc.Register(context.Background(), "test@example.com", "Test User", "Password1", "en")
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

	_, err := svc.Register(context.Background(), "dup@example.com", "User 1", "Password1", "en")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// Second registration with same email should return 409 error
	result, err := svc.Register(context.Background(), "dup@example.com", "User 2", "Password2", "en")
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

func TestAuthService_Login(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(context.Background(), queries, pool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	ctx := context.Background()
	result, err := svc.Register(ctx, "login@example.com", "Login User", "Password1", "en")
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
	result, err := svc.Register(ctx, "wrong@example.com", "Wrong User", "Password1", "en")
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
	result, err := svc.Register(ctx, "rate@example.com", "Rate User", "Password1", "en")
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
	result, err := svc.Register(ctx, "verify@example.com", "Verify User", "Password1", "en")
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
	result, err := svc.Register(ctx, "reset@example.com", "Reset User", "Password1", "en")
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
	result, err := svc.Register(ctx, "change@example.com", "Change User", "Password1", "en")
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

	result, err := svc.Register(ctx, "atomic@example.com", "Atomic User", "Password1", "en")
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

	result, err := svc.Register(ctx, "cleanup@example.com", "Cleanup User", "Password1", "en")
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
	_, err = svc.Register(ctx, "first@example.com", "First User", "Password1", "en")
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
