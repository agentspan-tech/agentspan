package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/agentorbit-tech/agentorbit/processing/internal/crypto"
	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
	"github.com/agentorbit-tech/agentorbit/processing/internal/txutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// dummyHash is a pre-computed bcrypt hash used to equalize timing when a user
// is not found during login. Without this, the absence of a bcrypt comparison
// reveals whether an email address exists in the database.
var dummyHash string

func init() {
	h, _ := bcrypt.GenerateFromPassword([]byte("timing-equalization-dummy"), bcrypt.DefaultCost)
	dummyHash = string(h)
}

// RegisterResult is returned by AuthService.Register.
type RegisterResult struct {
	UserID          uuid.UUID  `json:"user_id"`
	Email           string     `json:"email"`
	OrganizationID  *uuid.UUID `json:"organization_id,omitempty"`
	VerificationURL string     `json:"verification_url,omitempty"`
	AutoLogin       bool       `json:"auto_login,omitempty"`
}

// LoginResult is returned by AuthService.Login.
type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ResetRequestResult is returned by AuthService.RequestPasswordReset.
type ResetRequestResult struct {
	ResetURL string `json:"reset_url,omitempty"`
}

// SetupStatusResult is returned by AuthService.SetupStatus.
type SetupStatusResult struct {
	SetupComplete    bool `json:"setup_complete"`
	RegistrationOpen bool `json:"registration_open"`
}

// loginRateLimiter tracks failed login attempts per email.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func (l *loginRateLimiter) allow(email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	ts := l.attempts[email]
	valid := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		delete(l.attempts, email)
	} else {
		l.attempts[email] = valid
	}
	return len(valid) < l.limit
}

func (l *loginRateLimiter) record(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[email] = append(l.attempts[email], time.Now())
}

// cleanup periodically evicts stale entries to prevent unbounded memory growth.
// Accepts a context so the goroutine exits cleanly on server shutdown (H-1).
func (l *loginRateLimiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-l.window)
			for email, ts := range l.attempts {
				allExpired := true
				for _, t := range ts {
					if t.After(cutoff) {
						allExpired = false
						break
					}
				}
				if allExpired {
					delete(l.attempts, email)
				}
			}
			l.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// AuthService handles authentication business logic.
type AuthService struct {
	queries               *db.Queries
	pool                  *pgxpool.Pool
	mailer                email.Mailer
	jwtSecret             string
	hmacSecret            string
	jwtTTL                time.Duration
	deploymentMode        string
	skipEmailVerification bool
	loginLimiter          *loginRateLimiter
}

// NewAuthService creates a new AuthService.
// ctx controls the lifetime of the background cleanup goroutine (H-1).
func NewAuthService(ctx context.Context, queries *db.Queries, pool *pgxpool.Pool, mailer email.Mailer, jwtSecret, hmacSecret, deploymentMode string, jwtTTL time.Duration, skipEmailVerification bool) *AuthService {
	limiter := &loginRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    5,
		window:   15 * time.Minute,
	}
	go limiter.cleanup(ctx)
	return &AuthService{
		queries:               queries,
		pool:                  pool,
		mailer:                mailer,
		jwtSecret:             jwtSecret,
		hmacSecret:            hmacSecret,
		jwtTTL:                jwtTTL,
		deploymentMode:        deploymentMode,
		skipEmailVerification: skipEmailVerification,
		loginLimiter:          limiter,
	}
}

// SetupStatus returns whether the initial setup (first user registration) is complete.
// For self-host mode, setup is complete when at least one user exists.
// For cloud mode, setup is always complete (open registration).
func (s *AuthService) SetupStatus(ctx context.Context) (*SetupStatusResult, error) {
	if s.deploymentMode != "self_host" {
		return &SetupStatusResult{SetupComplete: true, RegistrationOpen: true}, nil
	}
	count, err := s.queries.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup status: count users: %w", err)
	}
	setupDone := count > 0
	return &SetupStatusResult{SetupComplete: setupDone, RegistrationOpen: !setupDone}, nil
}

// Register creates a new user account, provisions their organization, and sends
// a verification email. User + organization + owner membership are created in a
// single transaction so an abandoned signup never leaves an orphaned user.
// In self-host mode, registration is only allowed for the first user (setup).
// locale controls the language of the verification email ("en" or "ru").
func (s *AuthService) Register(ctx context.Context, emailAddr, name, password, locale, orgName string) (*RegisterResult, error) {
	// Validate inputs (before any DB work).
	if strings.TrimSpace(emailAddr) == "" {
		return nil, &ServiceError{Code: "invalid_email", Message: "Email is required", Status: http.StatusUnprocessableEntity}
	}
	if _, err := mail.ParseAddress(emailAddr); err != nil {
		return nil, &ServiceError{Code: "invalid_email", Message: "Invalid email address", Status: http.StatusUnprocessableEntity}
	}
	if strings.TrimSpace(name) == "" {
		return nil, &ServiceError{Code: "invalid_name", Message: "Name is required", Status: http.StatusUnprocessableEntity}
	}
	if len(password) < 8 {
		return nil, &ServiceError{Code: "password_too_short", Message: "Password must be at least 8 characters", Status: http.StatusUnprocessableEntity}
	}
	if len(password) > 128 {
		return nil, &ServiceError{Code: "password_too_long", Message: "Password must be at most 128 characters", Status: http.StatusUnprocessableEntity}
	}
	if !hasPasswordComplexity(password) {
		return nil, &ServiceError{Code: "password_too_weak", Message: "Password must include uppercase, lowercase, and a digit", Status: http.StatusUnprocessableEntity}
	}
	if err := validateOrgName(orgName); err != nil {
		return nil, err
	}

	// Hash password
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	// Self-host: use advisory lock + transaction to prevent TOCTOU race where two
	// concurrent requests both see zero users and both create admin accounts.
	if s.deploymentMode == "self_host" {
		return s.registerFirstUser(ctx, emailAddr, name, hash, orgName)
	}

	// Cloud path: normal registration.
	if s.skipEmailVerification {
		// Skip email verification: create user + org + membership, auto-verify in one transaction.
		var user db.User
		var org *db.Organization
		err = txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
			q := s.queries.WithTx(tx)

			var txErr error
			user, txErr = q.CreateUser(ctx, db.CreateUserParams{
				Email:        emailAddr,
				Name:         name,
				PasswordHash: hash,
			})
			if txErr != nil {
				if isDuplicateError(txErr) {
					return &ServiceError{Status: 409, Code: "email_exists", Message: "email already exists"}
				}
				return fmt.Errorf("register: create user: %w", txErr)
			}
			if txErr = q.SetUserEmailVerified(ctx, user.ID); txErr != nil {
				return fmt.Errorf("register: auto-verify user: %w", txErr)
			}
			if txErr = q.SetUserTermsAccepted(ctx, db.SetUserTermsAcceptedParams{
				ID:            user.ID,
				PolicyVersion: 1,
			}); txErr != nil {
				return fmt.Errorf("register: record terms acceptance: %w", txErr)
			}
			org, txErr = createOrganizationWithOwnerTx(ctx, q, orgName, s.deploymentMode, user.ID)
			if txErr != nil {
				return txErr
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return &RegisterResult{
			UserID:         user.ID,
			Email:          user.Email,
			OrganizationID: &org.ID,
			AutoLogin:      true,
		}, nil
	}

	// Wrap user creation, org provisioning, verification token, and email send in a
	// single transaction so a mailer failure rolls back the whole signup —
	// otherwise the email gets "stuck" as an unverified account and subsequent
	// registration attempts return 409.
	var user db.User
	var org *db.Organization
	var link string
	err = txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		var txErr error
		user, txErr = q.CreateUser(ctx, db.CreateUserParams{
			Email:        emailAddr,
			Name:         name,
			PasswordHash: hash,
		})
		if txErr != nil {
			if isDuplicateError(txErr) {
				return &ServiceError{Status: 409, Code: "email_exists", Message: "email already exists"}
			}
			return fmt.Errorf("register: create user: %w", txErr)
		}

		if txErr = q.SetUserTermsAccepted(ctx, db.SetUserTermsAcceptedParams{
			ID:            user.ID,
			PolicyVersion: 1,
		}); txErr != nil {
			return fmt.Errorf("register: record terms acceptance: %w", txErr)
		}

		org, txErr = createOrganizationWithOwnerTx(ctx, q, orgName, s.deploymentMode, user.ID)
		if txErr != nil {
			return txErr
		}

		rawToken, tokenHash, txErr := crypto.GenerateToken()
		if txErr != nil {
			return fmt.Errorf("register: generate token: %w", txErr)
		}

		if _, txErr = q.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
			UserID:    user.ID,
			TokenHash: tokenHash,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}); txErr != nil {
			return fmt.Errorf("register: store verification token: %w", txErr)
		}

		link, txErr = s.mailer.SendVerification(emailAddr, name, rawToken, locale)
		if txErr != nil {
			return fmt.Errorf("register: send verification email: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &RegisterResult{
		UserID:         user.ID,
		Email:          user.Email,
		OrganizationID: &org.ID,
	}
	if !s.mailer.IsSMTP() {
		result.VerificationURL = link
	}
	return result, nil
}

// registerFirstUser handles self-host first-user registration inside an advisory-locked transaction.
func (s *AuthService) registerFirstUser(ctx context.Context, emailAddr, name, passwordHash, orgName string) (*RegisterResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("register: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is no-op after commit

	const selfHostRegLockID = int64(0x616753656C66)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", selfHostRegLockID); err != nil {
		return nil, fmt.Errorf("register: advisory lock: %w", err)
	}

	txQueries := s.queries.WithTx(tx)
	count, err := txQueries.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("register: count users: %w", err)
	}
	if count > 0 {
		return nil, &ServiceError{Code: "registration_closed", Message: "Registration is closed. Ask an admin to invite you.", Status: http.StatusForbidden}
	}

	user, err := txQueries.CreateUser(ctx, db.CreateUserParams{
		Email:        emailAddr,
		Name:         name,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if isDuplicateError(err) {
			return nil, &ServiceError{Code: "email_taken", Message: "An account with this email already exists", Status: http.StatusConflict}
		}
		return nil, fmt.Errorf("register: create user: %w", err)
	}

	if err := txQueries.SetUserEmailVerified(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("register: auto-verify first user: %w", err)
	}

	if err := txQueries.SetUserTermsAccepted(ctx, db.SetUserTermsAcceptedParams{
		ID:            user.ID,
		PolicyVersion: 1,
	}); err != nil {
		return nil, fmt.Errorf("register: record terms acceptance: %w", err)
	}

	org, err := createOrganizationWithOwnerTx(ctx, txQueries, orgName, s.deploymentMode, user.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("register: commit: %w", err)
	}

	return &RegisterResult{
		UserID:         user.ID,
		Email:          user.Email,
		OrganizationID: &org.ID,
		AutoLogin:      true,
	}, nil
}

// ResendVerification generates a fresh verification token for an unverified
// user and sends a new verification email. No-ops (silently) if the user is
// unknown, already verified, or email delivery fails — callers must return
// the same response regardless of outcome to avoid email enumeration.
func (s *AuthService) ResendVerification(ctx context.Context, emailAddr, locale string) error {
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("resend verification: get user: %w", err)
	}
	if user.EmailVerifiedAt.Valid {
		return nil
	}

	rawToken, tokenHash, err := crypto.GenerateToken()
	if err != nil {
		return fmt.Errorf("resend verification: generate token: %w", err)
	}

	err = txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := q.DeleteEmailVerificationTokensByUser(ctx, user.ID); err != nil {
			return fmt.Errorf("resend verification: delete old tokens: %w", err)
		}
		if _, err := q.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
			UserID:    user.ID,
			TokenHash: tokenHash,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}); err != nil {
			return fmt.Errorf("resend verification: store token: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if _, err := s.mailer.SendVerification(emailAddr, user.Name, rawToken, locale); err != nil {
		return fmt.Errorf("resend verification: send email: %w", err)
	}
	return nil
}

// VerifyEmail verifies a user's email address using the provided token.
func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	// Hash the token to look it up
	hash, err := crypto.HashToken(token)
	if err != nil {
		return &ServiceError{Code: "invalid_token", Message: "Invalid or expired verification token", Status: http.StatusBadRequest}
	}

	// Look up token
	tokenRow, err := s.queries.GetEmailVerificationTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ServiceError{Code: "invalid_token", Message: "Invalid or expired verification token", Status: http.StatusBadRequest}
		}
		return fmt.Errorf("verify email: get token: %w", err)
	}

	// Check token expiry
	if time.Now().After(tokenRow.ExpiresAt) {
		return &ServiceError{Code: "invalid_token", Message: "Invalid or expired verification token", Status: http.StatusBadRequest}
	}

	// Set email as verified + clean up tokens atomically.
	return txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		if err := q.SetUserEmailVerified(ctx, tokenRow.UserID); err != nil {
			return fmt.Errorf("verify email: set verified: %w", err)
		}

		if err := q.DeleteEmailVerificationTokensByUser(ctx, tokenRow.UserID); err != nil {
			return fmt.Errorf("verify email: delete tokens: %w", err)
		}

		return nil
	})
}

// Login authenticates a user and returns a JWT token.
// Per-email rate limiting: max 5 failed attempts per 15 minutes.
func (s *AuthService) Login(ctx context.Context, emailAddr, password string) (*LoginResult, error) {
	if !s.loginLimiter.allow(emailAddr) {
		return nil, &ServiceError{Code: "too_many_attempts", Message: "Too many login attempts. Try again later.", Status: http.StatusTooManyRequests}
	}

	// Look up user
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Equalize timing: run bcrypt comparison against dummy hash so response
			// time is indistinguishable from an invalid-password attempt.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
			s.loginLimiter.record(emailAddr)
			return nil, &ServiceError{Code: "invalid_credentials", Message: "Invalid email or password", Status: http.StatusUnauthorized}
		}
		return nil, fmt.Errorf("login: get user: %w", err)
	}

	// Check password
	if err := crypto.CheckPassword(password, user.PasswordHash); err != nil {
		s.loginLimiter.record(emailAddr)
		return nil, &ServiceError{Code: "invalid_credentials", Message: "Invalid email or password", Status: http.StatusUnauthorized}
	}

	// Check email verified
	if !s.skipEmailVerification && !user.EmailVerifiedAt.Valid {
		return nil, &ServiceError{Code: "email_not_verified", Message: "Please verify your email before logging in", Status: http.StatusForbidden}
	}

	// Generate JWT
	expiresAt := time.Now().Add(s.jwtTTL)
	claims := jwt.MapClaims{
		"sub": user.ID.String(),
		"exp": expiresAt.Unix(),
		"iat": time.Now().Unix(),
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := jwtToken.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, fmt.Errorf("login: sign jwt: %w", err)
	}

	return &LoginResult{
		Token:     signed,
		ExpiresAt: expiresAt,
	}, nil
}

// RequestPasswordReset initiates the password reset flow.
// Always returns nil error (no email enumeration per AUTH-04).
// locale controls the language of the password reset email ("en" or "ru").
func (s *AuthService) RequestPasswordReset(ctx context.Context, emailAddr, locale string) (*ResetRequestResult, error) {
	// Look up user — if not found, return nil silently (no email enumeration)
	user, err := s.queries.GetUserByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("request password reset: get user: %w", err)
	}

	// Generate token
	rawToken, tokenHash, err := crypto.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("request password reset: generate token: %w", err)
	}

	// Store token
	_, err = s.queries.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	if err != nil {
		return nil, fmt.Errorf("request password reset: store token: %w", err)
	}

	// Send reset email
	link, err := s.mailer.SendPasswordReset(emailAddr, user.Name, rawToken, locale)
	if err != nil {
		return nil, fmt.Errorf("request password reset: send email: %w", err)
	}

	result := &ResetRequestResult{}
	if !s.mailer.IsSMTP() {
		result.ResetURL = link
	}
	return result, nil
}

// ResetPassword resets a user's password using the provided token.
// Uses a transaction with SELECT FOR UPDATE to prevent concurrent reset races.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Validate password length
	if len(newPassword) < 8 {
		return &ServiceError{Code: "password_too_short", Message: "Password must be at least 8 characters", Status: http.StatusUnprocessableEntity}
	}
	if len(newPassword) > 128 {
		return &ServiceError{Code: "password_too_long", Message: "Password must be at most 128 characters", Status: http.StatusUnprocessableEntity}
	}
	if !hasPasswordComplexity(newPassword) {
		return &ServiceError{Code: "password_too_weak", Message: "Password must include uppercase, lowercase, and a digit", Status: http.StatusUnprocessableEntity}
	}

	// Hash the token to look it up
	hash, err := crypto.HashToken(token)
	if err != nil {
		return &ServiceError{Code: "invalid_token", Message: "Invalid or expired reset token", Status: http.StatusBadRequest}
	}

	// Hash new password (before transaction to minimize lock hold time).
	newHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("reset password: hash password: %w", err)
	}

	// Wrap in a transaction: look up token FOR UPDATE, validate, update password, delete tokens.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reset password: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is no-op after commit

	// SELECT FOR UPDATE on the token row prevents concurrent resets with the same token.
	var tokenRow db.PasswordResetToken
	err = tx.QueryRow(ctx,
		"SELECT id, user_id, token_hash, expires_at, used_at, created_at FROM password_reset_tokens WHERE token_hash = $1 FOR UPDATE",
		hash,
	).Scan(&tokenRow.ID, &tokenRow.UserID, &tokenRow.TokenHash, &tokenRow.ExpiresAt, &tokenRow.UsedAt, &tokenRow.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ServiceError{Code: "invalid_token", Message: "Invalid or expired reset token", Status: http.StatusBadRequest}
		}
		return fmt.Errorf("reset password: get token: %w", err)
	}

	if time.Now().After(tokenRow.ExpiresAt) {
		return &ServiceError{Code: "invalid_token", Message: "Invalid or expired reset token", Status: http.StatusBadRequest}
	}
	if tokenRow.UsedAt.Valid {
		return &ServiceError{Code: "invalid_token", Message: "Invalid or expired reset token", Status: http.StatusBadRequest}
	}

	txQueries := s.queries.WithTx(tx)

	if err := txQueries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		PasswordHash: newHash,
		ID:           tokenRow.UserID,
	}); err != nil {
		return fmt.Errorf("reset password: update password: %w", err)
	}

	if err := txQueries.DeletePasswordResetTokensByUser(ctx, tokenRow.UserID); err != nil {
		return fmt.Errorf("reset password: delete tokens: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("reset password: commit: %w", err)
	}

	slog.Info("audit", "action", "password.reset", "user", tokenRow.UserID)
	return nil
}

// GetMe returns the current user's profile.
func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*db.GetUserByIDRow, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return nil, &ServiceError{Status: http.StatusNotFound, Code: "user_not_found", Message: "User not found"}
	}
	return &user, nil
}

// UpdateProfile updates the user's display name.
func (s *AuthService) UpdateProfile(ctx context.Context, userID uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 100 {
		return &ServiceError{Status: http.StatusBadRequest, Code: "invalid_name", Message: "Name must be 1-100 characters"}
	}
	return s.queries.UpdateUserName(ctx, db.UpdateUserNameParams{
		Name: name,
		ID:   userID,
	})
}

// ChangePassword changes the user's password (requires current password verification).
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return &ServiceError{Status: http.StatusBadRequest, Code: "password_too_short", Message: "Password must be at least 8 characters"}
	}
	if len(newPassword) > 128 {
		return &ServiceError{Status: http.StatusBadRequest, Code: "password_too_long", Message: "Password must be at most 128 characters"}
	}
	if !hasPasswordComplexity(newPassword) {
		return &ServiceError{Status: http.StatusBadRequest, Code: "password_too_weak", Message: "Password must include uppercase, lowercase, and a digit"}
	}

	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return &ServiceError{Status: http.StatusNotFound, Code: "user_not_found", Message: "User not found"}
	}

	// Verify current password
	if err := crypto.CheckPassword(currentPassword, user.PasswordHash); err != nil {
		return &ServiceError{Status: http.StatusUnauthorized, Code: "wrong_password", Message: "Current password is incorrect"}
	}

	// Hash new password
	hash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	return s.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		PasswordHash: hash,
		ID:           userID,
	})
}

// hasPasswordComplexity checks that a password contains at least one uppercase,
// one lowercase letter, and one digit.
func hasPasswordComplexity(password string) bool {
	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

// isDuplicateError returns true if the error is a PostgreSQL unique constraint violation.
func isDuplicateError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
