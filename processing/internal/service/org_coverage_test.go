//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
	"github.com/google/uuid"
)

func TestOrgService_ListUserOrganizations(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "lu@example.com", "LU User")

	// No orgs yet
	orgs, err := svc.ListUserOrganizations(ctx, user.ID)
	if err != nil {
		t.Fatalf("list user orgs (empty): %v", err)
	}
	if len(orgs) != 0 {
		t.Errorf("expected 0 orgs, got %d", len(orgs))
	}

	// Create 2 orgs, both owned by this user
	_, err = svc.CreateOrganization(ctx, user.ID, "Org One")
	if err != nil {
		t.Fatalf("create org1: %v", err)
	}
	_, err = svc.CreateOrganization(ctx, user.ID, "Org Two")
	if err != nil {
		t.Fatalf("create org2: %v", err)
	}

	orgs, err = svc.ListUserOrganizations(ctx, user.ID)
	if err != nil {
		t.Fatalf("list user orgs: %v", err)
	}
	if len(orgs) != 2 {
		t.Errorf("expected 2 orgs, got %d", len(orgs))
	}
}

func TestOrgService_GetPrivacySettings_Default(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-get@example.com", "Priv User")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	settings, err := svc.GetPrivacySettings(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPrivacySettings: %v", err)
	}
	// Default: store_span_content=true, masking_config=NULL (no masking).
	if !settings.StoreSpanContent {
		t.Errorf("expected default StoreSpanContent=true, got false")
	}
}

func TestOrgService_UpdatePrivacySettings_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-upd@example.com", "Priv U")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv Upd Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	cfg := json.RawMessage(`{"mode":"llm_only","rules":[{"name":"email","pattern":"\\S+@\\S+","builtin":true}]}`)
	if err := svc.UpdatePrivacySettings(ctx, org.ID, true, cfg); err != nil {
		t.Fatalf("UpdatePrivacySettings: %v", err)
	}

	settings, err := svc.GetPrivacySettings(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPrivacySettings: %v", err)
	}
	if !settings.StoreSpanContent {
		t.Error("StoreSpanContent mismatch")
	}
}

func TestOrgService_UpdatePrivacySettings_InvalidJSON(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-bad@example.com", "Priv Bad")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv Bad Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	err = svc.UpdatePrivacySettings(ctx, org.ID, true, json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_masking_config" {
		t.Errorf("expected invalid_masking_config, got %+v", err)
	}
}

func TestOrgService_UpdatePrivacySettings_InvalidMode(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-mode@example.com", "Priv M")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv M Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	err = svc.UpdatePrivacySettings(ctx, org.ID, true, json.RawMessage(`{"mode":"invalid_mode","rules":[]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_masking_mode" {
		t.Errorf("expected invalid_masking_mode, got %+v", err)
	}
}

func TestOrgService_UpdatePrivacySettings_BadRegex(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-rx@example.com", "Priv RX")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv RX Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	err = svc.UpdatePrivacySettings(ctx, org.ID, true, json.RawMessage(`{"mode":"llm_only","rules":[{"name":"bad","pattern":"["}]}`))
	if err == nil {
		t.Fatal("expected error for bad regex")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_rule_pattern" {
		t.Errorf("expected invalid_rule_pattern, got %+v", err)
	}
}

func TestOrgService_UpdatePrivacySettings_EmptyName(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-en@example.com", "Priv EN")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv EN Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	err = svc.UpdatePrivacySettings(ctx, org.ID, true, json.RawMessage(`{"mode":"llm_only","rules":[{"name":"","pattern":"x"}]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_rule" {
		t.Errorf("expected invalid_rule, got %+v", err)
	}
}

func TestOrgService_UpdatePrivacySettings_TooManyRules(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-many@example.com", "Priv M")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv M Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Build 21 rules
	rules := `{"mode":"llm_only","rules":[`
	for i := 0; i < 21; i++ {
		if i > 0 {
			rules += ","
		}
		rules += `{"name":"r","pattern":"x"}`
	}
	rules += `]}`

	err = svc.UpdatePrivacySettings(ctx, org.ID, true, json.RawMessage(rules))
	if err == nil {
		t.Fatal("expected error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "too_many_rules" {
		t.Errorf("expected too_many_rules, got %+v", err)
	}
}

func TestOrgService_UpdatePrivacySettings_StoreFalseForcesMaskingOff(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "privacy-off@example.com", "Priv Off")
	org, err := svc.CreateOrganization(ctx, user.ID, "Priv Off Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// storeSpanContent=false with masking should force mode=off
	cfg := json.RawMessage(`{"mode":"llm_only","rules":[]}`)
	if err := svc.UpdatePrivacySettings(ctx, org.ID, false, cfg); err != nil {
		t.Fatalf("UpdatePrivacySettings: %v", err)
	}
	settings, err := svc.GetPrivacySettings(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPrivacySettings: %v", err)
	}
	if settings.StoreSpanContent {
		t.Error("expected StoreSpanContent=false")
	}
}

func TestOrgService_GetSpanMaskingMaps_Empty(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	// No masking maps exist for random UUID
	maps, err := svc.GetSpanMaskingMaps(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetSpanMaskingMaps: %v", err)
	}
	if len(maps) != 0 {
		t.Errorf("expected 0 maps, got %d", len(maps))
	}
	_ = svc
}

func TestOrgService_RunHardDeleteCron_Empty(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	// No orgs pending deletion - should be a no-op
	if err := svc.RunHardDeleteCron(ctx); err != nil {
		t.Fatalf("RunHardDeleteCron (empty): %v", err)
	}
}

func TestOrgService_RunHardDeleteCron_DeletesExpired(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "hard-del@example.com", "Del User")
	org, err := svc.CreateOrganization(ctx, user.ID, "To Delete Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Force deletion_scheduled_at into the past
	past := time.Now().Add(-1 * time.Hour)
	_, err = sharedPool.Exec(ctx, "UPDATE organizations SET deletion_scheduled_at=$1, status='pending_deletion' WHERE id=$2", past, org.ID)
	if err != nil {
		t.Fatalf("update org: %v", err)
	}

	if err := svc.RunHardDeleteCron(ctx); err != nil {
		t.Fatalf("RunHardDeleteCron: %v", err)
	}

	// Org should now be gone
	_, err = sharedQueries.GetOrganizationByID(ctx, org.ID)
	if err == nil {
		t.Error("expected org to be deleted")
	}
}

func TestOrgService_GetOrganization_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	_, err := svc.GetOrganization(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected not found")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 404 {
		t.Errorf("expected 404, got %d", svcErr.Status)
	}
}

func TestOrgService_UpdateSettings_InvalidLocale(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "ul@example.com", "UL")
	org, err := svc.CreateOrganization(ctx, user.ID, "UL Org")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateSettings(ctx, org.ID, "xx", 60)
	if err == nil {
		t.Fatal("expected error for invalid locale")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_locale" {
		t.Errorf("expected invalid_locale, got %+v", err)
	}
}

func TestOrgService_UpdateSettings_InvalidTimeout(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	svc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")

	user := createTestUser(t, ctx, sharedQueries, "ut@example.com", "UT")
	org, err := svc.CreateOrganization(ctx, user.ID, "UT Org")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateSettings(ctx, org.ID, "en", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	svcErr, _ := err.(*service.ServiceError)
	if svcErr == nil || svcErr.Code != "invalid_timeout" {
		t.Errorf("expected invalid_timeout, got %+v", err)
	}

	err = svc.UpdateSettings(ctx, org.ID, "en", 99999)
	if err == nil {
		t.Fatal("expected error for large timeout")
	}
}

