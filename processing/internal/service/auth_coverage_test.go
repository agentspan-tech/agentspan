//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
	"github.com/google/uuid"
)

func newAuthSvc(t *testing.T) *service.AuthService {
	t.Helper()
	mailer := &testutil.MockMailer{}
	return service.NewAuthService(context.Background(), sharedQueries, sharedPool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)
}

func TestAuthService_ResendVerification_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(ctx, sharedQueries, sharedPool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	if _, err := svc.Register(ctx, "resend@example.com", "Resend User", "Password1", "en", "Test Org"); err != nil {
		t.Fatalf("register: %v", err)
	}
	before := len(mailer.Calls)

	if err := svc.ResendVerification(ctx, "resend@example.com", "en"); err != nil {
		t.Fatalf("ResendVerification: %v", err)
	}
	if len(mailer.Calls) != before+1 {
		t.Fatalf("expected 1 additional mailer call, got %d", len(mailer.Calls)-before)
	}
}

func TestAuthService_ResendVerification_UnknownEmail(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)
	// Must return nil (silent no-op to avoid email enumeration)
	if err := svc.ResendVerification(ctx, "noone@example.com", "en"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAuthService_ResendVerification_AlreadyVerified(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(ctx, sharedQueries, sharedPool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	result, err := svc.Register(ctx, "verified@example.com", "Verified User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := sharedQueries.SetUserEmailVerified(ctx, result.UserID); err != nil {
		t.Fatalf("set verified: %v", err)
	}

	before := len(mailer.Calls)
	if err := svc.ResendVerification(ctx, "verified@example.com", "en"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(mailer.Calls) != before {
		t.Fatalf("expected no new mailer calls, got %d", len(mailer.Calls)-before)
	}
}

func TestAuthService_GetMe_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "me@example.com", "Me User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	user, err := svc.GetMe(ctx, result.UserID)
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if user.Email != "me@example.com" {
		t.Errorf("expected email me@example.com, got %s", user.Email)
	}
	if user.Name != "Me User" {
		t.Errorf("expected name 'Me User', got %s", user.Name)
	}
}

func TestAuthService_GetMe_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	// Pass a random UUID that doesn't exist
	missing := uuid.New()
	_, err := svc.GetMe(ctx, missing)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 404 {
		t.Errorf("expected 404, got %d", svcErr.Status)
	}
}

func TestAuthService_UpdateProfile_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "update@example.com", "Old Name", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.UpdateProfile(ctx, result.UserID, "New Name"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	user, err := svc.GetMe(ctx, result.UserID)
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if user.Name != "New Name" {
		t.Errorf("expected updated name 'New Name', got %s", user.Name)
	}
}

func TestAuthService_UpdateProfile_EmptyName(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "empty@example.com", "Some User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Whitespace-only name should be rejected (trimmed to empty)
	err = svc.UpdateProfile(ctx, result.UserID, "   ")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 400 || svcErr.Code != "invalid_name" {
		t.Errorf("expected 400/invalid_name, got %d/%s", svcErr.Status, svcErr.Code)
	}
}

func TestAuthService_UpdateProfile_NameTooLong(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "long@example.com", "User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	longName := strings.Repeat("a", 101)
	err = svc.UpdateProfile(ctx, result.UserID, longName)
	if err == nil {
		t.Fatal("expected error for too-long name")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "invalid_name" {
		t.Errorf("expected code invalid_name, got %s", svcErr.Code)
	}
}

func TestAuthService_Login_UnknownEmail(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	_, err := svc.Login(ctx, "ghost@example.com", "Password1")
	if err == nil {
		t.Fatal("expected error for unknown email")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "invalid_credentials" {
		t.Errorf("expected invalid_credentials, got %s", svcErr.Code)
	}
}

func TestAuthService_VerifyEmail_InvalidToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	err := svc.VerifyEmail(ctx, "totally-bogus-token")
	if err == nil {
		t.Fatal("expected error for bad token")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "invalid_token" {
		t.Errorf("expected invalid_token, got %s", svcErr.Code)
	}
}

func TestAuthService_ResetPassword_InvalidToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	err := svc.ResetPassword(ctx, "bad-token", "Password1")
	if err == nil {
		t.Fatal("expected error")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "invalid_token" {
		t.Errorf("expected invalid_token, got %s", svcErr.Code)
	}
}

func TestAuthService_ResetPassword_WeakPassword(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	// Short password
	err := svc.ResetPassword(ctx, "any", "short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Code != "password_too_short" {
		t.Errorf("expected password_too_short, got %s", svcErr.Code)
	}
}

func TestAuthService_RequestPasswordReset_UnknownEmail(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(ctx, sharedQueries, sharedPool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	// Should silently succeed to avoid email enumeration
	if _, err := svc.RequestPasswordReset(ctx, "noone@example.com", "en"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(mailer.Calls) != 0 {
		t.Fatalf("expected no mailer calls, got %d", len(mailer.Calls))
	}
}

func TestAuthService_RequestPasswordReset_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewAuthService(ctx, sharedQueries, sharedPool, mailer, "test-jwt-secret", "test-hmac-secret", "cloud", 24*time.Hour, false)

	if _, err := svc.Register(ctx, "reset-req@example.com", "Reset User", "Password1", "en", "Test Org"); err != nil {
		t.Fatalf("register: %v", err)
	}
	before := len(mailer.Calls)

	if _, err := svc.RequestPasswordReset(ctx, "reset-req@example.com", "en"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if len(mailer.Calls) != before+1 {
		t.Fatalf("expected 1 more mailer call, got %d", len(mailer.Calls)-before)
	}
}

func TestAuthService_ChangePassword_WeakPassword(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "chweak@example.com", "User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	err = svc.ChangePassword(ctx, result.UserID, "Password1", "short")
	if err == nil {
		t.Fatal("expected weak password error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "password_too_short" {
		t.Errorf("expected password_too_short, got %+v", err)
	}

	err = svc.ChangePassword(ctx, result.UserID, "Password1", "alllowercase1")
	if err == nil {
		t.Fatal("expected complexity error")
	}
	svcErr, _ = err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "password_too_weak" {
		t.Errorf("expected password_too_weak, got %+v", err)
	}

	longPwd := strings.Repeat("A1b", 50) // 150 chars
	err = svc.ChangePassword(ctx, result.UserID, "Password1", longPwd)
	if err == nil {
		t.Fatal("expected password_too_long error")
	}
	svcErr, _ = err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "password_too_long" {
		t.Errorf("expected password_too_long, got %+v", err)
	}
}

func TestAuthService_ChangePassword_WrongCurrent(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	svc := newAuthSvc(t)

	result, err := svc.Register(ctx, "chwrong@example.com", "User", "Password1", "en", "Test Org")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	err = svc.ChangePassword(ctx, result.UserID, "WrongCurrent1", "NewPassword1")
	if err == nil {
		t.Fatal("expected wrong password error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "wrong_password" {
		t.Errorf("expected wrong_password, got %+v", err)
	}
}
