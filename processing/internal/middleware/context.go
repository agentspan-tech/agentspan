package middleware

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	UserEmailKey contextKey = "user_email"
	OrgIDKey     contextKey = "org_id"
	OrgRoleKey   contextKey = "org_role"
	OrgStatusKey contextKey = "org_status"
	OrgPlanKey   contextKey = "org_plan"
	APIKeyIDKey  contextKey = "api_key_id"
)

// GetUserID retrieves the authenticated user's UUID from the context.
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return v, ok
}

// GetOrgID retrieves the current organization UUID from the context.
func GetOrgID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(OrgIDKey).(uuid.UUID)
	return v, ok
}

// GetOrgRole retrieves the user's role in the current organization.
func GetOrgRole(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(OrgRoleKey).(string)
	return v, ok
}

// GetOrgStatus retrieves the current organization's status from the context.
func GetOrgStatus(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(OrgStatusKey).(string)
	return v, ok
}

// GetOrgPlan retrieves the current organization's plan from the context.
func GetOrgPlan(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(OrgPlanKey).(string)
	return v, ok
}

// GetAPIKeyID retrieves the authenticated API key's UUID from the context.
func GetAPIKeyID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(APIKeyIDKey).(uuid.UUID)
	return v, ok
}
