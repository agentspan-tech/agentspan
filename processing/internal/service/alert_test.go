//go:build integration

package service_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentspan/processing/internal/hub"
	"github.com/agentspan/processing/internal/service"
	"github.com/agentspan/processing/internal/testutil"
	"github.com/google/uuid"
)

func TestAlertService_CreateRule(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	// Create a self_host org (alerts require non-free plan)
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-create@example.com", "Alert User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)
	rule, err := alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "High Error Rate",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	if rule.Name != "High Error Rate" {
		t.Errorf("expected name 'High Error Rate', got '%s'", rule.Name)
	}
	if rule.AlertType != "failure_rate" {
		t.Errorf("expected type 'failure_rate', got '%s'", rule.AlertType)
	}
	if !rule.Enabled {
		t.Error("expected rule to be enabled by default")
	}
}

func TestAlertService_TierGating(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	// Create a free org
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "alert-free@example.com", "Free User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Free Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Blocked Rule",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err == nil {
		t.Fatal("expected error for free tier, got nil")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if svcErr.Status != 403 {
		t.Errorf("expected status 403, got %d", svcErr.Status)
	}
}

func TestAlertService_ListRules(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-list@example.com", "List User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert List Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.8)
	windowMin := int32(30)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Rule 1",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	rules, err := alertSvc.List(ctx, org.ID, org.Plan)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestAlertService_CreateRule_LimitEnforced(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-limit@example.com", "Limit User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert Limit Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)

	// Create 20 rules (the max).
	for i := 0; i < 20; i++ {
		_, err := alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
			Name:          fmt.Sprintf("Rule %d", i+1),
			AlertType:     "failure_rate",
			Threshold:     &threshold,
			WindowMinutes: &windowMin,
		})
		if err != nil {
			t.Fatalf("create rule %d: %v", i+1, err)
		}
	}

	// The 21st should fail.
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Rule 21",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err == nil {
		t.Fatal("expected limit error, got nil")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if svcErr.Code != "alert_limit_reached" {
		t.Errorf("expected code 'alert_limit_reached', got %q", svcErr.Code)
	}
}

func TestAlertService_RunEvaluationCron_NoOrgs(t *testing.T) {
	truncate(t)
	queries := sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	// With empty DB, RunEvaluationCron should complete without error.
	err := alertSvc.RunEvaluationCron(ctx)
	if err != nil {
		t.Fatalf("RunEvaluationCron with no orgs: %v", err)
	}
}

func TestAlertService_RunEvaluationCron_WithRulesNoData(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	// Create a non-free org with alert rules but no span data.
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-cron@example.com", "Cron User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert Cron Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Failure Rate Rule",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	latencyThreshold := float64(1000)
	latencyWindow := int32(30)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Latency Rule",
		AlertType:     "anomalous_latency",
		Threshold:     &latencyThreshold,
		WindowMinutes: &latencyWindow,
	})
	if err != nil {
		t.Fatalf("create latency rule: %v", err)
	}

	spikeThreshold := float64(10)
	spikeWindow := int32(15)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "Error Spike Rule",
		AlertType:     "error_spike",
		Threshold:     &spikeThreshold,
		WindowMinutes: &spikeWindow,
	})
	if err != nil {
		t.Fatalf("create spike rule: %v", err)
	}

	// Run evaluation — should succeed without triggering any alerts (no span data).
	err = alertSvc.RunEvaluationCron(ctx)
	if err != nil {
		t.Fatalf("RunEvaluationCron: %v", err)
	}
}

func TestAlertService_StartReactiveSubscription(t *testing.T) {
	truncate(t)
	queries := sharedQueries
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	ctx, cancel := context.WithCancel(context.Background())
	alertSvc.StartReactiveSubscription(ctx)

	// Publish an event with invalid payload — should be handled gracefully.
	h.Publish(uuid.Nil, "failure_cluster_created", hub.Event{Type: "failure_cluster_created", Payload: "invalid-payload"})

	// Small sleep to let the goroutine process the event.
	time.Sleep(50 * time.Millisecond)

	// Cancel to stop the subscription goroutine.
	cancel()
}

func TestAlertService_ReactiveSubscription_ValidPayload(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, pool, h, mailer, "http://localhost:3000")

	// Create a self_host org with a new_failure_cluster alert rule.
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "reactive@example.com", "Reactive User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Reactive Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:      "New Failure Cluster Alert",
		AlertType: "new_failure_cluster",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	alertSvc.StartReactiveSubscription(subCtx)

	// Publish a properly structured event.
	h.Publish(uuid.Nil, "failure_cluster_created", hub.Event{
		Type: "failure_cluster_created",
		Payload: map[string]interface{}{
			"organization_id": org.ID,
			"cluster_id":      uuid.New(),
			"label":           "HTTP 429: rate limited",
		},
	})

	// Let the goroutine process.
	time.Sleep(100 * time.Millisecond)
	cancel()
}

func TestAlertService_RunEvaluationCron_WithData(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, pool, h, mailer, "http://localhost:3000")

	// Create self_host org.
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "cron-data@example.com", "Cron Data")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Cron Data Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create rules of each type.
	threshold := float64(0.1) // very low threshold so it triggers
	windowMin := int32(60)
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name: "FR Rule", AlertType: "failure_rate",
		Threshold: &threshold, WindowMinutes: &windowMin,
		NotifyRoles: []string{"owner"},
	})
	if err != nil {
		t.Fatalf("create FR rule: %v", err)
	}

	latThreshold := float64(1.0) // 1ms — everything triggers
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name: "Lat Rule", AlertType: "anomalous_latency",
		Threshold: &latThreshold, WindowMinutes: &windowMin,
		NotifyRoles: []string{"owner"},
	})
	if err != nil {
		t.Fatalf("create lat rule: %v", err)
	}

	spikeThreshold := float64(0.0) // 0 = everything triggers
	_, err = alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name: "Spike Rule", AlertType: "error_spike",
		Threshold: &spikeThreshold, WindowMinutes: &windowMin,
		NotifyRoles: []string{"owner"},
	})
	if err != nil {
		t.Fatalf("create spike rule: %v", err)
	}

	// Ingest some spans so there's data for evaluation.
	encKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hmacSecret := "test-hmac-secret-32chars-minimum!"
	apiKeySvc := service.NewAPIKeyService(queries, hmacSecret, encKey)
	apiKeyResult, _ := apiKeySvc.CreateAPIKey(ctx, org.ID, "Cron Agent", "openai", "sk-cron", nil)

	internalSvc := service.NewInternalService(queries, pool, hmacSecret, encKey, h)
	for i := 0; i < 5; i++ {
		status := int32(200)
		if i%2 == 0 {
			status = 500 // some errors
		}
		_ = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "test input",
			Output:         "test output",
			InputTokens:    10,
			OutputTokens:   5,
			DurationMs:     100,
			HTTPStatus:     status,
			StartedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
	}

	// Run evaluation cron — should evaluate rules against real span data.
	err = alertSvc.RunEvaluationCron(ctx)
	if err != nil {
		t.Fatalf("RunEvaluationCron with data: %v", err)
	}

	// Check if any alerts were generated.
	events, err := alertSvc.ListEvents(ctx, org.ID, org.Plan, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	// With such low thresholds and error data, some alerts should have fired.
	t.Logf("alert events fired: %d", len(events))
}

func TestAlertService_GetUpdateDelete(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-crud@example.com", "CRUD User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert CRUD Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)
	rule, err := alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
		Name:          "CRUD Rule",
		AlertType:     "failure_rate",
		Threshold:     &threshold,
		WindowMinutes: &windowMin,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get
	got, err := alertSvc.Get(ctx, org.ID, org.Plan, rule.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "CRUD Rule" {
		t.Errorf("get: name = %q, want 'CRUD Rule'", got.Name)
	}

	// Update
	newName := "Updated Rule"
	enabled := false
	updated, err := alertSvc.Update(ctx, org.ID, org.Plan, rule.ID, service.UpdateAlertRuleRequest{
		Name:    &newName,
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Updated Rule" {
		t.Errorf("update: name = %q", updated.Name)
	}
	if updated.Enabled {
		t.Error("update: expected enabled=false")
	}

	// List events (empty)
	events, err := alertSvc.ListEvents(ctx, org.ID, org.Plan, 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}

	// Delete
	err = alertSvc.Delete(ctx, org.ID, org.Plan, rule.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestAlertService_CreateRule_ConcurrentLimit(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()
	alertSvc := service.NewAlertService(queries, sharedPool, h, mailer, "http://localhost:3000")

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "alert-concurrent@example.com", "Concurrent User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Alert Concurrent Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threshold := float64(0.5)
	windowMin := int32(60)

	// Launch 25 goroutines each creating a rule. Exactly 20 should succeed.
	var wg sync.WaitGroup
	var succeeded atomic.Int64

	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := alertSvc.Create(ctx, org.ID, org.Plan, service.CreateAlertRuleRequest{
				Name:          fmt.Sprintf("Concurrent Rule %d", idx),
				AlertType:     "failure_rate",
				Threshold:     &threshold,
				WindowMinutes: &windowMin,
			})
			if err == nil {
				succeeded.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if succeeded.Load() != 20 {
		t.Errorf("expected exactly 20 successful rule creations, got %d", succeeded.Load())
	}
}
