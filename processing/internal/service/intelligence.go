package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/agentspan/processing/internal/db"
	"github.com/agentspan/processing/internal/hub"
	"github.com/agentspan/processing/internal/llm"
	"github.com/agentspan/processing/internal/txutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IntelligenceService implements the 3-step post-session pipeline:
// system prompt extraction, narrative generation, and failure clustering.
type IntelligenceService struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	llm     llm.Client // nil when LLM not configured
	hub     *hub.Hub   // for failure_cluster_created events
}

// NewIntelligenceService creates a new IntelligenceService.
func NewIntelligenceService(queries *db.Queries, pool *pgxpool.Pool, llmClient llm.Client, h *hub.Hub) *IntelligenceService {
	return &IntelligenceService{queries: queries, pool: pool, llm: llmClient, hub: h}
}

// RunPipeline runs the full 3-step intelligence pipeline for a closed session.
// Each step logs errors but does NOT return early — pipeline continues on partial failure (per D-04).
func (s *IntelligenceService) RunPipeline(ctx context.Context, sessionID, orgID uuid.UUID) error {
	// 1. Fetch spans ordered by created_at ASC.
	spans, err := s.queries.GetSpansBySessionID(ctx, db.GetSpansBySessionIDParams{
		SessionID:      sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("intelligence: get spans for session %s: %w", sessionID, err)
	}

	if len(spans) == 0 {
		// Nothing to process.
		return nil
	}

	// 2. Fetch org plan and locale.
	org, err := s.queries.GetOrganizationByIDForIntel(ctx, orgID)
	if err != nil {
		return fmt.Errorf("intelligence: get org for session %s: %w", sessionID, err)
	}

	// 3. Extract system prompts (all plans).
	if err := s.extractSystemPrompts(ctx, orgID, spans); err != nil {
		slog.Error("intelligence: extractSystemPrompts error", "session", sessionID, "error", err)
		// Continue pipeline per D-04.
	}

	// 4. Generate narrative (plan-gated inside).
	if err := s.generateNarrative(ctx, sessionID, orgID, org.Plan, org.Locale, spans); err != nil {
		slog.Error("intelligence: generateNarrative error", "session", sessionID, "error", err)
		// Continue pipeline per D-04.
	}

	// 5. Check session status for failure clustering.
	session, err := s.queries.GetSessionByID(ctx, db.GetSessionByIDParams{
		ID:             sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		slog.Error("intelligence: get session status error", "session", sessionID, "error", err)
		return nil
	}

	if session.Status == "failed" || session.Status == "completed_with_errors" {
		if err := s.clusterFailure(ctx, sessionID, orgID, org.Plan, spans); err != nil {
			slog.Error("intelligence: clusterFailure error", "session", sessionID, "error", err)
		}
	}

	return nil
}

// extractSystemPrompts finds a common prefix across all span inputs (>= 100 chars),
// deduplicates via SHA256 hash, replaces the prefix in span inputs with [System Prompt #SP-N].
func (s *IntelligenceService) extractSystemPrompts(ctx context.Context, orgID uuid.UUID, spans []db.Span) error {
	if len(spans) < 2 {
		return nil // Need at least 2 spans to compare.
	}

	// Only process spans that have non-nil inputs.
	var inputSpans []db.Span
	for _, sp := range spans {
		if sp.Input != nil {
			inputSpans = append(inputSpans, sp)
		}
	}
	if len(inputSpans) < 2 {
		return nil
	}

	// Compute LCP of first two spans.
	candidate := longestCommonPrefix(deref(inputSpans[0].Input), deref(inputSpans[1].Input))
	if len(candidate) < 100 {
		return nil // Per SYSP-02.
	}

	// Verify LCP against remaining spans.
	for _, sp := range inputSpans[2:] {
		candidate = longestCommonPrefix(candidate, deref(sp.Input))
		if len(candidate) < 100 {
			return nil
		}
	}

	// Compute SHA256 hash of the prefix content.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(candidate)))

	// Get next short_uid using org's existing count.
	count, err := s.queries.CountSystemPromptsForOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("extractSystemPrompts: count system prompts: %w", err)
	}
	shortUID := fmt.Sprintf("SP-%d", count+1)

	// Find or create the system prompt record.
	sp, err := s.queries.FindOrCreateSystemPrompt(ctx, db.FindOrCreateSystemPromptParams{
		OrganizationID: orgID,
		Content:        candidate,
		ContentHash:    hash,
		ShortUid:       shortUID,
	})
	if err != nil {
		return fmt.Errorf("extractSystemPrompts: find or create system prompt: %w", err)
	}

	replacement := "[System Prompt #" + sp.ShortUid + "]"

	// Replace prefix in each span input and link span to system prompt — atomically.
	return txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		for _, span := range inputSpans {
			if !strings.HasPrefix(deref(span.Input), candidate) {
				continue
			}
			if err := q.ReplaceSpanInputPrefix(ctx, db.ReplaceSpanInputPrefixParams{
				ID:             span.ID,
				OrganizationID: orgID,
				OldPrefix:      candidate,
				Replacement:    &replacement,
			}); err != nil {
				slog.Error("intelligence: replace span input prefix", "span", span.ID, "error", err)
			}
			if err := q.InsertSpanSystemPrompt(ctx, db.InsertSpanSystemPromptParams{
				SpanID:         span.ID,
				SystemPromptID: sp.ID,
			}); err != nil {
				slog.Error("intelligence: insert span system prompt", "span", span.ID, "error", err)
			}
		}
		return nil
	})
}

// generateNarrative creates a narrative for the session.
// Free plan: deterministic metadata summary (per D-08, not an LLM narrative).
// Pro/Self-host plan: LLM-generated narrative in org locale; falls back to metadata summary on LLM error.
func (s *IntelligenceService) generateNarrative(ctx context.Context, sessionID, orgID uuid.UUID, plan, locale string, spans []db.Span) error {
	// Free plan or no LLM configured: use deterministic metadata summary (per D-08).
	if plan == "free" || s.llm == nil {
		summary := buildMetadataSummary(spans, "")
		if err := s.queries.UpdateSessionNarrative(ctx, db.UpdateSessionNarrativeParams{
			ID:             sessionID,
			Narrative:      &summary,
			OrganizationID: orgID,
		}); err != nil {
			return fmt.Errorf("generateNarrative: update session narrative (free): %w", err)
		}
		return nil
	}

	// Pro/Self-host: re-fetch spans to get cleaned inputs (after system prompt extraction).
	cleanedSpans, err := s.queries.GetSpansBySessionID(ctx, db.GetSpansBySessionIDParams{
		SessionID:      sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		// Fall back to original spans if re-fetch fails.
		cleanedSpans = spans
	}

	// Build LLM messages.
	systemMsg := fmt.Sprintf(
		"You are an observability assistant. Summarize this AI agent session in 2-3 concise sentences from the user's perspective. Write in %s language. Focus on what the agent did, what it accomplished, and any notable issues.",
		locale,
	)

	var userMsgBuilder strings.Builder
	userMsgBuilder.WriteString("Session spans:\n\n")
	for i, sp := range cleanedSpans {
		fmt.Fprintf(&userMsgBuilder, "Span %d:\n", i+1)
		fmt.Fprintf(&userMsgBuilder, "  model: %s\n", sp.Model)
		fmt.Fprintf(&userMsgBuilder, "  input: %s\n", truncate(deref(sp.Input), 500))
		fmt.Fprintf(&userMsgBuilder, "  output: %s\n", truncate(deref(sp.Output), 500))
		if sp.InputTokens != nil {
			fmt.Fprintf(&userMsgBuilder, "  input_tokens: %d\n", *sp.InputTokens)
		}
		if sp.OutputTokens != nil {
			fmt.Fprintf(&userMsgBuilder, "  output_tokens: %d\n", *sp.OutputTokens)
		}
		fmt.Fprintf(&userMsgBuilder, "  http_status: %d\n", sp.HttpStatus)
		fmt.Fprintf(&userMsgBuilder, "  duration_ms: %d\n", sp.DurationMs)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsgBuilder.String()},
	}

	narrative, err := s.llm.Complete(ctx, messages)
	if err != nil {
		// Log only session ID and error type — no span I/O or LLM response body (CLAUDE.md security).
		slog.Error("intelligence: LLM narrative error", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
		// Fall back to metadata summary.
		narrative = buildMetadataSummary(cleanedSpans, "")
	}

	if err := s.queries.UpdateSessionNarrative(ctx, db.UpdateSessionNarrativeParams{
		ID:             sessionID,
		Narrative:      &narrative,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("generateNarrative: update session narrative: %w", err)
	}

	return nil
}

// clusterFailure assigns a failed session to a failure cluster.
// Free plan: deterministic label from HTTP status + truncated output (per D-09).
// Pro/Self-host plan: LLM-classified with fallback to deterministic (per D-10).
func (s *IntelligenceService) clusterFailure(ctx context.Context, sessionID, orgID uuid.UUID, plan string, spans []db.Span) error {
	if len(spans) == 0 {
		return nil
	}

	// Use the last span (ordered by created_at ASC, so last element is most recent).
	lastSpan := spans[len(spans)-1]

	var cluster db.FailureCluster
	isNew := false

	if plan == "free" || s.llm == nil {
		// Deterministic clustering (per D-09).
		var clusterErr error
		cluster, clusterErr = s.deterministicCluster(ctx, orgID, lastSpan)
		if clusterErr != nil {
			return fmt.Errorf("clusterFailure: deterministic cluster: %w", clusterErr)
		}
	} else {
		// LLM-classified clustering (per D-10).
		existingLabels, err := s.queries.ListFailureClusterLabels(ctx, orgID)
		if err != nil {
			// Fall back to deterministic on list error.
			slog.Error("intelligence: list failure cluster labels error", "session", sessionID, "error", err)
			cluster, err = s.deterministicCluster(ctx, orgID, lastSpan)
			if err != nil {
				return fmt.Errorf("clusterFailure: deterministic fallback: %w", err)
			}
		} else {
			// Build LLM prompt with existing clusters.
			var listStr strings.Builder
			for _, l := range existingLabels {
				fmt.Fprintf(&listStr, "- %s: %s\n", l.ID, l.Label)
			}

			systemMsg := fmt.Sprintf(
				"You are a failure classifier for an AI agent observability platform. Given a session's error context, either assign it to an existing failure cluster or create a new one. Existing clusters:\n%s\nRespond with EXACTLY one line: either the cluster ID (UUID) of an existing cluster, or NEW: <short label describing the failure pattern>",
				listStr.String(),
			)

			userMsg := fmt.Sprintf(
				"Session error context:\n  http_status: %d\n  model: %s\n  output: %s",
				lastSpan.HttpStatus,
				lastSpan.Model,
				truncate(deref(lastSpan.Output), 500),
			)

			messages := []llm.Message{
				{Role: "system", Content: systemMsg},
				{Role: "user", Content: userMsg},
			}

			llmResp, err := s.llm.Complete(ctx, messages)
			if err != nil {
				// Log session ID and error type only (CLAUDE.md security).
				slog.Error("intelligence: LLM cluster error", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
				// Fall back to deterministic.
				cluster, err = s.deterministicCluster(ctx, orgID, lastSpan)
				if err != nil {
					return fmt.Errorf("clusterFailure: deterministic fallback after LLM error: %w", err)
				}
			} else {
				llmResp = strings.TrimSpace(llmResp)
				if strings.HasPrefix(llmResp, "NEW: ") {
					// Create new cluster with LLM-provided label.
					label := strings.TrimPrefix(llmResp, "NEW: ")
					newCluster, err := s.queries.CreateFailureCluster(ctx, db.CreateFailureClusterParams{
						OrganizationID: orgID,
						Label:          label,
					})
					if err != nil {
						// Fall back to deterministic.
						slog.Error("intelligence: create failure cluster error", "session", sessionID, "error", err)
						cluster, err = s.deterministicCluster(ctx, orgID, lastSpan)
						if err != nil {
							return fmt.Errorf("clusterFailure: deterministic fallback after create error: %w", err)
						}
					} else {
						cluster = newCluster
						isNew = true
					}
				} else {
					// LLM returned a cluster UUID — parse and look up.
					clusterID, err := uuid.Parse(llmResp)
					if err != nil {
						// Invalid UUID from LLM — fall back to deterministic.
						slog.Warn("intelligence: invalid cluster UUID from LLM", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
						cluster, err = s.deterministicCluster(ctx, orgID, lastSpan)
						if err != nil {
							return fmt.Errorf("clusterFailure: deterministic fallback after parse error: %w", err)
						}
					} else {
						// Look up the cluster to verify it exists and belongs to this org.
						found, lookupErr := s.queries.GetFailureClusterByID(ctx, db.GetFailureClusterByIDParams{
							ID:             clusterID,
							OrganizationID: orgID,
						})
						if lookupErr != nil {
							slog.Warn("intelligence: cluster not found, falling back to deterministic", "cluster", clusterID, "org", orgID)
							cluster, err = s.deterministicCluster(ctx, orgID, lastSpan)
							if err != nil {
								return fmt.Errorf("clusterFailure: deterministic fallback after lookup error: %w", err)
							}
						} else {
							cluster = found
						}
					}
				}
			}
		}
	}

	// Link session to cluster and increment count — atomically.
	if err := txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)

		if err := q.InsertSessionFailureCluster(ctx, db.InsertSessionFailureClusterParams{
			SessionID:        sessionID,
			FailureClusterID: cluster.ID,
		}); err != nil {
			return fmt.Errorf("clusterFailure: insert session failure cluster: %w", err)
		}

		return q.IncrementFailureClusterCount(ctx, db.IncrementFailureClusterCountParams{
			ID:             cluster.ID,
			OrganizationID: orgID,
		})
	}); err != nil {
		return err
	}

	// Publish hub event for new clusters (D-12) — triggers reactive alert subscription.
	if isNew && s.hub != nil {
		s.hub.Publish(uuid.Nil, "failure_cluster_created", hub.Event{
			Type: "failure_cluster_created",
			Payload: map[string]interface{}{
				"cluster_id":      cluster.ID,
				"label":           cluster.Label,
				"organization_id": orgID,
			},
		})
	}

	return nil
}

// deterministicCluster builds a failure cluster label deterministically from the last span's HTTP status
// and truncated output, then finds or creates the cluster record.
func (s *IntelligenceService) deterministicCluster(ctx context.Context, orgID uuid.UUID, lastSpan db.Span) (db.FailureCluster, error) {
	label := fmt.Sprintf("HTTP %d: %s", lastSpan.HttpStatus, truncate(deref(lastSpan.Output), 100))

	cluster, err := s.queries.FindOrCreateFailureClusterByLabel(ctx, db.FindOrCreateFailureClusterByLabelParams{
		OrganizationID: orgID,
		Label:          label,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// DO NOTHING conflict — row exists, fetch it.
			cluster, err = s.queries.GetFailureClusterByLabel(ctx, db.GetFailureClusterByLabelParams{
				OrganizationID: orgID,
				Label:          label,
			})
			if err != nil {
				return db.FailureCluster{}, fmt.Errorf("deterministicCluster: get existing cluster: %w", err)
			}
		} else {
			return db.FailureCluster{}, fmt.Errorf("deterministicCluster: find or create cluster: %w", err)
		}
	}

	return cluster, nil
}

// longestCommonPrefix returns the longest common prefix of strings a and b,
// always splitting on valid UTF-8 rune boundaries.
func longestCommonPrefix(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) {
		ra, sza := utf8.DecodeRuneInString(a[i:])
		rb, szb := utf8.DecodeRuneInString(b[i:])
		if ra != rb || sza != szb {
			break
		}
		i += sza
	}
	return a[:i]
}

// deref safely dereferences a string pointer, returning "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// truncate returns s truncated to approximately maxLen bytes, always splitting
// on valid UTF-8 rune boundaries. Returns s unchanged if len(s) <= maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Walk backward from maxLen to find a valid rune boundary.
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen]
}

// buildMetadataSummary builds a deterministic metadata summary from spans.
// Format: "N spans, M models (model1, model2), Xk tokens, status"
func buildMetadataSummary(spans []db.Span, status string) string {
	if len(spans) == 0 {
		return "0 spans"
	}

	// Collect unique models preserving first-seen order.
	seen := make(map[string]bool)
	var models []string
	var totalTokens int64

	for _, sp := range spans {
		if !seen[sp.Model] {
			seen[sp.Model] = true
			models = append(models, sp.Model)
		}
		if sp.InputTokens != nil {
			totalTokens += int64(*sp.InputTokens)
		}
		if sp.OutputTokens != nil {
			totalTokens += int64(*sp.OutputTokens)
		}
	}

	// Format model list (up to 3).
	displayModels := models
	if len(displayModels) > 3 {
		displayModels = displayModels[:3]
	}

	modelCount := len(models)
	modelList := strings.Join(displayModels, ", ")

	// Format token count.
	var tokenStr string
	if totalTokens >= 1000 {
		tokenStr = fmt.Sprintf("%.1fk", float64(totalTokens)/1000.0)
	} else {
		tokenStr = fmt.Sprintf("%d", totalTokens)
	}

	spanWord := "spans"
	if len(spans) == 1 {
		spanWord = "span"
	}

	modelWord := "models"
	if modelCount == 1 {
		modelWord = "model"
	}

	result := fmt.Sprintf("%d %s, %d %s (%s), %s tokens",
		len(spans), spanWord,
		modelCount, modelWord,
		modelList,
		tokenStr,
	)

	if status != "" {
		result += ", " + status
	}

	return result
}
