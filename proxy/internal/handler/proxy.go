package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agentspan/proxy/internal/auth"
	"github.com/agentspan/proxy/internal/masking"
	"github.com/agentspan/proxy/internal/metrics"
	"github.com/agentspan/proxy/internal/span"
)

// ProxyHandler forwards agent requests to LLM providers, handling auth, provider-specific
// headers, SSE streaming, timeout, and async span dispatch.

// transportCache caches HTTP transports keyed by resolved IP to reuse connection pools
// instead of creating a new transport (and TCP connection pool) per request.
type transportCache struct {
	mu         sync.RWMutex
	cache      map[string]*http.Transport
	maxEntries int
}

func (tc *transportCache) getOrCreate(resolvedIPs []net.IP) *http.Transport {
	key := resolvedIPs[0].String()
	tc.mu.RLock()
	if t, ok := tc.cache[key]; ok {
		tc.mu.RUnlock()
		return t
	}
	tc.mu.RUnlock()
	tc.mu.Lock()
	defer tc.mu.Unlock()
	// Double-check after write lock
	if t, ok := tc.cache[key]; ok {
		return t
	}
	// Evict one entry if at capacity
	if len(tc.cache) >= tc.maxEntries {
		for evictKey, evictTransport := range tc.cache {
			evictTransport.CloseIdleConnections()
			delete(tc.cache, evictKey)
			break
		}
	}
	t := &http.Transport{
		DialContext:         pinnedDialContext(resolvedIPs),
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	tc.cache[key] = t
	return t
}

// keyRateLimiter tracks per-API-key request timestamps for rate limiting.
type keyRateLimiter struct {
	mu         sync.Mutex
	requests   map[string][]time.Time
	limit      int
	window     time.Duration
	maxEntries int
}

func newKeyRateLimiter(limit int, window time.Duration) *keyRateLimiter {
	return &keyRateLimiter{
		requests:   make(map[string][]time.Time),
		limit:      limit,
		window:     window,
		maxEntries: 10000,
	}
}

func (krl *keyRateLimiter) allow(key string) bool {
	if krl.limit <= 0 {
		return true
	}
	krl.mu.Lock()
	defer krl.mu.Unlock()

	// Cap: reject new keys when at capacity (existing keys still work)
	if _, exists := krl.requests[key]; !exists && len(krl.requests) >= krl.maxEntries {
		return false
	}

	now := time.Now()
	cutoff := now.Add(-krl.window)
	ts := krl.requests[key]
	valid := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= krl.limit {
		krl.requests[key] = valid
		return false
	}
	krl.requests[key] = append(valid, now)
	return true
}

// startCleanup periodically removes map entries with no recent requests
// to prevent unbounded memory growth from unique keys that stop sending.
func (krl *keyRateLimiter) startCleanup(ctx context.Context) {
	if krl.limit <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				krl.mu.Lock()
				now := time.Now()
				cutoff := now.Add(-krl.window)
				for key, ts := range krl.requests {
					allExpired := true
					for _, t := range ts {
						if t.After(cutoff) {
							allExpired = false
							break
						}
					}
					if allExpired {
						delete(krl.requests, key)
					}
				}
				krl.mu.Unlock()
			}
		}
	}()
}

type ProxyHandler struct {
	cache                   *auth.AuthCache
	dispatcher              *span.SpanDispatcher
	providerClient          *http.Client
	defaultAnthropicVersion string
	allowPrivateIPs         bool
	keyLimiter              *keyRateLimiter
	transports              *transportCache
}

// defaultMaskers is the set of PII maskers applied to request bodies when masking is enabled.
var defaultMaskers = []masking.Masker{&masking.PhoneMasker{}}

// safeMask applies masking with panic recovery. Returns nil, false if masking panics.
// Fail-open: proxy forwards original body unmasked on any masking error (D-16).
func safeMask(body []byte, cfg masking.MaskingConfig, maskers []masking.Masker) (result *masking.MaskResult, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("masking panic, failing open", "error", r)
			ok = false
		}
	}()
	return masking.ApplyMasking(body, cfg, maskers), true
}

// NewProxyHandler creates a ProxyHandler with the given auth cache, span dispatcher,
// provider request timeout, default anthropic API version, and per-key rate limit.
// perKeyRateLimit <= 0 disables per-key rate limiting.
func NewProxyHandler(ctx context.Context, cache *auth.AuthCache, dispatcher *span.SpanDispatcher, providerTimeout time.Duration, defaultAnthropicVersion string, allowPrivateIPs bool, perKeyRateLimit int) *ProxyHandler {
	kl := newKeyRateLimiter(perKeyRateLimit, time.Minute)
	kl.startCleanup(ctx)
	return &ProxyHandler{
		cache:      cache,
		dispatcher: dispatcher,
		providerClient: &http.Client{
			Timeout: providerTimeout,
		},
		defaultAnthropicVersion: defaultAnthropicVersion,
		allowPrivateIPs:         allowPrivateIPs,
		keyLimiter:              kl,
		transports:              &transportCache{cache: make(map[string]*http.Transport), maxEntries: 100},
	}
}

// ServeHTTP handles incoming proxy requests. It validates the API key, checks endpoint
// compatibility, and forwards the request to the appropriate provider.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only POST is allowed (defense-in-depth — chi router also enforces this)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{
			Error:   "method_not_allowed",
			Message: "Only POST requests are supported",
		})
		return
	}

	// Extract API key from Authorization header
	rawKey := extractAPIKey(r)
	if rawKey == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error:   "missing_api_key",
			Message: "Authorization header with AgentSpan API key required",
		})
		return
	}

	// Look up key in cache (may call Processing)
	result, err := h.cache.Lookup(r.Context(), rawKey)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "Authentication service temporarily unavailable",
		})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{
			Error:   "no_provider_config",
			Message: "Could not determine provider configuration",
		})
		return
	}

	if !result.Valid {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Error:   "invalid_api_key",
			Message: result.Reason,
		})
		return
	}

	// Per-key rate limiting (uses API key ID, not raw key, to avoid storing secrets in memory)
	if !h.keyLimiter.allow(result.APIKeyID) {
		metrics.RateLimitRejections.Inc()
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, errorResponse{
			Error:   "rate_limit_exceeded",
			Message: "Too many requests for this API key",
		})
		return
	}

	// Validate endpoint compatibility
	if r.URL.Path == "/v1/messages" && result.ProviderType != "anthropic" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "endpoint_mismatch",
			Message: "This provider does not support /v1/messages endpoint",
		})
		return
	}
	if r.URL.Path == "/v1/chat/completions" && result.ProviderType == "anthropic" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "endpoint_mismatch",
			Message: "Anthropic provider requires /v1/messages endpoint",
		})
		return
	}

	h.forward(w, r, result)
}

// responseMetadata holds parsed model and token usage data from a provider response.
type responseMetadata struct {
	Model        string
	InputTokens  int
	OutputTokens int
	FinishReason string
}

// parseNonSSEResponse extracts model and usage from a non-streaming provider response body.
// Returns zero values on any parse failure (defensive).
func parseNonSSEResponse(body []byte, providerType string) responseMetadata {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return responseMetadata{}
	}

	var meta responseMetadata

	// Extract model
	if modelRaw, ok := raw["model"]; ok {
		if err := json.Unmarshal(modelRaw, &meta.Model); err != nil {
			slog.Warn("parse response: failed to parse model", "error", err)
		}
	}

	// Extract usage based on provider type
	usageRaw, ok := raw["usage"]
	if !ok {
		return meta
	}

	var usage map[string]json.RawMessage
	if err := json.Unmarshal(usageRaw, &usage); err != nil {
		return meta
	}

	if providerType == "anthropic" {
		if v, ok := usage["input_tokens"]; ok {
			if err := json.Unmarshal(v, &meta.InputTokens); err != nil {
				slog.Warn("parse response: failed to parse input_tokens", "error", err)
			}
		}
		if v, ok := usage["output_tokens"]; ok {
			if err := json.Unmarshal(v, &meta.OutputTokens); err != nil {
				slog.Warn("parse response: failed to parse output_tokens", "error", err)
			}
		}
	} else {
		// OpenAI-compatible: openai, deepseek, mistral, groq, gemini, custom
		if v, ok := usage["prompt_tokens"]; ok {
			if err := json.Unmarshal(v, &meta.InputTokens); err != nil {
				slog.Warn("parse response: failed to parse prompt_tokens", "error", err)
			}
		}
		if v, ok := usage["completion_tokens"]; ok {
			if err := json.Unmarshal(v, &meta.OutputTokens); err != nil {
				slog.Warn("parse response: failed to parse completion_tokens", "error", err)
			}
		}
	}

	// Extract finish_reason (parsed before usage block for Anthropic, after for OpenAI).
	if providerType == "anthropic" {
		// Anthropic: top-level "stop_reason" field.
		if srRaw, ok := raw["stop_reason"]; ok {
			if err := json.Unmarshal(srRaw, &meta.FinishReason); err != nil {
				slog.Warn("parse response: failed to parse stop_reason", "error", err)
			}
		}
	} else {
		// OpenAI-compatible: choices[0].finish_reason.
		if choicesRaw, ok := raw["choices"]; ok {
			var choices []map[string]json.RawMessage
			if json.Unmarshal(choicesRaw, &choices) == nil && len(choices) > 0 {
				if fr, ok := choices[0]["finish_reason"]; ok {
					if err := json.Unmarshal(fr, &meta.FinishReason); err != nil {
						slog.Warn("parse response: failed to parse finish_reason", "error", err)
					}
				}
			}
		}
	}

	return meta
}

// parseSSELastChunk extracts model and usage from the last meaningful SSE data line.
// For OpenAI-compatible providers, usage may appear in the final chunk (stream_options.include_usage).
// For Anthropic, usage appears in the message_delta event.
// Returns zero values if usage not found.
func parseSSELastChunk(lastDataLine string, providerType string) responseMetadata {
	if lastDataLine == "" {
		return responseMetadata{}
	}

	return parseNonSSEResponse([]byte(lastDataLine), providerType)
}

// forward builds the upstream request, sends it to the provider, streams the response
// back to the agent, and dispatches a span payload.
func (h *ProxyHandler) forward(w http.ResponseWriter, r *http.Request, result *auth.AuthVerifyResult) {
	startedAt := time.Now()

	// Capture AgentSpan headers before forwarding, with length limits and sanitization.
	externalSessionID := sanitizeHeader(r.Header.Get("X-AgentSpan-Session"), 256)
	agentName := sanitizeHeader(r.Header.Get("X-AgentSpan-Agent"), 128)

	// CRLF injection defense-in-depth: blank values containing CR/LF before they reach span payload.
	if containsCRLF(externalSessionID) {
		externalSessionID = ""
	}
	if containsCRLF(agentName) {
		agentName = ""
	}

	// Read request body for span input capture (max 10MB to prevent OOM).
	// MaxBytesReader returns an error immediately when the limit is exceeded,
	// preventing the full body from being buffered in memory.
	const maxBodySize = 10 * 1024 * 1024
	const maxOutputCapture = 10 * 1024 * 1024 // max output captured for span (10MB)
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	inputBody, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
				Error:   "request_too_large",
				Message: "Request body exceeds 10MB limit",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "bad_request",
			Message: "Failed to read request body",
		})
		return
	}

	// Parse masking config from auth result
	var maskCfg masking.MaskingConfig
	if result.MaskingConfig != nil {
		_ = json.Unmarshal(result.MaskingConfig, &maskCfg) // ignore error, defaults to all off
	}

	// Save original body before masking for span dispatch (needed for LLM Only mode D-10)
	originalInputBody := make([]byte, len(inputBody))
	copy(originalInputBody, inputBody)

	var maskResult *masking.MaskResult
	maskingActive := false

	// Only mask if storing content AND a masking mode is enabled (D-03)
	if result.StoreSpanContent {
		if mr, ok := safeMask(inputBody, maskCfg, defaultMaskers); ok && mr != nil && len(mr.Entries) > 0 {
			maskResult = mr
			maskingActive = true
			inputBody = maskResult.Content // use masked body for upstream request
		}
	}

	// Build upstream URL with validation (SSRF prevention)
	baseURL, err := url.Parse(result.BaseURL)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		writeJSON(w, http.StatusBadGateway, errorResponse{
			Error:   "invalid_provider_url",
			Message: "Invalid provider base URL scheme",
		})
		return
	}
	// SSRF prevention: resolve DNS once and block requests to private/internal IP addresses.
	// The resolved IPs are pinned and used for the actual connection to prevent DNS rebinding.
	resolvedIPs, isPrivate, dnsErr := resolveAndCheckHost(baseURL.Hostname())
	if dnsErr != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{
			Error:   "dns_resolution_failed",
			Message: "Could not resolve provider hostname",
		})
		return
	}
	if !h.allowPrivateIPs && isPrivate {
		writeJSON(w, http.StatusForbidden, errorResponse{
			Error:   "forbidden_host",
			Message: "Provider URL points to a private or reserved IP address",
		})
		return
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + r.URL.Path
	if len(r.URL.RawQuery) > 8192 {
		writeJSON(w, http.StatusRequestURITooLong, errorResponse{
			Error:   "query_string_too_long",
			Message: "Query string exceeds 8192 byte limit",
		})
		return
	}
	baseURL.RawQuery = r.URL.RawQuery

	// Create outbound request with captured body
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, baseURL.String(), bytes.NewReader(inputBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{
			Error:   "internal_error",
			Message: "Failed to create upstream request",
		})
		return
	}

	// Copy headers, skipping Authorization, Host, X-Agentspan-*, X-Forwarded-*, and hop-by-hop headers
	for key, values := range r.Header {
		keyLower := strings.ToLower(key)
		if keyLower == "authorization" || keyLower == "host" {
			continue
		}
		if strings.HasPrefix(keyLower, "x-agentspan-") {
			continue
		}
		if strings.HasPrefix(keyLower, "x-forwarded-") {
			continue
		}
		if isHopByHop(keyLower) {
			continue
		}
		for _, v := range values {
			outReq.Header.Add(key, v)
		}
	}

	// Set provider-specific auth header
	if result.ProviderType == "anthropic" {
		outReq.Header.Set("x-api-key", result.ProviderKey)
		// Remove any Authorization header that may have been copied
		outReq.Header.Del("Authorization")
		// Set default anthropic-version if agent did not provide it
		if outReq.Header.Get("anthropic-version") == "" {
			outReq.Header.Set("anthropic-version", h.defaultAnthropicVersion)
		}
	} else {
		outReq.Header.Set("Authorization", "Bearer "+result.ProviderKey)
	}

	// Record proxy overhead (auth + routing) before upstream call
	metrics.ProxyOverheadDuration.WithLabelValues(r.URL.Path).Observe(time.Since(startedAt).Seconds())

	// Execute upstream request with pinned DNS to prevent rebinding attacks.
	// Cached transports reuse connection pools per resolved IP.
	upstreamStart := time.Now()
	client := h.providerClient
	if len(resolvedIPs) > 0 {
		client = &http.Client{
			Timeout:   client.Timeout,
			Transport: h.transports.getOrCreate(resolvedIPs),
		}
	}
	resp, err := client.Do(outReq)
	if err != nil {
		if isTimeout(err) {
			metrics.UpstreamErrors.WithLabelValues(result.ProviderType, "timeout").Inc()
			writeJSON(w, http.StatusGatewayTimeout, errorResponse{
				Error:   "upstream_timeout",
				Message: "Provider did not respond within timeout",
			})
			return
		}
		metrics.UpstreamErrors.WithLabelValues(result.ProviderType, "connection").Inc()
		writeJSON(w, http.StatusBadGateway, errorResponse{
			Error:   "upstream_error",
			Message: "Failed to reach provider",
		})
		return
	}
	defer resp.Body.Close()

	// Determine if response is SSE
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	// copyResponseHeaders copies provider response headers, filtering hop-by-hop and security-sensitive headers.
	copyResponseHeaders := func() {
		for key, values := range resp.Header {
			keyLower := strings.ToLower(key)
			if isHopByHop(keyLower) || isFilteredResponseHeader(keyLower) {
				continue
			}
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
	}

	var meta responseMetadata
	var outputStr string

	if isSSE {
		// SSE: must stream immediately, so send headers before reading body
		copyResponseHeaders()
		w.WriteHeader(resp.StatusCode)

		flusher, canFlush := w.(http.Flusher)
		var outputBuf bytes.Buffer
		var lastDataLine string
		outputCaptureFull := false

		scanner := bufio.NewScanner(resp.Body)
		// Increase scanner buffer for potentially large SSE lines
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		clientDisconnected := false
		for scanner.Scan() {
			line := scanner.Text()
			// Unmask SSE data lines before writing to agent (D-18)
			lineBytes := []byte(line + "\n")
			if maskingActive && maskResult != nil {
				lineBytes = masking.UnmaskContent(lineBytes, maskResult.Entries)
			}
			if _, writeErr := w.Write(lineBytes); writeErr != nil {
				clientDisconnected = true
				// Close provider connection immediately to stop streaming.
				// The deferred resp.Body.Close() becomes a no-op after this.
				resp.Body.Close()
				break
			}
			if canFlush {
				flusher.Flush()
			}

			// Only capture data lines (skip SSE comments, event/id/retry lines) to reduce memory.
			if !outputCaptureFull && strings.HasPrefix(line, "data: ") {
				outputBuf.WriteString(line)
				outputBuf.WriteByte('\n')
				if outputBuf.Len() > maxOutputCapture {
					outputCaptureFull = true
				}
			}

			// Track last data line containing JSON (skip [DONE])
			if strings.HasPrefix(line, "data: {") {
				lastDataLine = strings.TrimPrefix(line, "data: ")
			}
		}

		if !clientDisconnected {
			if err := scanner.Err(); err != nil {
				slog.Error("SSE stream read error", "error", err)
			}
		}

		meta = parseSSELastChunk(lastDataLine, result.ProviderType)
		outputStr = outputBuf.String()
	} else {
		// Non-SSE: read entire body first so we can return 502 on failure
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxOutputCapture))
		if readErr != nil {
			slog.Error("failed to read provider response body", "error", readErr)
			writeJSON(w, http.StatusBadGateway, errorResponse{
				Error:   "upstream_read_error",
				Message: "Failed to read provider response",
			})
			return
		}
		// Unmask non-SSE response before writing to agent (D-14)
		if maskingActive && maskResult != nil {
			bodyBytes = masking.UnmaskContent(bodyBytes, maskResult.Entries)
		}
		copyResponseHeaders()
		w.WriteHeader(resp.StatusCode)
		w.Write(bodyBytes)
		flusher, canFlush := w.(http.Flusher)
		if canFlush {
			flusher.Flush()
		}
		meta = parseNonSSEResponse(bodyBytes, result.ProviderType)
		outputStr = string(bodyBytes)
	}

	durationMs := time.Since(startedAt).Milliseconds()
	upstreamDuration := time.Since(upstreamStart)

	// Record upstream and overall metrics
	metrics.UpstreamRequestDuration.WithLabelValues(result.ProviderType, meta.Model).Observe(upstreamDuration.Seconds())
	metrics.ObserveRequest(r.URL.Path, result.ProviderType, resp.StatusCode, time.Since(startedAt))
	if resp.StatusCode >= 400 {
		metrics.UpstreamErrors.WithLabelValues(result.ProviderType, "http_error").Inc()
	}
	if meta.InputTokens > 0 {
		metrics.TokensProcessed.WithLabelValues("input", result.ProviderType).Add(float64(meta.InputTokens))
	}
	if meta.OutputTokens > 0 {
		metrics.TokensProcessed.WithLabelValues("output", result.ProviderType).Add(float64(meta.OutputTokens))
	}

	// Determine span text by masking mode (D-02, D-10, D-11)
	var spanInput, spanOutput string
	if !result.StoreSpanContent {
		// Metadata-only: no text stored (D-02)
		spanInput = ""
		spanOutput = ""
	} else if maskingActive && maskResult != nil {
		if maskCfg.Phone == masking.MaskModeLLMOnly {
			// LLM Only (D-10): store ORIGINAL text
			spanInput = extractInputText(originalInputBody, result.ProviderType)
			spanOutput = extractOutputText(outputStr, result.ProviderType, isSSE)
		} else if maskCfg.Phone == masking.MaskModeLLMStorage {
			// LLM + Storage (D-11): store MASKED text
			spanInput = extractInputText(inputBody, result.ProviderType) // inputBody is masked
			// Re-mask output for storage using the SAME mask entries from request.
			// This ensures consistent masked values (same phone -> same masked value).
			remaskedOutput := []byte(extractOutputText(outputStr, result.ProviderType, isSSE))
			for _, entry := range maskResult.Entries {
				remaskedOutput = bytes.ReplaceAll(remaskedOutput, []byte(entry.Original), []byte(entry.Masked))
			}
			spanOutput = string(remaskedOutput)
		} else {
			// Fallback: no masking mode matched but entries found (shouldn't happen)
			spanInput = extractInputText(originalInputBody, result.ProviderType)
			spanOutput = extractOutputText(outputStr, result.ProviderType, isSSE)
		}
	} else {
		// No masking: normal path
		spanInput = extractInputText(originalInputBody, result.ProviderType)
		spanOutput = extractOutputText(outputStr, result.ProviderType, isSSE)
	}

	// Build span payload with masking metadata
	payload := span.SpanPayload{
		APIKeyID:          result.APIKeyID,
		OrganizationID:    result.OrganizationID,
		ProviderType:      result.ProviderType,
		Model:             meta.Model,
		Input:             spanInput,
		Output:            spanOutput,
		InputTokens:       meta.InputTokens,
		OutputTokens:      meta.OutputTokens,
		DurationMs:        durationMs,
		HTTPStatus:        resp.StatusCode,
		StartedAt:         startedAt.UTC().Format(time.RFC3339Nano),
		FinishReason:      meta.FinishReason,
		ExternalSessionID: externalSessionID,
		AgentName:         agentName,
		MaskingApplied:    maskingActive,
	}

	// Only include masking map for LLM Only mode (D-10) — allows audit trail
	// For LLM+Storage (D-11): no masking map sent — PII never persisted
	if maskingActive && maskResult != nil && maskCfg.Phone == masking.MaskModeLLMOnly {
		for _, e := range maskResult.Entries {
			payload.MaskingMap = append(payload.MaskingMap, span.MaskingMapEntry{
				MaskType:      string(e.MaskType),
				OriginalValue: e.Original,
				MaskedValue:   e.Masked,
			})
		}
	}

	// Dispatch span asynchronously after response is fully written
	h.dispatcher.Dispatch(payload)
}

// extractAPIKey extracts the raw API key from the Authorization: Bearer header.
// Returns empty string if the header is missing or doesn't have the as- prefix.
func extractAPIKey(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	key := parts[1]
	if !strings.HasPrefix(key, "as-") || len(key) != 35 {
		return ""
	}

	// Validate charset: as- prefix + 32 lowercase hex characters
	for _, c := range key[3:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}

	return key
}

// isTimeout checks if an error is a timeout error.
func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// hopByHopHeaders are headers that must not be forwarded by proxies (RFC 2616 §13.5.1).
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

func isHopByHop(headerLower string) bool {
	return hopByHopHeaders[headerLower]
}

// filteredResponseHeaders are provider response headers that should not be forwarded
// to the agent (may leak internal infrastructure details).
var filteredResponseHeaders = map[string]bool{
	"server":       true,
	"x-debug":      true,
	"x-powered-by": true,
	"via":          true,
	"x-cache":      true,
	"x-served-by":  true,
	// Prevent provider infrastructure leakage
	"x-ratelimit-limit":     true,
	"x-ratelimit-remaining": true,
	"x-ratelimit-reset":     true,
	"x-request-id":          true,
}

func isFilteredResponseHeader(headerLower string) bool {
	return filteredResponseHeaders[headerLower]
}

// errorResponse is the JSON error format returned by the proxy.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// containsCRLF checks if a string contains CR or LF characters (CRLF injection defense).
func containsCRLF(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}

// writeJSON writes a JSON response with the given status code.
// sanitizeHeader truncates a header value to maxLen and strips control characters.
func sanitizeHeader(value string, maxLen int) string {
	if len(value) > maxLen {
		value = value[:maxLen]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1 // strip control characters
		}
		return r
	}, value)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode error", "error", err)
	}
}

// privateNetworks lists CIDR ranges that should be blocked as provider URLs (SSRF prevention).
var privateNetworks = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",      // current network (often aliases localhost)
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"100.64.0.0/10",  // carrier-grade NAT / cloud metadata ranges
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, _ := net.ParseCIDR(cidr)
		nets = append(nets, n)
	}
	return nets
}()

// resolveAndCheckHost resolves the hostname and returns the resolved IPs.
// Returns (ips, isPrivate, err). If any resolved IP is private, isPrivate is true.
// Returns error on DNS resolution failure to prevent SSRF bypass via DNS rebinding.
// For literal IPs, returns the IP directly without DNS lookup.
func resolveAndCheckHost(host string) ([]net.IP, bool, error) {
	// First try direct IP parse (avoids DNS lookup for literal IPs)
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, isPrivateIP(ip), nil
	}
	// Resolve hostname and check all IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, false, fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, false, fmt.Errorf("dns lookup returned no IPs for %s", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return ips, true, nil
		}
	}
	return ips, false, nil
}

// pinnedDialContext returns a DialContext that connects to the pre-resolved IP
// instead of resolving DNS again, preventing DNS rebinding attacks.
func pinnedDialContext(resolvedIPs []net.IP) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if len(resolvedIPs) == 0 {
			return nil, fmt.Errorf("no resolved IPs for pinning %s", addr)
		}
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		pinnedAddr := net.JoinHostPort(resolvedIPs[0].String(), port)
		var d net.Dialer
		return d.DialContext(ctx, network, pinnedAddr)
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
