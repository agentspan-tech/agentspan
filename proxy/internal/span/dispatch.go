package span

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agentspan/proxy/internal/metrics"
)

// MaskingMapEntry records one original-to-masked replacement for span storage.
// Only populated for LLM Only mode (D-10).
type MaskingMapEntry struct {
	MaskType      string `json:"mask_type"`
	OriginalValue string `json:"original_value"`
	MaskedValue   string `json:"masked_value"`
}

// SpanPayload is the data sent to Processing for span ingestion.
// Fields match SpanIngestRequest on the Processing side.
type SpanPayload struct {
	APIKeyID          string            `json:"api_key_id"`
	OrganizationID    string            `json:"organization_id"`
	ProviderType      string            `json:"provider_type"`
	Model             string            `json:"model"`
	Input             string            `json:"input"`
	Output            string            `json:"output"`
	InputTokens       int               `json:"input_tokens"`
	OutputTokens      int               `json:"output_tokens"`
	DurationMs        int64             `json:"duration_ms"`
	HTTPStatus        int               `json:"http_status"`
	StartedAt         string            `json:"started_at"`
	FinishReason      string            `json:"finish_reason,omitempty"`
	ExternalSessionID string            `json:"external_session_id,omitempty"`
	AgentName         string            `json:"agent_name,omitempty"`
	MaskingApplied    bool              `json:"masking_applied"`
	MaskingMap        []MaskingMapEntry `json:"masking_map,omitempty"`
}

// SpanDispatcher sends span payloads to Processing asynchronously via a buffered channel.
// When the channel is full, payloads are dropped silently (fail-open).
type SpanDispatcher struct {
	ch            chan SpanPayload
	processingURL string
	internalToken string
	client        *http.Client
	dropped       atomic.Int64
	sendTimeout   time.Duration
	drainTimeout  time.Duration
	numWorkers    int
}

// NewSpanDispatcher creates a new SpanDispatcher with the given buffer size and send timeout.
// drainTimeout controls how long to wait for buffered spans on shutdown (0 defaults to 5s).
// numWorkers controls how many concurrent worker goroutines send spans (0 defaults to 3).
func NewSpanDispatcher(processingURL, internalToken string, bufferSize int, client *http.Client, sendTimeout time.Duration, drainTimeout time.Duration, numWorkers int) *SpanDispatcher {
	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Second
	}
	if numWorkers <= 0 {
		numWorkers = 3
	}
	return &SpanDispatcher{
		ch:            make(chan SpanPayload, bufferSize),
		processingURL: processingURL,
		internalToken: internalToken,
		client:        client,
		sendTimeout:   sendTimeout,
		drainTimeout:  drainTimeout,
		numWorkers:    numWorkers,
	}
}

// Dispatch enqueues a span payload for async delivery. Non-blocking: if the
// channel is full, the payload is dropped and the dropped counter increments.
func (d *SpanDispatcher) Dispatch(payload SpanPayload) {
	select {
	case d.ch <- payload:
		metrics.SpansDispatched.Inc()
		metrics.SpanBufferUsage.Set(float64(len(d.ch)))
	default:
		count := d.dropped.Add(1)
		metrics.SpansDropped.Inc()
		if count == 1 || count%100 == 0 {
			slog.Warn("span buffer full", "dropped_total", count)
		}
	}
}

// Dropped returns the total number of payloads dropped due to a full channel.
func (d *SpanDispatcher) Dropped() int64 {
	return d.dropped.Load()
}

// DrainOne reads one payload from the channel without blocking.
// Returns the payload and true if one was available, or zero value and false otherwise.
// Intended for use in tests to inspect dispatched spans.
func (d *SpanDispatcher) DrainOne() (SpanPayload, bool) {
	select {
	case p := <-d.ch:
		return p, true
	default:
		return SpanPayload{}, false
	}
}

// Start launches numWorkers goroutines that read from the channel and send
// payloads to Processing. When ctx is cancelled, a separate goroutine drains
// remaining spans with a configurable timeout before exiting.
func (d *SpanDispatcher) Start(ctx context.Context) {
	for i := 0; i < d.numWorkers; i++ {
		go func() {
			for {
				select {
				case payload := <-d.ch:
					d.send(payload)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	// Drain goroutine — runs after workers exit on ctx cancellation
	go func() {
		<-ctx.Done()
		d.drain()
	}()
}

// drain sends remaining buffered spans with a timeout.
func (d *SpanDispatcher) drain() {
	deadline := time.After(d.drainTimeout)
	count := 0
	for {
		select {
		case payload := <-d.ch:
			d.send(payload)
			count++
		case <-deadline:
			remaining := len(d.ch)
			if count > 0 || remaining > 0 {
				slog.Info("span dispatcher shutdown", "drained", count, "lost", remaining)
			}
			return
		default:
			if count > 0 {
				slog.Info("span dispatcher shutdown", "drained", count, "lost", 0)
			}
			return
		}
	}
}

// send POSTs a span payload to Processing's /internal/spans/ingest endpoint.
// Errors are logged but do not affect proxy operation (fail-open).
func (d *SpanDispatcher) send(payload SpanPayload) {
	start := time.Now()

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("span dispatch marshal failed", "error", err)
		metrics.SpanSendErrors.Inc()
		return
	}

	sendCtx, cancel := context.WithTimeout(context.Background(), d.sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, strings.TrimRight(d.processingURL, "/")+"/internal/spans/ingest", bytes.NewReader(body))
	if err != nil {
		slog.Error("span dispatch request creation failed", "error", err)
		metrics.SpanSendErrors.Inc()
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", d.internalToken)

	resp, err := d.client.Do(req)
	if err != nil {
		slog.Error("span dispatch send failed", "error", err)
		metrics.SpanSendErrors.Inc()
		return
	}
	defer resp.Body.Close()

	metrics.SpanSendDuration.Observe(time.Since(start).Seconds())
	metrics.SpanBufferUsage.Set(float64(len(d.ch)))

	if resp.StatusCode >= 400 {
		slog.Warn("span dispatch unexpected status", "status", resp.StatusCode)
		metrics.SpanSendErrors.Inc()
	} else {
		metrics.SpansSent.Inc()
	}
}
