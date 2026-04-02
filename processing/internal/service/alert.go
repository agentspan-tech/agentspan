package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/agentspan/processing/internal/db"
	"github.com/agentspan/processing/internal/email"
	"github.com/agentspan/processing/internal/hub"
	"github.com/agentspan/processing/internal/txutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// validAlertTypes defines the set of allowed alert_type values.
var validAlertTypes = map[string]bool{
	"failure_rate":        true,
	"anomalous_latency":  true,
	"new_failure_cluster": true,
	"error_spike":         true,
}

// AlertService handles alert rule CRUD, cron-based evaluation, reactive subscription,
// tier gating, email delivery, and Hub integration.
type AlertService struct {
	queries    *db.Queries
	pool       *pgxpool.Pool
	hub        *hub.Hub
	mailer     email.Mailer
	appBaseURL string
}

// NewAlertService creates a new AlertService.
func NewAlertService(queries *db.Queries, pool *pgxpool.Pool, h *hub.Hub, mailer email.Mailer, appBaseURL string) *AlertService {
	return &AlertService{queries: queries, pool: pool, hub: h, mailer: mailer, appBaseURL: appBaseURL}
}

// checkTier returns a 403 ServiceError for free-plan organizations (D-11).
func (s *AlertService) checkTier(plan string) error {
	if plan == "free" {
		return &ServiceError{Status: 403, Code: "tier_restricted", Message: "Alerts require Pro or Self-host plan"}
	}
	return nil
}

// CreateAlertRuleRequest is the input for creating a new alert rule.
type CreateAlertRuleRequest struct {
	Name            string   `json:"name"`
	AlertType       string   `json:"alert_type"`
	Threshold       *float64 `json:"threshold"`
	WindowMinutes   *int32   `json:"window_minutes"`
	CooldownMinutes *int32   `json:"cooldown_minutes"`
	NotifyRoles     []string `json:"notify_roles"`
	Enabled         *bool    `json:"enabled"`
}

// UpdateAlertRuleRequest is the input for updating an existing alert rule.
type UpdateAlertRuleRequest struct {
	Name            *string  `json:"name"`
	Threshold       *float64 `json:"threshold"`
	WindowMinutes   *int32   `json:"window_minutes"`
	CooldownMinutes *int32   `json:"cooldown_minutes"`
	NotifyRoles     []string `json:"notify_roles"`
	Enabled         *bool    `json:"enabled"`
}

// Create creates a new alert rule (ALRT-01, ALRT-07, ALRT-08).
func (s *AlertService) Create(ctx context.Context, orgID uuid.UUID, plan string, req CreateAlertRuleRequest) (db.AlertRule, error) {
	if err := s.checkTier(plan); err != nil {
		return db.AlertRule{}, err
	}

	// Validate alert_type.
	if !validAlertTypes[req.AlertType] {
		return db.AlertRule{}, &ServiceError{
			Status:  400,
			Code:    "invalid_alert_type",
			Message: "alert_type must be one of: failure_rate, anomalous_latency, new_failure_cluster, error_spike",
		}
	}

	// Validate name.
	if req.Name == "" {
		return db.AlertRule{}, &ServiceError{
			Status:  400,
			Code:    "invalid_name",
			Message: "name is required",
		}
	}

	// For cron-based types, threshold and window_minutes are required.
	// For new_failure_cluster (reactive, event-driven), they default to 0/nil — not evaluated.
	if req.AlertType == "new_failure_cluster" {
		// Reactive alert: threshold/window_minutes are not used, set explicit defaults.
		if req.Threshold == nil {
			zero := float64(0)
			req.Threshold = &zero
		}
		if req.WindowMinutes == nil {
			zero := int32(0)
			req.WindowMinutes = &zero
		}
	} else {
		if req.Threshold == nil {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_threshold",
				Message: "threshold is required for " + req.AlertType,
			}
		}
		if *req.Threshold < 0 || *req.Threshold > 1e9 {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_threshold",
				Message: "threshold must be between 0 and 1,000,000,000",
			}
		}
		if req.WindowMinutes == nil {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_window_minutes",
				Message: "window_minutes is required for " + req.AlertType,
			}
		}
		if *req.WindowMinutes < 1 || *req.WindowMinutes > 1440 {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_window_minutes",
				Message: "window_minutes must be between 1 and 1440",
			}
		}
	}

	// Validate cooldown_minutes: 15-1440 if provided, default 60 (ALRT-04).
	cooldownMinutes := int32(60)
	if req.CooldownMinutes != nil {
		cooldownMinutes = *req.CooldownMinutes
		if cooldownMinutes < 15 || cooldownMinutes > 1440 {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_cooldown_minutes",
				Message: "cooldown_minutes must be between 15 and 1440",
			}
		}
	}

	// Convert threshold to pgtype.Numeric.
	threshold := pgtype.Numeric{}
	if req.Threshold != nil {
		threshold = float64ToNumeric(*req.Threshold)
	}

	// Default enabled to true.
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Default notify_roles to empty slice.
	notifyRoles := req.NotifyRoles
	if notifyRoles == nil {
		notifyRoles = []string{}
	}

	windowMinutes := int32(0)
	if req.WindowMinutes != nil {
		windowMinutes = *req.WindowMinutes
	}

	// Check alert rule limit + create in transaction with FOR UPDATE to prevent TOCTOU race.
	var rule db.AlertRule
	err := txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		// Lock org row to serialize concurrent rule creations.
		if _, err := q.GetOrganizationByIDForUpdate(ctx, orgID); err != nil {
			return fmt.Errorf("create alert rule: lock org: %w", err)
		}

		count, err := q.CountAlertRules(ctx, orgID)
		if err != nil {
			return fmt.Errorf("create alert rule: count rules: %w", err)
		}
		if count >= 20 {
			return &ServiceError{
				Status:  422,
				Code:    "alert_limit_reached",
				Message: "Maximum 20 alert rules per organization",
			}
		}

		rule, err = q.CreateAlertRule(ctx, db.CreateAlertRuleParams{
			OrganizationID:  orgID,
			Name:            req.Name,
			AlertType:       req.AlertType,
			Threshold:       threshold,
			WindowMinutes:   windowMinutes,
			CooldownMinutes: cooldownMinutes,
			NotifyRoles:     notifyRoles,
			Enabled:         enabled,
		})
		if err != nil {
			return fmt.Errorf("create alert rule: %w", err)
		}
		return nil
	})
	if err != nil {
		return db.AlertRule{}, err
	}

	return rule, nil
}

// List returns all alert rules for an organization.
func (s *AlertService) List(ctx context.Context, orgID uuid.UUID, plan string) ([]db.AlertRule, error) {
	if err := s.checkTier(plan); err != nil {
		return nil, err
	}
	return s.queries.ListAlertRules(ctx, orgID)
}

// Get returns a single alert rule by ID.
func (s *AlertService) Get(ctx context.Context, orgID uuid.UUID, plan string, alertID uuid.UUID) (db.AlertRule, error) {
	if err := s.checkTier(plan); err != nil {
		return db.AlertRule{}, err
	}
	rule, err := s.queries.GetAlertRule(ctx, db.GetAlertRuleParams{
		ID:             alertID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AlertRule{}, &ServiceError{Status: 404, Code: "not_found", Message: "Alert rule not found"}
		}
		return db.AlertRule{}, fmt.Errorf("get alert rule: %w", err)
	}
	return rule, nil
}

// Update modifies an existing alert rule (ALRT-01).
func (s *AlertService) Update(ctx context.Context, orgID uuid.UUID, plan string, alertID uuid.UUID, req UpdateAlertRuleRequest) (db.AlertRule, error) {
	if err := s.checkTier(plan); err != nil {
		return db.AlertRule{}, err
	}

	// Fetch existing rule to merge updates.
	existing, err := s.queries.GetAlertRule(ctx, db.GetAlertRuleParams{
		ID:             alertID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AlertRule{}, &ServiceError{Status: 404, Code: "not_found", Message: "Alert rule not found"}
		}
		return db.AlertRule{}, fmt.Errorf("update alert rule: get existing: %w", err)
	}

	// Merge fields.
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
		if name == "" {
			return db.AlertRule{}, &ServiceError{Status: 400, Code: "invalid_name", Message: "name cannot be empty"}
		}
	}

	threshold := existing.Threshold
	if req.Threshold != nil {
		threshold = float64ToNumeric(*req.Threshold)
	}

	windowMinutes := existing.WindowMinutes
	if req.WindowMinutes != nil {
		windowMinutes = *req.WindowMinutes
	}

	cooldownMinutes := existing.CooldownMinutes
	if req.CooldownMinutes != nil {
		cooldownMinutes = *req.CooldownMinutes
		if cooldownMinutes < 15 || cooldownMinutes > 1440 {
			return db.AlertRule{}, &ServiceError{
				Status:  400,
				Code:    "invalid_cooldown_minutes",
				Message: "cooldown_minutes must be between 15 and 1440",
			}
		}
	}

	notifyRoles := existing.NotifyRoles
	if req.NotifyRoles != nil {
		notifyRoles = req.NotifyRoles
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule, err := s.queries.UpdateAlertRule(ctx, db.UpdateAlertRuleParams{
		ID:              alertID,
		OrganizationID:  orgID,
		Name:            name,
		Threshold:       threshold,
		WindowMinutes:   windowMinutes,
		CooldownMinutes: cooldownMinutes,
		NotifyRoles:     notifyRoles,
		Enabled:         enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AlertRule{}, &ServiceError{Status: 404, Code: "not_found", Message: "Alert rule not found"}
		}
		return db.AlertRule{}, fmt.Errorf("update alert rule: %w", err)
	}
	return rule, nil
}

// Delete removes an alert rule (ALRT-01).
func (s *AlertService) Delete(ctx context.Context, orgID uuid.UUID, plan string, alertID uuid.UUID) error {
	if err := s.checkTier(plan); err != nil {
		return err
	}
	return s.queries.DeleteAlertRule(ctx, db.DeleteAlertRuleParams{
		ID:             alertID,
		OrganizationID: orgID,
	})
}

// ListEvents returns recent alert events for an organization.
func (s *AlertService) ListEvents(ctx context.Context, orgID uuid.UUID, plan string, limit int32) ([]db.AlertEvent, error) {
	if err := s.checkTier(plan); err != nil {
		return nil, err
	}
	return s.queries.ListAlertEvents(ctx, db.ListAlertEventsParams{
		OrganizationID: orgID,
		Limit:          limit,
	})
}

// alertEventPayload is the JSON payload stored with each alert event.
type alertEventPayload struct {
	AlertType    string  `json:"alert_type"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	WindowMin    int32   `json:"window_minutes"`
}

// RunEvaluationCron evaluates all enabled alert rules for non-free orgs (ALRT-02, ALRT-04, ALRT-05, ALRT-06).
// Rules are grouped by organization to batch evaluation queries (one set of metric queries per org).
func (s *AlertService) RunEvaluationCron(ctx context.Context) error {
	orgIDs, err := s.queries.ListNonFreeOrgIDs(ctx)
	if err != nil {
		return fmt.Errorf("alert evaluation: list org ids: %w", err)
	}

	var totalRules, triggeredCount int
	for _, orgID := range orgIDs {
		rules, err := s.queries.GetEnabledAlertRulesByOrg(ctx, orgID)
		if err != nil {
			slog.Error("alert-evaluation: get rules failed", "org", orgID, "error", err)
			continue
		}

		// Determine max window per metric type to batch evaluation queries per org.
		var maxWindowFailureRate, maxWindowLatency, maxWindowErrorSpike int32
		for _, rule := range rules {
			if rule.AlertType == "new_failure_cluster" {
				continue
			}
			w := rule.WindowMinutes
			if w <= 0 {
				continue
			}
			switch rule.AlertType {
			case "failure_rate":
				if w > maxWindowFailureRate {
					maxWindowFailureRate = w
				}
			case "anomalous_latency":
				if w > maxWindowLatency {
					maxWindowLatency = w
				}
			case "error_spike":
				if w > maxWindowErrorSpike {
					maxWindowErrorSpike = w
				}
			}
		}

		// Pre-fetch metrics per org (one query per metric type that has rules).
		type failureRateData struct {
			totalCount int64
			errorCount int64
		}
		var frData *failureRateData
		var latencyData *float64
		var errorSpikeData *int64

		if maxWindowFailureRate > 0 {
			row, err := s.queries.EvaluateFailureRate(ctx, db.EvaluateFailureRateParams{
				OrganizationID: orgID,
				WindowMinutes:  maxWindowFailureRate,
			})
			if err != nil {
				slog.Error("alert-evaluation: failure_rate error", "org", orgID, "error", err)
			} else {
				frData = &failureRateData{totalCount: row.TotalCount, errorCount: row.ErrorCount}
			}
		}
		if maxWindowLatency > 0 {
			avgMs, err := s.queries.EvaluateAvgLatency(ctx, db.EvaluateAvgLatencyParams{
				OrganizationID: orgID,
				WindowMinutes:  maxWindowLatency,
			})
			if err != nil {
				slog.Error("alert-evaluation: anomalous_latency error", "org", orgID, "error", err)
			} else {
				latencyData = &avgMs
			}
		}
		if maxWindowErrorSpike > 0 {
			count, err := s.queries.EvaluateErrorSpike(ctx, db.EvaluateErrorSpikeParams{
				OrganizationID: orgID,
				WindowMinutes:  maxWindowErrorSpike,
			})
			if err != nil {
				slog.Error("alert-evaluation: error_spike error", "org", orgID, "error", err)
			} else {
				errorSpikeData = &count
			}
		}

		// Collect all roles that need email notifications for deduplication.
		type triggeredRule struct {
			rule         db.AlertRule
			currentValue float64
			threshold    float64
		}
		var triggered []triggeredRule

		for _, rule := range rules {
			totalRules++

			if rule.AlertType == "new_failure_cluster" {
				continue
			}

			windowMinutes := rule.WindowMinutes
			if windowMinutes <= 0 {
				continue
			}

			thresholdFloat := numericToFloat64(rule.Threshold)
			var currentValue float64
			var fired bool

			switch rule.AlertType {
			case "failure_rate":
				if frData == nil || frData.totalCount == 0 {
					continue
				}
				currentValue = float64(frData.errorCount) / float64(frData.totalCount)
				fired = currentValue >= thresholdFloat

			case "anomalous_latency":
				if latencyData == nil {
					continue
				}
				currentValue = *latencyData
				fired = currentValue >= thresholdFloat

			case "error_spike":
				if errorSpikeData == nil {
					continue
				}
				currentValue = float64(*errorSpikeData)
				fired = currentValue >= thresholdFloat
			}

			if !fired {
				continue
			}

			// Atomic cooldown check (ALRT-04).
			_, err := s.queries.TryTriggerAlert(ctx, rule.ID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				slog.Error("alert-evaluation: try trigger error", "rule", rule.ID, "error", err)
				continue
			}

			triggeredCount++

			payload := alertEventPayload{
				AlertType:    rule.AlertType,
				CurrentValue: currentValue,
				Threshold:    thresholdFloat,
				WindowMin:    windowMinutes,
			}
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				slog.Error("alert-evaluation: marshal payload error", "rule", rule.ID, "error", err)
				continue
			}

			alertEvent, err := s.queries.InsertAlertEvent(ctx, db.InsertAlertEventParams{
				OrganizationID: orgID,
				AlertRuleID:    rule.ID,
				Payload:        payloadJSON,
			})
			if err != nil {
				slog.Error("alert-evaluation: insert event error", "rule", rule.ID, "error", err)
				continue
			}

			s.hub.Publish(orgID, "sessions_list:"+orgID.String(), hub.Event{
				Type:    "alert.triggered",
				Payload: alertEvent,
			})

			triggered = append(triggered, triggeredRule{rule: rule, currentValue: currentValue, threshold: thresholdFloat})
		}

		// Send email notifications for all triggered rules in this org.
		for _, tr := range triggered {
			s.sendAlertEmails(ctx, tr.rule, tr.currentValue, tr.threshold)
		}
	}

	slog.Info("alert-evaluation complete", "rules_evaluated", totalRules, "triggered", triggeredCount)
	return nil
}

// sendAlertEmails sends alert notification emails to users matching notify_roles (ALRT-05, D-09).
func (s *AlertService) sendAlertEmails(ctx context.Context, rule db.AlertRule, currentValue, threshold float64) {
	if len(rule.NotifyRoles) == 0 {
		return
	}

	recipients, err := s.queries.GetMemberEmailsByRoles(ctx, db.GetMemberEmailsByRolesParams{
		OrganizationID: rule.OrganizationID,
		Roles:          rule.NotifyRoles,
	})
	if err != nil {
		slog.Error("alert-email: get recipients error", "rule", rule.ID, "error", err)
		return
	}

	dashboardLink := s.appBaseURL + "/orgs/" + rule.OrganizationID.String() + "/alerts"
	currentValueStr := fmt.Sprintf("%.4f", currentValue)
	thresholdStr := fmt.Sprintf("%.4f", threshold)

	for _, r := range recipients {
		if err := s.mailer.SendAlert(r.Email, r.Name, rule.Name, rule.AlertType, currentValueStr, thresholdStr, dashboardLink, "en"); err != nil {
			slog.Error("alert-email: send error", "rule", rule.ID, "error", err)
		}
	}
}

// StartReactiveSubscription subscribes to failure_cluster_created events for reactive alert
// triggering (ALRT-03, D-07). Phase 7 will publish to this topic when creating clusters.
func (s *AlertService) StartReactiveSubscription(ctx context.Context) {
	sub := s.hub.Subscribe(uuid.Nil, "failure_cluster_created")
	go func() {
		defer s.hub.Unsubscribe(sub)
		for {
			select {
			case event, ok := <-sub.Ch:
				if !ok {
					return
				}
				s.handleReactiveAlert(ctx, event)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// handleReactiveAlert processes a failure_cluster_created event from the hub.
// It queries enabled new_failure_cluster rules for the event's org, checks cooldown,
// creates alert events, publishes via Hub, and sends email notifications (ALRT-03).
func (s *AlertService) handleReactiveAlert(ctx context.Context, event hub.Event) {
	payload, ok := event.Payload.(map[string]interface{})
	if !ok {
		slog.Warn("alert-reactive: unexpected payload type", "type", fmt.Sprintf("%T", event.Payload))
		return
	}

	orgID, ok := payload["organization_id"].(uuid.UUID)
	if !ok {
		slog.Warn("alert-reactive: missing or invalid organization_id in payload")
		return
	}

	label, _ := payload["label"].(string)
	clusterID, _ := payload["cluster_id"].(uuid.UUID)

	rules, err := s.queries.GetEnabledAlertRulesByOrgAndType(ctx, db.GetEnabledAlertRulesByOrgAndTypeParams{
		OrganizationID: orgID,
		AlertType:      "new_failure_cluster",
	})
	if err != nil {
		slog.Error("alert-reactive: get rules error", "org", orgID, "error", err)
		return
	}

	triggeredCount := 0
	for _, rule := range rules {
		// Atomic cooldown check (ALRT-04).
		_, err := s.queries.TryTriggerAlert(ctx, rule.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Cooldown still active — skip.
				continue
			}
			slog.Error("alert-reactive: try trigger error", "rule", rule.ID, "error", err)
			continue
		}

		// Build event payload.
		type reactivePayload struct {
			AlertType    string    `json:"alert_type"`
			ClusterLabel string    `json:"cluster_label"`
			ClusterID    uuid.UUID `json:"cluster_id"`
		}
		payloadJSON, err := json.Marshal(reactivePayload{
			AlertType:    "new_failure_cluster",
			ClusterLabel: label,
			ClusterID:    clusterID,
		})
		if err != nil {
			slog.Error("reactive-alert: marshal payload error", "cluster", clusterID, "error", err)
			continue
		}

		alertEvent, err := s.queries.InsertAlertEvent(ctx, db.InsertAlertEventParams{
			OrganizationID: orgID,
			AlertRuleID:    rule.ID,
			Payload:        payloadJSON,
		})
		if err != nil {
			slog.Error("alert-reactive: insert event error", "rule", rule.ID, "error", err)
			continue
		}

		triggeredCount++

		// Broadcast via Hub (ALRT-06).
		s.hub.Publish(orgID, "sessions_list:"+orgID.String(), hub.Event{
			Type:    "alert.triggered",
			Payload: alertEvent,
		})

		// Send email notifications (ALRT-05).
		s.sendAlertEmails(ctx, rule, 1.0, 0.0)
	}

	slog.Info("alert-reactive complete", "org", orgID, "rules_processed", len(rules), "triggered", triggeredCount)
}

// float64ToNumeric converts a float64 to pgtype.Numeric.
func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}
