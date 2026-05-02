package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/errtrack"
	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/llm"
	"github.com/agentorbit-tech/agentorbit/processing/internal/txutil"
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

	// 4. Analyze spans for content anomalies (plan-gated inside).
	// This also upgrades session status and creates failure clusters for anomalies.
	if err := s.analyzeSpans(ctx, sessionID, orgID, org.Plan, org.Locale, spans); err != nil {
		slog.Error("intelligence: analyzeSpans error", "session", sessionID, "error", err)
		// Continue pipeline per D-04.
	}

	// 5. Generate narrative (plan-gated inside).
	if err := s.generateNarrative(ctx, sessionID, orgID, org.Plan, org.Locale, spans); err != nil {
		slog.Error("intelligence: generateNarrative error", "session", sessionID, "error", err)
		// Continue pipeline per D-04.
	}

	// 6. Check session status for HTTP-error failure clustering.
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

// extractSystemPrompts extracts the system prompt from span inputs.
// Strategy 1: if inputs contain "system: ..." lines, extract them as the system prompt.
//   Works with a single span — explicit system blocks don't need cross-span confirmation.
// Strategy 2 (fallback): if no system lines, use longest common prefix across all inputs.
//   Requires at least 2 spans to compare.
// The candidate must be >= 100 chars.
func (s *IntelligenceService) extractSystemPrompts(ctx context.Context, orgID uuid.UUID, spans []db.GetSpansBySessionIDRow) error {
	// Collect spans with non-nil inputs.
	var inputSpans []db.GetSpansBySessionIDRow
	for _, sp := range spans {
		if sp.Input != nil {
			inputSpans = append(inputSpans, sp)
		}
	}
	if len(inputSpans) == 0 {
		return nil
	}

	// Strategy 1: extract "system: ..." block from each span.
	candidate := extractSystemBlockCandidate(inputSpans)

	// Strategy 2 (fallback): longest common prefix — needs 2+ spans.
	if candidate == "" && len(inputSpans) >= 2 {
		candidate = longestCommonPrefix(deref(inputSpans[0].Input), deref(inputSpans[1].Input))
		for _, sp := range inputSpans[2:] {
			candidate = longestCommonPrefix(candidate, deref(sp.Input))
			if len(candidate) < 100 {
				return nil
			}
		}
	}

	if len(candidate) < 100 {
		return nil
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

	replacement := "[System Prompt #" + sp.ShortUid + "]\n"

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

// Allowed anomaly categories — used as failure cluster labels.
var validAnomalyCategories = map[string]bool{
	"hallucination": true,
	"context_loss":  true,
	"echo":          true,
	"off_topic":     true,
	"empty_output":  true,
	"malformed":     true,
}

// spanVerdict is the expected JSON structure from the LLM span analysis.
type spanVerdict struct {
	SpanIndex int    `json:"span_index"`
	OK        bool   `json:"ok"`
	Category  string `json:"category"`
	Reason    string `json:"reason"`
}

// analyzeSpans sends all session spans to the LLM in one call and asks for a per-span verdict.
// Marks anomalous spans, upgrades session status, and creates failure clusters.
// Free plan or no LLM configured: skips analysis.
func (s *IntelligenceService) analyzeSpans(ctx context.Context, sessionID, orgID uuid.UUID, plan, locale string, spans []db.GetSpansBySessionIDRow) error {
	if plan == "free" || s.llm == nil {
		return nil
	}

	// Re-fetch spans to get cleaned inputs (after system prompt extraction).
	cleanedSpans, err := s.queries.GetSpansBySessionID(ctx, db.GetSpansBySessionIDParams{
		SessionID:      sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		cleanedSpans = spans
	}

	systemMsg := fmt.Sprintf(
		`You are a quality analyst for AI agent sessions. Analyze each span for content anomalies.
Write in %s language for the reason field.

For each span, determine if there is a content-level problem. If there is, assign exactly one category:
- "echo" — output echoes or parrots the input instead of answering
- "context_loss" — loss of context between turns (agent forgets earlier conversation)
- "hallucination" — hallucinated, fabricated, or placeholder data in output
- "off_topic" — output is nonsensical, off-topic, or contradicts the input
- "empty_output" — unusually short or empty output relative to input complexity
- "malformed" — unparseable, garbled, or structurally broken output

Respond with ONLY a JSON array. One object per span:
[{"span_index": 0, "ok": true}, {"span_index": 1, "ok": false, "category": "echo", "reason": "One sentence describing the issue"}]

If a span has HTTP error status (4xx/5xx), mark it as ok=true — HTTP errors are tracked separately.
Only flag genuine content-level anomalies.`, locale)

	var userMsgBuilder strings.Builder
	userMsgBuilder.WriteString("Session spans (in chronological order):\n\n")
	for i, sp := range cleanedSpans {
		fmt.Fprintf(&userMsgBuilder, "--- Span %d ---\n", i)
		fmt.Fprintf(&userMsgBuilder, "  model: %s\n", sp.Model)
		fmt.Fprintf(&userMsgBuilder, "  http_status: %d\n", sp.HttpStatus)
		fmt.Fprintf(&userMsgBuilder, "  duration_ms: %d\n", sp.DurationMs)
		fmt.Fprintf(&userMsgBuilder, "  input: %s\n", truncateForLLM(deref(sp.Input), 500, 300))
		fmt.Fprintf(&userMsgBuilder, "  output: %s\n", truncateForLLM(deref(sp.Output), 500, 300))
	}

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsgBuilder.String()},
	}

	resp, err := s.llm.Complete(ctx, messages)
	if err != nil {
		slog.Error("intelligence: LLM analyzeSpans error", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
		errtrack.Capture(err, errtrack.Fields{"component": "llm", "stage": "analyze_spans"})
		return err
	}

	verdicts, err := parseSpanVerdicts(resp)
	if err != nil {
		slog.Error("intelligence: parse span verdicts error", "session", sessionID, "error", err)
		return err
	}

	// Apply verdicts to spans and collect unique categories.
	hasAnomalies := false
	categories := make(map[string]bool)
	for _, v := range verdicts {
		if v.OK || v.Reason == "" {
			continue
		}
		if v.SpanIndex < 0 || v.SpanIndex >= len(cleanedSpans) {
			continue
		}
		// Validate category; fall back to "malformed" if LLM returned unknown category.
		cat := v.Category
		if !validAnomalyCategories[cat] {
			cat = "malformed"
		}
		hasAnomalies = true
		categories[cat] = true
		sp := cleanedSpans[v.SpanIndex]
		if err := s.queries.SetSpanAnomaly(ctx, db.SetSpanAnomalyParams{
			ID:              sp.ID,
			OrganizationID:  orgID,
			AnomalyReason:   &v.Reason,
			AnomalyCategory: &cat,
		}); err != nil {
			slog.Error("intelligence: set span anomaly", "span", sp.ID, "error", err)
		}
	}

	if !hasAnomalies {
		return nil
	}

	// Upgrade session status completed → completed_with_errors immediately.
	if err := s.queries.UpgradeSessionToErrors(ctx, db.UpgradeSessionToErrorsParams{
		ID:             sessionID,
		OrganizationID: orgID,
	}); err != nil {
		slog.Error("intelligence: upgrade session to errors", "session", sessionID, "error", err)
	}

	// Create failure clusters for each unique anomaly category and link session.
	for cat := range categories {
		if err := s.linkAnomalyCluster(ctx, sessionID, orgID, cat); err != nil {
			slog.Error("intelligence: link anomaly cluster", "session", sessionID, "category", cat, "error", err)
		}
	}

	return nil
}

// linkAnomalyCluster finds or creates a failure cluster for the given anomaly category
// and links the session to it.
func (s *IntelligenceService) linkAnomalyCluster(ctx context.Context, sessionID, orgID uuid.UUID, category string) error {
	cluster, err := s.queries.FindOrCreateFailureClusterByLabel(ctx, db.FindOrCreateFailureClusterByLabelParams{
		OrganizationID: orgID,
		Label:          category,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			cluster, err = s.queries.GetFailureClusterByLabel(ctx, db.GetFailureClusterByLabelParams{
				OrganizationID: orgID,
				Label:          category,
			})
			if err != nil {
				return fmt.Errorf("get existing cluster: %w", err)
			}
		} else {
			return fmt.Errorf("find or create cluster: %w", err)
		}
	}

	return txutil.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := q.InsertSessionFailureCluster(ctx, db.InsertSessionFailureClusterParams{
			SessionID:        sessionID,
			FailureClusterID: cluster.ID,
		}); err != nil {
			return fmt.Errorf("insert session failure cluster: %w", err)
		}
		return q.IncrementFailureClusterCount(ctx, db.IncrementFailureClusterCountParams{
			ID:             cluster.ID,
			OrganizationID: orgID,
		})
	})
}

// parseSpanVerdicts extracts the JSON array from the LLM response.
// The LLM may wrap the JSON in markdown code fences — strip them.
func parseSpanVerdicts(resp string) ([]spanVerdict, error) {
	resp = strings.TrimSpace(resp)

	// Strip markdown code fences if present.
	if strings.HasPrefix(resp, "```") {
		lines := strings.SplitN(resp, "\n", 2)
		if len(lines) == 2 {
			resp = lines[1]
		}
		if idx := strings.LastIndex(resp, "```"); idx >= 0 {
			resp = resp[:idx]
		}
		resp = strings.TrimSpace(resp)
	}

	var verdicts []spanVerdict
	if err := json.Unmarshal([]byte(resp), &verdicts); err != nil {
		return nil, fmt.Errorf("unmarshal span verdicts: %w (response: %s)", err, truncate(resp, 200))
	}
	return verdicts, nil
}

// generateNarrative creates a narrative for the session.
// Free plan: deterministic metadata summary (per D-08, not an LLM narrative).
// Pro/Self-host plan: LLM-generated narrative in org locale; falls back to metadata summary on LLM error.
func (s *IntelligenceService) generateNarrative(ctx context.Context, sessionID, orgID uuid.UUID, plan, locale string, spans []db.GetSpansBySessionIDRow) error {
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
		`You are an observability analyst for AI agent sessions. Write in %s language.

Write 1-2 short sentences. Be terse and factual — no filler, no hedging, no preamble.

Sentence 1: what the agent did (one clause, concrete).
Sentence 2 (only if anomalies exist): flag them. Otherwise omit entirely.

Anomalies to flag: HTTP 4xx/5xx, output echoing input, lost context between turns, hallucinated/placeholder data, suspiciously empty outputs, malformed spans.

Do not restate the task framing. Do not explain your reasoning. Output only the narrative.`,
		locale,
	)

	var userMsgBuilder strings.Builder
	userMsgBuilder.WriteString("Session spans (in chronological order):\n\n")
	for i, sp := range cleanedSpans {
		fmt.Fprintf(&userMsgBuilder, "--- Span %d ---\n", i+1)
		fmt.Fprintf(&userMsgBuilder, "  model: %s\n", sp.Model)
		fmt.Fprintf(&userMsgBuilder, "  http_status: %d\n", sp.HttpStatus)
		fmt.Fprintf(&userMsgBuilder, "  duration_ms: %d\n", sp.DurationMs)
		if sp.InputTokens != nil {
			fmt.Fprintf(&userMsgBuilder, "  input_tokens: %d\n", *sp.InputTokens)
		}
		if sp.OutputTokens != nil {
			fmt.Fprintf(&userMsgBuilder, "  output_tokens: %d\n", *sp.OutputTokens)
		}
		fmt.Fprintf(&userMsgBuilder, "  input: %s\n", truncateForLLM(deref(sp.Input), 500, 300))
		fmt.Fprintf(&userMsgBuilder, "  output: %s\n", truncateForLLM(deref(sp.Output), 500, 300))
	}

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsgBuilder.String()},
	}

	narrative, err := s.llm.Complete(ctx, messages)
	if err != nil {
		// Log only session ID and error type — no span I/O or LLM response body (CLAUDE.md security).
		slog.Error("intelligence: LLM narrative error", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
		errtrack.Capture(err, errtrack.Fields{"component": "llm", "stage": "narrative"})
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
func (s *IntelligenceService) clusterFailure(ctx context.Context, sessionID, orgID uuid.UUID, plan string, spans []db.GetSpansBySessionIDRow) error {
	if len(spans) == 0 {
		return nil
	}

	// Find the last span with an HTTP error (>= 400). If no error spans exist
	// (e.g. session is completed_with_errors only due to content anomalies),
	// skip HTTP-based clustering — anomaly clusters are handled by analyzeSpans.
	var lastSpan db.GetSpansBySessionIDRow
	found := false
	for i := len(spans) - 1; i >= 0; i-- {
		if spans[i].HttpStatus >= 400 {
			lastSpan = spans[i]
			found = true
			break
		}
	}
	if !found {
		return nil
	}

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
				truncateForLLM(deref(lastSpan.Output), 500, 300),
			)

			messages := []llm.Message{
				{Role: "system", Content: systemMsg},
				{Role: "user", Content: userMsg},
			}

			llmResp, err := s.llm.Complete(ctx, messages)
			if err != nil {
				// Log session ID and error type only (CLAUDE.md security).
				slog.Error("intelligence: LLM cluster error", "session", sessionID, "error_type", fmt.Sprintf("%T", err))
				errtrack.Capture(err, errtrack.Fields{"component": "llm", "stage": "cluster"})
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
func (s *IntelligenceService) deterministicCluster(ctx context.Context, orgID uuid.UUID, lastSpan db.GetSpansBySessionIDRow) (db.FailureCluster, error) {
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

// extractSystemBlock returns the "system: ..." lines from the beginning of
// an input text (produced by extractInputText in the proxy). It stops at the
// first line that does not start with "system: ". The trailing newline after
// the last system line is included so it can be cleanly replaced.
func extractSystemBlock(input string) string {
	var end int
	for {
		nlIdx := strings.IndexByte(input[end:], '\n')
		if nlIdx < 0 {
			// Last line without trailing newline.
			if strings.HasPrefix(input[end:], "system: ") {
				return input[:len(input)]
			}
			break
		}
		line := input[end : end+nlIdx]
		if !strings.HasPrefix(line, "system: ") {
			break
		}
		end += nlIdx + 1 // include the '\n'
	}
	if end == 0 {
		return ""
	}
	return input[:end]
}

// extractSystemBlockCandidate tries to find a consistent system block across spans.
// Spans with missing/broken inputs are skipped. Returns the system block if all
// parseable spans share the same one; empty string if blocks differ across spans.
func extractSystemBlockCandidate(spans []db.GetSpansBySessionIDRow) string {
	var candidate string
	for _, sp := range spans {
		sysBlock := extractSystemBlock(deref(sp.Input))
		if sysBlock == "" {
			continue // Skip spans without a system block (e.g. unparseable body).
		}
		if candidate == "" {
			candidate = sysBlock
		} else if sysBlock != candidate {
			return "" // System blocks differ across parseable spans — fall back.
		}
	}
	return candidate
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

// truncateForLLM returns s shortened for inclusion in an LLM prompt.
// If s fits within maxLen, returned unchanged. Otherwise keeps the first
// headLen bytes and the last (maxLen-headLen) bytes, joined by an explicit
// "...[truncated, N bytes]..." marker so the model knows it sees a fragment.
// All splits respect UTF-8 rune boundaries.
func truncateForLLM(s string, maxLen, headLen int) string {
	if len(s) <= maxLen {
		return s
	}
	tailLen := maxLen - headLen
	if tailLen <= 0 || headLen <= 0 {
		return truncate(s, maxLen)
	}
	head := truncate(s, headLen)

	tailStart := len(s) - tailLen
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	tail := s[tailStart:]

	omitted := len(s) - len(head) - len(tail)
	return fmt.Sprintf("%s...[truncated, %d bytes]...%s", head, omitted, tail)
}

// buildMetadataSummary builds a deterministic metadata summary from spans.
// Format: "N spans, M models (model1, model2), Xk tokens, status"
func buildMetadataSummary(spans []db.GetSpansBySessionIDRow, status string) string {
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
