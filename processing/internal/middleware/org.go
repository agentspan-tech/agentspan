package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/agentspan/processing/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// writeError writes a JSON error response. Used internally to avoid importing handler (cycle prevention).
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	resp.Error.Code = code
	resp.Error.Message = message
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// RequireOrg is an HTTP middleware that enforces organization membership.
//
// It extracts orgID from the URL path (chi.URLParam "orgID"), verifies that the
// authenticated user (set by Authenticate middleware) is a member, and sets
// OrgIDKey, OrgRoleKey, OrgStatusKey, and OrgPlanKey in the request context (D-05).
func RequireOrg(queries *db.Queries) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgIDStr := chi.URLParam(r, "orgID")
			orgID, err := uuid.Parse(orgIDStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID")
				return
			}

			userID, ok := GetUserID(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}

			membership, err := queries.GetMembership(r.Context(), db.GetMembershipParams{
				OrganizationID: orgID,
				UserID:         userID,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, http.StatusForbidden, "forbidden", "Not a member of this organization")
					return
				}
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify membership")
				return
			}

			org, err := queries.GetOrganizationByID(r.Context(), orgID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, http.StatusNotFound, "not_found", "Organization not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch organization")
				return
			}

			ctx := context.WithValue(r.Context(), OrgIDKey, membership.OrganizationID)
			ctx = context.WithValue(ctx, OrgRoleKey, membership.Role)
			ctx = context.WithValue(ctx, OrgStatusKey, org.Status)
			ctx = context.WithValue(ctx, OrgPlanKey, org.Plan)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole is an HTTP middleware that checks the user's role in the current organization.
// It must be used after RequireOrg.
func RequireRole(roles ...string) func(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := GetOrgRole(r.Context())
			if !ok {
				writeError(w, http.StatusForbidden, "forbidden", "Role not set in context")
				return
			}
			if !allowed[role] {
				writeError(w, http.StatusForbidden, "forbidden", "Insufficient role — required: "+strings.Join(roles, " or "))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireActiveOrg is an HTTP middleware that rejects requests when the organization
// is in pending_deletion state. It must be used after RequireOrg.
func RequireActiveOrg() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			status, ok := GetOrgStatus(r.Context())
			if !ok {
				writeError(w, http.StatusForbidden, "forbidden", "Organization status not set in context")
				return
			}
			if status == "pending_deletion" {
				writeError(w, http.StatusForbidden, "org_pending_deletion", "Organization is scheduled for deletion")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
