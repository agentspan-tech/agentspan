package middleware_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/crypto"
	mw "github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const testJWTSecret = "test-jwt-secret-32chars-minimum!"
const testHMACSecret = "test-hmac-secret"

// mockDBTX implements db.DBTX for testing.
type mockDBTX struct {
	queryRowFn func(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func (m *mockDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{err: pgx.ErrNoRows}
}

// mockRow implements pgx.Row.
type mockRow struct {
	err error
}

func (m *mockRow) Scan(dest ...interface{}) error {
	return m.err
}

func createValidJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func createExpiredJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(-time.Hour).Unix(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

// setupAuthRouter creates a chi router with Authenticate middleware and a test handler.
func setupAuthRouter(jwtSecret, hmacSecret string, mock *mockDBTX) *chi.Mux {
	r := chi.NewRouter()
	// Use db.New with mock DBTX
	queries := newMockQueries(mock)
	r.Use(mw.Authenticate(jwtSecret, hmacSecret, queries))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	return r
}

func TestAuthenticate_ValidJWT(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	userID := uuid.New()
	token := createValidJWT(t, userID)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "agentorbit_token", Value: token})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthenticate_ExpiredJWT(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	userID := uuid.New()
	token := createExpiredJWT(t, userID)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "agentorbit_token", Value: token})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthenticate_InvalidSignature(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	// Sign with a different secret
	userID := uuid.New()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("wrong-secret-entirely-different!!"))

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "agentorbit_token", Value: signed})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthenticate_NoToken(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthenticate_APIKey_Invalid(t *testing.T) {
	// Mock returns ErrNoRows (key not found)
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ao-"+crypto.HMACDigest("fake", testHMACSecret))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthenticate_BearerHeaderJWT(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	userID := uuid.New()
	token := createValidJWT(t, userID)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer header JWT, got %d", rr.Code)
	}
}

// mockRowWithTime returns a pgx.Row that scans a sql.NullTime into the first dest argument.
type mockRowWithTime struct {
	t time.Time
}

func (m *mockRowWithTime) Scan(dest ...interface{}) error {
	if len(dest) > 0 {
		if nt, ok := dest[0].(*sql.NullTime); ok {
			nt.Valid = true
			nt.Time = m.t
			return nil
		}
	}
	return nil
}

func TestAuthenticate_JWTAfterPasswordChange(t *testing.T) {
	// Create a JWT with iat = now - 2 hours
	userID := uuid.New()
	iat := time.Now().Add(-2 * time.Hour)
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": iat.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	// Mock returns password_changed_at = now - 1 hour (AFTER the JWT iat)
	pwChanged := time.Now().Add(-1 * time.Hour)
	mock := &mockDBTX{
		queryRowFn: func(ctx context.Context, sql string, args ...interface{}) pgx.Row {
			return &mockRowWithTime{t: pwChanged}
		},
	}
	r := setupAuthRouter(testJWTSecret, testHMACSecret, mock)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "agentorbit_token", Value: signed})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for JWT issued before password change, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthenticate_FutureIAT(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	userID := uuid.New()
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(2 * time.Hour).Unix(),
		"iat": time.Now().Add(1 * time.Minute).Unix(), // 1 minute in the future (> 5s tolerance)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for JWT with future iat, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthenticate_InvalidTokenFormat(t *testing.T) {
	r := setupAuthRouter(testJWTSecret, testHMACSecret, &mockDBTX{})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer some-random-token")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
