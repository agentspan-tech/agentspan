package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agentspan/processing/internal/crypto"
	"github.com/agentspan/processing/internal/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Authenticate is an HTTP middleware that validates Bearer tokens.
//
// Token discrimination (AUTH-07):
//   - Tokens starting with "eyJ" are parsed as JWT (HS256) — browser sessions.
//   - Tokens starting with "as-" are looked up by HMAC-SHA256 digest — agent/proxy auth.
//   - All other tokens and missing headers result in 401.
func Authenticate(jwtSecret string, hmacSecret string, queries *db.Queries) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				if !strings.HasPrefix(authHeader, "Bearer ") {
					writeError(w, http.StatusUnauthorized, "unauthorized", "Authorization header must use Bearer scheme")
					return
				}
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else if cookie, err := r.Cookie("agentspan_token"); err == nil && cookie.Value != "" {
				token = cookie.Value
			}

			if token == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}

			if strings.HasPrefix(token, "eyJ") {
				// JWT authentication — browser sessions
				parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
					if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, jwt.ErrSignatureInvalid
					}
					return []byte(jwtSecret), nil
				}, jwt.WithValidMethods([]string{"HS256"}))
				if err != nil || !parsed.Valid {
					writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired token")
					return
				}
				sub, err := parsed.Claims.GetSubject()
				if err != nil || sub == "" {
					writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid token claims")
					return
				}
				userID, err := uuid.Parse(sub)
				if err != nil {
					writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid token subject")
					return
				}

				// Reject JWTs with iat set in the future (clock skew tolerance: 5s).
				if iatClaim, ok := parsed.Claims.(jwt.MapClaims)["iat"]; ok {
					if iatFloat, ok := iatClaim.(float64); ok {
						iat := time.Unix(int64(iatFloat), 0)
						if iat.After(time.Now().Add(5 * time.Second)) {
							writeError(w, http.StatusUnauthorized, "invalid_token", "Token issued in the future")
							return
						}
					}
				}

				// M-1: Reject JWTs issued before the user's last password change.
				pwChangedAt, err := queries.GetUserPasswordChangedAt(r.Context(), userID)
				if err == nil && pwChangedAt.Valid {
					if iatClaim, ok := parsed.Claims.(jwt.MapClaims)["iat"]; ok {
						if iatFloat, ok := iatClaim.(float64); ok {
							iat := time.Unix(int64(iatFloat), 0)
							if iat.Before(pwChangedAt.Time) {
								writeError(w, http.StatusUnauthorized, "token_expired", "Token was issued before password change")
								return
							}
						}
					}
				}

				ctx := context.WithValue(r.Context(), UserIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if strings.HasPrefix(token, "as-") {
				// API key authentication — agent/proxy auth
				digest := crypto.HMACDigest(token, hmacSecret)
				apiKey, err := queries.GetApiKeyByDigest(r.Context(), digest)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid API key")
						return
					}
					writeError(w, http.StatusInternalServerError, "internal_error", "Authentication service error")
					return
				}
				ctx := context.WithValue(r.Context(), APIKeyIDKey, apiKey.ID)
				ctx = context.WithValue(ctx, OrgIDKey, apiKey.OrganizationID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid token format")
		})
	}
}
