package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/agentspan/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRequireRole_OwnerRequired_IsOwner(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.OrgRoleKey, "owner")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireRole("owner"))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireRole_OwnerRequired_IsMember(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.OrgRoleKey, "member")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireRole("owner"))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.OrgRoleKey, "admin")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireRole("owner", "admin"))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin with owner|admin requirement, got %d", rr.Code)
	}
}

func TestRequireActiveOrg_ActiveOrg(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.OrgStatusKey, "active")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireActiveOrg())
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for active org, got %d", rr.Code)
	}
}

func TestRequireActiveOrg_DeletingOrg(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.OrgStatusKey, "pending_deletion")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireActiveOrg())
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for pending_deletion org, got %d", rr.Code)
	}
}

func TestRequireRole_NoRoleInContext(t *testing.T) {
	r := chi.NewRouter()
	// Deliberately not setting OrgRoleKey
	r.Use(mw.RequireRole("owner"))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no role in context, got %d", rr.Code)
	}
}

func TestRequireOrg_InvalidOrgID(t *testing.T) {
	mock := &mockDBTX{}
	queries := newMockQueries(mock)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.UserIDKey, uuid.New())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Route("/{orgID}", func(r chi.Router) {
		r.Use(mw.RequireOrg(queries))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest("GET", "/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid orgID, got %d", rr.Code)
	}
}

func TestRequireOrg_NoUserInContext(t *testing.T) {
	mock := &mockDBTX{}
	queries := newMockQueries(mock)

	r := chi.NewRouter()
	// Deliberately NOT setting UserIDKey
	r.Route("/{orgID}", func(r chi.Router) {
		r.Use(mw.RequireOrg(queries))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	orgID := uuid.New()
	req := httptest.NewRequest("GET", "/"+orgID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no user in context, got %d", rr.Code)
	}
}

func TestRequireOrg_NonMember(t *testing.T) {
	// Mock returns ErrNoRows for GetMembership (user is not a member)
	mock := &mockDBTX{}
	queries := newMockQueries(mock)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.UserIDKey, uuid.New())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Route("/{orgID}", func(r chi.Router) {
		r.Use(mw.RequireOrg(queries))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	orgID := uuid.New()
	req := httptest.NewRequest("GET", "/"+orgID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Mock DBTX returns ErrNoRows by default, so GetMembership fails -> 403
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-member, got %d: %s", rr.Code, rr.Body.String())
	}
}

// Verify context getters work correctly
func TestContextHelpers(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	apiKeyID := uuid.New()

	ctx := context.Background()
	ctx = context.WithValue(ctx, mw.UserIDKey, userID)
	ctx = context.WithValue(ctx, mw.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, mw.OrgRoleKey, "admin")
	ctx = context.WithValue(ctx, mw.OrgStatusKey, "active")
	ctx = context.WithValue(ctx, mw.OrgPlanKey, "free")
	ctx = context.WithValue(ctx, mw.APIKeyIDKey, apiKeyID)

	if uid, ok := mw.GetUserID(ctx); !ok || uid != userID {
		t.Error("GetUserID failed")
	}
	if oid, ok := mw.GetOrgID(ctx); !ok || oid != orgID {
		t.Error("GetOrgID failed")
	}
	if role, ok := mw.GetOrgRole(ctx); !ok || role != "admin" {
		t.Error("GetOrgRole failed")
	}
	if status, ok := mw.GetOrgStatus(ctx); !ok || status != "active" {
		t.Error("GetOrgStatus failed")
	}
	if plan, ok := mw.GetOrgPlan(ctx); !ok || plan != "free" {
		t.Error("GetOrgPlan failed")
	}
	if akid, ok := mw.GetAPIKeyID(ctx); !ok || akid != apiKeyID {
		t.Error("GetAPIKeyID failed")
	}
}
