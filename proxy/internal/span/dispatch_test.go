package span

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchSendsToChannel(t *testing.T) {
	d := NewSpanDispatcher("http://unused", "tok", 10, http.DefaultClient, 10*time.Second, 0, 1)
	payload := SpanPayload{APIKeyID: "key-1", OrganizationID: "org-1"}
	d.Dispatch(payload)

	select {
	case got := <-d.ch:
		if got.APIKeyID != "key-1" || got.OrganizationID != "org-1" {
			t.Errorf("unexpected payload: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for payload on channel")
	}
}

func TestDispatchDropsOnFullChannel(t *testing.T) {
	d := NewSpanDispatcher("http://unused", "tok", 1, http.DefaultClient, 10*time.Second, 0, 1)
	// Fill the channel
	d.Dispatch(SpanPayload{APIKeyID: "first"})
	// This should be dropped
	d.Dispatch(SpanPayload{APIKeyID: "second"})

	if d.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", d.Dropped())
	}
}

func TestDispatchNonBlocking(t *testing.T) {
	// Unbuffered channel — Dispatch should still return immediately
	d := NewSpanDispatcher("http://unused", "tok", 0, http.DefaultClient, 10*time.Second, 0, 1)

	done := make(chan struct{})
	go func() {
		d.Dispatch(SpanPayload{APIKeyID: "x"})
		close(done)
	}()

	select {
	case <-done:
		// Good — non-blocking
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Dispatch blocked on full/unbuffered channel")
	}

	if d.Dropped() != 1 {
		t.Errorf("expected 1 dropped on unbuffered channel, got %d", d.Dropped())
	}
}

func TestWorkerPostsToProcessing(t *testing.T) {
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload SpanPayload
		json.NewDecoder(r.Body).Decode(&payload)
		received.Store(payload)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewSpanDispatcher(srv.URL, "tok", 10, srv.Client(), 10*time.Second, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	d.Dispatch(SpanPayload{APIKeyID: "key-1", OrganizationID: "org-1"})

	// Wait for worker to process
	deadline := time.After(2 * time.Second)
	for {
		if v := received.Load(); v != nil {
			p := v.(SpanPayload)
			if p.APIKeyID != "key-1" || p.OrganizationID != "org-1" {
				t.Errorf("unexpected payload: %+v", p)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for worker to POST")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkerSendsInternalToken(t *testing.T) {
	var receivedToken atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken.Store(r.Header.Get("X-Internal-Token"))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewSpanDispatcher(srv.URL, "my-internal-secret", 10, srv.Client(), 10*time.Second, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	d.Dispatch(SpanPayload{APIKeyID: "k"})

	deadline := time.After(2 * time.Second)
	for {
		if v := receivedToken.Load(); v != nil {
			tok := v.(string)
			if tok != "my-internal-secret" {
				t.Errorf("expected 'my-internal-secret', got '%s'", tok)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for token")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkerContinuesOnError(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewSpanDispatcher(srv.URL, "tok", 10, srv.Client(), 10*time.Second, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	d.Dispatch(SpanPayload{APIKeyID: "a"})
	d.Dispatch(SpanPayload{APIKeyID: "b"})

	deadline := time.After(2 * time.Second)
	for {
		if callCount.Load() >= 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected 2 calls, got %d", callCount.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestDrainOnShutdown(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// Use 1 worker and large buffer so items queue up
	d := NewSpanDispatcher(srv.URL, "tok", 100, srv.Client(), 10*time.Second, 2*time.Second, 1)
	ctx, cancel := context.WithCancel(context.Background())

	// Dispatch payloads BEFORE starting workers, so they sit in the channel
	for i := 0; i < 5; i++ {
		d.Dispatch(SpanPayload{APIKeyID: "key"})
	}

	// Start and immediately cancel — drain goroutine should process remaining items
	d.Start(ctx)
	cancel()

	// Wait for drain to complete
	time.Sleep(500 * time.Millisecond)

	count := callCount.Load()
	if count < 5 {
		t.Errorf("expected at least 5 calls (drain should send remaining), got %d", count)
	}
}

func TestMultipleWorkers(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate work
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewSpanDispatcher(srv.URL, "tok", 100, srv.Client(), 10*time.Second, 0, 5)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	// Dispatch 10 payloads
	for i := 0; i < 10; i++ {
		d.Dispatch(SpanPayload{APIKeyID: "key"})
	}

	// With 5 workers doing 50ms work, 10 items should complete in ~100ms
	deadline := time.After(2 * time.Second)
	for {
		if callCount.Load() >= 10 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected 10 calls, got %d", callCount.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestDefaultDrainTimeout(t *testing.T) {
	d := NewSpanDispatcher("http://unused", "tok", 10, http.DefaultClient, 10*time.Second, 0, 0)
	if d.drainTimeout != 5*time.Second {
		t.Errorf("drainTimeout = %v, want 5s", d.drainTimeout)
	}
	if d.numWorkers != 3 {
		t.Errorf("numWorkers = %d, want 3", d.numWorkers)
	}
}

func TestDrainOne(t *testing.T) {
	d := NewSpanDispatcher("http://unused", "tok", 10, http.DefaultClient, 10*time.Second, 0, 1)
	d.Dispatch(SpanPayload{APIKeyID: "key-1"})

	p, ok := d.DrainOne()
	if !ok {
		t.Fatal("expected payload from DrainOne")
	}
	if p.APIKeyID != "key-1" {
		t.Errorf("APIKeyID = %q, want key-1", p.APIKeyID)
	}

	// Second drain should return false
	_, ok = d.DrainOne()
	if ok {
		t.Error("expected false from DrainOne on empty channel")
	}
}

func TestWorkerStopsOnCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := NewSpanDispatcher(srv.URL, "tok", 10, srv.Client(), 10*time.Second, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)

	// Cancel context — worker should exit
	cancel()

	// Give worker time to notice cancellation
	time.Sleep(50 * time.Millisecond)

	// Dispatch should not block even though worker is stopped
	done := make(chan struct{})
	go func() {
		d.Dispatch(SpanPayload{APIKeyID: "x"})
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Dispatch blocked after worker stop")
	}
}
