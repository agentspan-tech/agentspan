package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/crypto"
	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/txutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultBaseURLs maps provider_type to the default upstream base URL.
var defaultBaseURLs = map[string]string{
	"openai":    "https://api.openai.com/v1",
	"anthropic": "https://api.anthropic.com/v1",
	"deepseek":  "https://api.deepseek.com/v1",
	"mistral":   "https://api.mistral.ai/v1",
	"groq":      "https://api.groq.com/openai/v1",
	"gemini":    "https://generativelanguage.googleapis.com/v1",
}

// AuthVerifyResult is the response from VerifyAPIKey. The Proxy caches this.
type AuthVerifyResult struct {
	Valid              bool             `json:"valid"`
	Reason             string           `json:"reason,omitempty"`
	APIKeyID           string           `json:"api_key_id,omitempty"`
	OrganizationID     string           `json:"organization_id,omitempty"`
	ProviderType       string           `json:"provider_type,omitempty"`
	ProviderKey        string           `json:"provider_key,omitempty"`
	BaseURL            string           `json:"base_url,omitempty"`
	OrganizationStatus string           `json:"organization_status,omitempty"`
	StoreSpanContent   bool             `json:"store_span_content"`
	MaskingConfig      json.RawMessage  `json:"masking_config,omitempty"`
}

// MaskingMapEntry represents a single masking replacement for span ingestion audit trail.
type MaskingMapEntry struct {
	MaskType      string `json:"mask_type"`
	OriginalValue string `json:"original_value"`
	MaskedValue   string `json:"masked_value"`
}

// SpanIngestRequest is the body accepted by POST /internal/spans/ingest.
type SpanIngestRequest struct {
	APIKeyID          string            `json:"api_key_id"`
	OrganizationID    string            `json:"organization_id"`
	ProviderType      string            `json:"provider_type"`
	Model             string            `json:"model"`
	Input             string            `json:"input"`
	Output            string            `json:"output"`
	InputTokens       int32             `json:"input_tokens"`
	OutputTokens      int32             `json:"output_tokens"`
	DurationMs        int32             `json:"duration_ms"`
	HTTPStatus        int32             `json:"http_status"`
	StartedAt         string            `json:"started_at"`
	FinishReason      string            `json:"finish_reason,omitempty"`
	ExternalSessionID string            `json:"external_session_id,omitempty"`
	AgentName         string            `json:"agent_name,omitempty"`
	MaskingApplied     bool              `json:"masking_applied"`
	MaskingMap         []MaskingMapEntry `json:"masking_map,omitempty"`
	ClientDisconnected bool              `json:"client_disconnected"`
}

// SpanQuotaExceededError is returned when a free-plan organization has reached the
// 3000 spans/month limit (ORG-12, D-20, D-21).
type SpanQuotaExceededError struct{}

func (e *SpanQuotaExceededError) Error() string { return "span quota exceeded" }

// SpanEvent is the real-time event payload for span.created (WS-03, SEC-03).
// Contains metrics only — Input/Output are explicitly excluded to prevent
// sensitive LLM I/O from leaking over WebSocket.
type SpanEvent struct {
	ID           uuid.UUID `json:"id"`
	SessionID    uuid.UUID `json:"session_id"`
	ProviderType string    `json:"provider_type"`
	Model        string    `json:"model"`
	InputTokens  *int32    `json:"input_tokens"`
	OutputTokens *int32    `json:"output_tokens"`
	DurationMs   int32     `json:"duration_ms"`
	HttpStatus   int32     `json:"http_status"`
	StartedAt    string    `json:"started_at"`
}

// SessionEvent is the real-time event payload for session.created and session.updated (WS-02).
type SessionEvent struct {
	ID        uuid.UUID `json:"id"`
	Status    string    `json:"status"`
	AgentName *string   `json:"agent_name,omitempty"`
	SpanCount int32     `json:"span_count"`
}

// InternalService handles operations on the Internal API (Proxy -> Processing).
type InternalService struct {
	queries             *db.Queries
	pool                *pgxpool.Pool
	hmacSecret          string
	encryptionKey       string
	hub                 *hub.Hub            // may be nil (graceful degradation)
	intelligenceService *IntelligenceService // may be nil (no intelligence pipeline)
}

// NewInternalService creates a new InternalService.
func NewInternalService(queries *db.Queries, pool *pgxpool.Pool, hmacSecret, encryptionKey string, h *hub.Hub) *InternalService {
	return &InternalService{
		queries:       queries,
		pool:          pool,
		hmacSecret:    hmacSecret,
		encryptionKey: encryptionKey,
		hub:           h,
	}
}

// SetIntelligenceService injects the IntelligenceService into InternalService.
// Called after both services are created in main.go to avoid circular construction order.
func (s *InternalService) SetIntelligenceService(intSvc *IntelligenceService) {
	s.intelligenceService = intSvc
}

// VerifyAPIKey looks up an API key by its HMAC digest, decrypts the provider key,
// and returns the full configuration needed by the Proxy (D-14, AUTH-05).
//
// last_used_at is updated asynchronously — the update is fire-and-forget so it
// never blocks the verify response path (AKEY-05).
func (s *InternalService) VerifyAPIKey(ctx context.Context, keyDigest string) (*AuthVerifyResult, error) {
	apiKey, err := s.queries.GetApiKeyByDigest(ctx, keyDigest)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &AuthVerifyResult{Valid: false, Reason: "invalid_key"}, nil
		}
		return nil, fmt.Errorf("verify api key: lookup: %w", err)
	}

	org, err := s.queries.GetOrganizationByID(ctx, apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("verify api key: get org: %w", err)
	}

	if org.Status == "pending_deletion" {
		return &AuthVerifyResult{Valid: false, Reason: "org_pending_deletion"}, nil
	}

	providerKeyBytes, err := crypto.Decrypt(apiKey.ProviderKeyEncrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("verify api key: decrypt provider key: %w", err)
	}

	// Fire-and-forget: update last_used_at without blocking the response (AKEY-05).
	go func() {
		if err := s.queries.UpdateApiKeyLastUsed(context.Background(), apiKey.ID); err != nil {
			slog.Warn("failed to update api key last_used_at", "error", err)
		}
	}()

	baseURL := defaultBaseURLs[apiKey.ProviderType]
	if apiKey.BaseUrl != nil && *apiKey.BaseUrl != "" {
		baseURL = *apiKey.BaseUrl
	}

	return &AuthVerifyResult{
		Valid:              true,
		APIKeyID:           apiKey.ID.String(),
		OrganizationID:     apiKey.OrganizationID.String(),
		ProviderType:       apiKey.ProviderType,
		ProviderKey:        string(providerKeyBytes),
		BaseURL:            baseURL,
		OrganizationStatus: org.Status,
		StoreSpanContent:   org.StoreSpanContent,
		MaskingConfig:      json.RawMessage(org.MaskingConfig),
	}, nil
}

// maxSpanFieldSize is the maximum allowed size for span input/output fields (1MB).
const maxSpanFieldSize = 1024 * 1024

// FreeSpanLimit is the monthly span quota for free-plan organizations (ORG-12, D-20, D-21).
const FreeSpanLimit = 3000

// IngestSpan validates a span ingest request, enforces the free-plan 3000 spans/month quota,
// calculates cost, finds or creates a session, inserts the span, and updates session counters.
func (s *InternalService) IngestSpan(ctx context.Context, req *SpanIngestRequest) error {
	// 0. Enforce payload size limits to prevent database bloat.
	if len(req.Input) > maxSpanFieldSize {
		return &ServiceError{
			Status:  400,
			Code:    "input_too_large",
			Message: fmt.Sprintf("input field exceeds maximum size of %d bytes", maxSpanFieldSize),
		}
	}
	if len(req.Output) > maxSpanFieldSize {
		return &ServiceError{
			Status:  400,
			Code:    "output_too_large",
			Message: fmt.Sprintf("output field exceeds maximum size of %d bytes", maxSpanFieldSize),
		}
	}

	// 0b. Validate token counts to prevent overflow in cost calculations.
	if req.InputTokens < 0 || req.InputTokens > 10_000_000 {
		return &ServiceError{
			Status:  400,
			Code:    "invalid_input_tokens",
			Message: "input_tokens must be between 0 and 10000000",
		}
	}
	if req.OutputTokens < 0 || req.OutputTokens > 10_000_000 {
		return &ServiceError{
			Status:  400,
			Code:    "invalid_output_tokens",
			Message: "output_tokens must be between 0 and 10000000",
		}
	}

	// 1. Validate UUIDs.
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		return &ServiceError{
			Status:  400,
			Code:    "invalid_organization_id",
			Message: "organization_id must be a valid UUID",
		}
	}
	apiKeyID, err := uuid.Parse(req.APIKeyID)
	if err != nil {
		return &ServiceError{
			Status:  400,
			Code:    "invalid_api_key_id",
			Message: "api_key_id must be a valid UUID",
		}
	}

	// 2. Calculate cost from model_prices (SPAN-02, SPAN-03).
	// Done outside transaction — read-only lookup, no atomicity needed.
	costUsd := pgtype.Numeric{Valid: false} // NULL for unknown models
	priceRow, err := s.queries.GetModelPrice(ctx, db.GetModelPriceParams{
		ProviderType: req.ProviderType,
		Model:        req.Model,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ingest span: get model price: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("unknown model pricing, cost will be NULL", "provider", req.ProviderType, "model", req.Model)
	}
	if err == nil {
		costUsd = calculateCost(req.InputTokens, req.OutputTokens, priceRow.InputPricePerToken, priceRow.OutputPricePerToken)
	}

	// 3. Parse started_at.
	startedAt, err := time.Parse(time.RFC3339Nano, req.StartedAt)
	if err != nil {
		return &ServiceError{
			Status:  400,
			Code:    "invalid_started_at",
			Message: "started_at must be RFC3339 format",
		}
	}

	// 4–7. Quota check, session find/create, span insert, session update — all in one transaction.
	// FOR UPDATE on org row serializes concurrent free-plan inserts to prevent quota bypass.
	var spanID uuid.UUID
	var sessionID uuid.UUID
	newSession := false

	inputPtr := &req.Input
	outputPtr := &req.Output
	inputTokens := &req.InputTokens
	outputTokens := &req.OutputTokens
	finishReason := req.FinishReason
	if finishReason == "" {
		finishReason = "unknown"
	}

	err = txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		// 4. Check quota (free-plan 3000 spans/month) with row lock.
		org, err := q.GetOrganizationByIDForUpdate(ctx, orgID)
		if err != nil {
			return fmt.Errorf("ingest span: get org: %w", err)
		}

		if org.Plan == "free" {
			count, err := q.CountSpansThisMonth(ctx, orgID)
			if err != nil {
				return fmt.Errorf("ingest span: count spans: %w", err)
			}
			if count >= FreeSpanLimit {
				return &SpanQuotaExceededError{}
			}
		}

		// 5. Find or create session (SESS-01, SESS-02, SESS-03).
		if req.ExternalSessionID != "" {
			// Explicit session via X-AgentOrbit-Session header (SESS-03).
			var agentName *string
			if req.AgentName != "" {
				agentName = &req.AgentName
			}
			_, existsErr := q.FindActiveExplicitSession(ctx, db.FindActiveExplicitSessionParams{
				ApiKeyID:   apiKeyID,
				ExternalID: &req.ExternalSessionID,
			})
			sessionExisted := existsErr == nil
			sessionID, err = q.FindOrCreateExplicitSession(ctx, db.FindOrCreateExplicitSessionParams{
				OrganizationID: orgID,
				ApiKeyID:       apiKeyID,
				ExternalID:     &req.ExternalSessionID,
				AgentName:      agentName,
			})
			if err != nil {
				return fmt.Errorf("ingest span: find or create explicit session: %w", err)
			}
			if !sessionExisted {
				newSession = true
			}
		} else {
			// Auto-grouping by API key + timeout (SESS-02).
			timeoutSeconds, err := q.GetSessionTimeoutForOrg(ctx, orgID)
			if err != nil {
				return fmt.Errorf("ingest span: get session timeout: %w", err)
			}

			sessionID, err = q.FindActiveSessionForAPIKey(ctx, db.FindActiveSessionForAPIKeyParams{
				ApiKeyID:       apiKeyID,
				TimeoutSeconds: timeoutSeconds,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					newSession = true
					var agentName *string
					if req.AgentName != "" {
						agentName = &req.AgentName
					}
					sessionID, err = q.CreateSession(ctx, db.CreateSessionParams{
						OrganizationID: orgID,
						ApiKeyID:       apiKeyID,
						ExternalID:     nil,
						AgentName:      agentName,
					})
					if err != nil {
						return fmt.Errorf("ingest span: create session: %w", err)
					}
				} else {
					return fmt.Errorf("ingest span: find active session: %w", err)
				}
			}
		}

		// 6. Insert span (SPAN-01).
		spanID, err = q.InsertSpan(ctx, db.InsertSpanParams{
			SessionID:          sessionID,
			OrganizationID:     orgID,
			ProviderType:       req.ProviderType,
			Model:              req.Model,
			Input:              inputPtr,
			Output:             outputPtr,
			InputTokens:        inputTokens,
			OutputTokens:       outputTokens,
			CostUsd:            costUsd,
			DurationMs:         req.DurationMs,
			HttpStatus:         req.HTTPStatus,
			StartedAt:          startedAt,
			FinishReason:       finishReason,
			MaskingApplied:     req.MaskingApplied,
			ClientDisconnected: req.ClientDisconnected,
		})
		if err != nil {
			return fmt.Errorf("ingest span: insert span: %w", err)
		}

		// 6b. Store masking map entries (D-10, D-11 audit trail).
		for _, entry := range req.MaskingMap {
			if err := q.InsertSpanMaskingMap(ctx, db.InsertSpanMaskingMapParams{
				SpanID:        spanID,
				MaskType:      entry.MaskType,
				OriginalValue: entry.OriginalValue,
				MaskedValue:   entry.MaskedValue,
			}); err != nil {
				slog.Warn("failed to insert masking map entry", "span_id", spanID, "error", err)
				// Non-fatal: masking map is audit data, don't fail span ingestion
			}
		}

		// 7. Update session counters (SPAN-04).
		sessionCostDelta := costUsd
		if !costUsd.Valid {
			sessionCostDelta = pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
		}
		return q.UpdateSessionAfterSpan(ctx, db.UpdateSessionAfterSpanParams{
			ID:      sessionID,
			CostUsd: sessionCostDelta,
		})
	})
	if err != nil {
		return err
	}

	// 8. Publish real-time events (WS-02, WS-03).
	if s.hub != nil {
		// span.created on session topic (metrics only, no I/O — WS-03, SEC-03).
		s.hub.Publish(orgID, "session:"+sessionID.String(), hub.Event{
			Type: "span.created",
			Payload: SpanEvent{
				ID:           spanID,
				SessionID:    sessionID,
				ProviderType: req.ProviderType,
				Model:        req.Model,
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				DurationMs:   req.DurationMs,
				HttpStatus:   req.HTTPStatus,
				StartedAt:    req.StartedAt,
			},
		})

		// session.created or session.updated on sessions_list topic (WS-02).
		eventType := "session.updated"
		if newSession {
			eventType = "session.created"
		}
		var agentNamePtr *string
		if req.AgentName != "" {
			agentNamePtr = &req.AgentName
		}
		s.hub.Publish(orgID, "sessions_list:"+orgID.String(), hub.Event{
			Type: eventType,
			Payload: SessionEvent{
				ID:        sessionID,
				Status:    "in_progress",
				AgentName: agentNamePtr,
				SpanCount: 1, // approximate — exact count would require extra query
			},
		})
	}

	return nil
}

// calculateCost computes the total cost in USD from token counts and per-token prices.
// Uses math/big for precise decimal arithmetic with pgtype.Numeric values.
func calculateCost(inputTokens, outputTokens int32, inputPrice, outputPrice pgtype.Numeric) pgtype.Numeric {
	// Convert pgtype.Numeric prices to big.Float.
	inputPriceFloat := numericToBigFloat(inputPrice)
	outputPriceFloat := numericToBigFloat(outputPrice)

	// cost = input_tokens * input_price + output_tokens * output_price
	inputCost := new(big.Float).Mul(new(big.Float).SetInt64(int64(inputTokens)), inputPriceFloat)
	outputCost := new(big.Float).Mul(new(big.Float).SetInt64(int64(outputTokens)), outputPriceFloat)
	totalCost := new(big.Float).Add(inputCost, outputCost)

	// Convert back to pgtype.Numeric via string representation.
	costStr := totalCost.Text('f', 8)
	var result pgtype.Numeric
	if err := result.Scan(costStr); err != nil {
		// Fallback: return zero on conversion error (should not happen with valid prices).
		return pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	}
	return result
}

// numericToBigFloat converts a pgtype.Numeric to a big.Float.
// The value is Int * 10^Exp.
func numericToBigFloat(n pgtype.Numeric) *big.Float {
	if !n.Valid || n.Int == nil {
		return new(big.Float)
	}
	// value = Int * 10^Exp
	intFloat := new(big.Float).SetInt(n.Int)
	if n.Exp == 0 {
		return intFloat
	}
	exp := big.NewInt(10)
	if n.Exp > 0 {
		exp.Exp(exp, big.NewInt(int64(n.Exp)), nil)
		return new(big.Float).Mul(intFloat, new(big.Float).SetInt(exp))
	}
	// Negative exponent: divide.
	exp.Exp(exp, big.NewInt(int64(-n.Exp)), nil)
	return new(big.Float).Quo(intFloat, new(big.Float).SetInt(exp))
}

// RunSessionClosureCron finds idle sessions and closes them with the appropriate status heuristic.
// Called by a time.Ticker goroutine every 30 seconds.
//
// Status heuristic (SESS-05, SESS-06):
//   - any client_disconnected spans: abandoned (client didn't wait for response)
//   - all >= 400 (all errors): failed
//   - some errors: completed_with_errors
//   - all 2xx (no errors): completed
//
// Terminal statuses are never reversed (AND status = 'in_progress' guard).
func (s *InternalService) RunSessionClosureCron(ctx context.Context) error {
	// 1. Find idle sessions.
	idleSessions, err := s.queries.GetIdleSessionsForClosure(ctx)
	if err != nil {
		return fmt.Errorf("session closure: get idle sessions: %w", err)
	}
	if len(idleSessions) == 0 {
		return nil
	}

	// 2. Collect session IDs and get error counts.
	sessionIDs := make([]uuid.UUID, len(idleSessions))
	for i, session := range idleSessions {
		sessionIDs[i] = session.ID
	}

	errorRows, err := s.queries.CountErrorSpansForSessions(ctx, sessionIDs)
	if err != nil {
		return fmt.Errorf("session closure: count error spans: %w", err)
	}

	errorCounts := make(map[uuid.UUID]int64, len(errorRows))
	for _, row := range errorRows {
		errorCounts[row.SessionID] = row.ErrorCount
	}

	disconnectedRows, err := s.queries.CountDisconnectedSpansForSessions(ctx, sessionIDs)
	if err != nil {
		return fmt.Errorf("session closure: count disconnected spans: %w", err)
	}

	disconnectedCounts := make(map[uuid.UUID]int64, len(disconnectedRows))
	for _, row := range disconnectedRows {
		disconnectedCounts[row.SessionID] = row.DisconnectedCount
	}

	// 3. Apply status heuristic and group by status (SESS-05, SESS-06).
	statusGroups := make(map[string][]uuid.UUID)
	for _, session := range idleSessions {
		errorCount := errorCounts[session.ID]
		disconnectedCount := disconnectedCounts[session.ID]
		var status string
		switch {
		case disconnectedCount > 0:
			status = "abandoned"
		case errorCount == int64(session.SpanCount):
			status = "failed"
		case errorCount > 0:
			status = "completed_with_errors"
		default:
			status = "completed"
		}
		statusGroups[status] = append(statusGroups[status], session.ID)
	}

	// 4. Batch close by status.
	for status, ids := range statusGroups {
		err := s.queries.CloseSessionsWithStatus(ctx, db.CloseSessionsWithStatusParams{
			Status:     status,
			SessionIds: ids,
		})
		if err != nil {
			return fmt.Errorf("session closure: close sessions with status %s: %w", status, err)
		}
	}

	// 4.5. Run intelligence pipeline concurrently for closed sessions (NARR-03, FLCL-03, SC#5).
	// Derives from server context so shutdown cancels in-flight pipelines cleanly (M-8).
	// 10s timeout aligns with Docker's default SIGKILL grace period.
	if s.intelligenceService != nil && len(idleSessions) > 0 {
		const maxConcurrent = 5 // bounded pool — prevents goroutine explosion under load
		pipelineCtx, pipelineCancel := context.WithTimeout(ctx, 30*time.Second)
		defer pipelineCancel()

		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup

		for _, session := range idleSessions {
			wg.Add(1)
			sem <- struct{}{} // acquire semaphore slot (blocks if pool full)
			go func(sessionID, orgID uuid.UUID) {
				defer wg.Done()
				defer func() { <-sem }() // release semaphore slot
				defer func() {
					if r := recover(); r != nil {
						slog.Error("intelligence pipeline panic", "session", sessionID, "panic", r)
					}
				}()
				if err := s.intelligenceService.RunPipeline(pipelineCtx, sessionID, orgID); err != nil {
					slog.Error("intelligence pipeline error", "session", sessionID, "error", err)
					// Non-fatal: other sessions continue (D-04).
				}
			}(session.ID, session.OrganizationID)
		}

		wg.Wait() // ensure all pipelines complete before publishing WS events
	}

	// 5. Publish session.updated events for all closed sessions (WS-02).
	if s.hub != nil {
		// Build reverse map: session ID -> status for O(1) lookup.
		sessionStatus := make(map[uuid.UUID]string, len(idleSessions))
		for status, ids := range statusGroups {
			for _, id := range ids {
				sessionStatus[id] = status
			}
		}

		for _, session := range idleSessions {
			status := sessionStatus[session.ID]
			s.hub.Publish(session.OrganizationID, "sessions_list:"+session.OrganizationID.String(), hub.Event{
				Type: "session.updated",
				Payload: SessionEvent{
					ID:        session.ID,
					Status:    status,
					SpanCount: session.SpanCount,
				},
			})
		}
	}

	slog.Info("session-closure complete", "closed", len(idleSessions))
	return nil
}
