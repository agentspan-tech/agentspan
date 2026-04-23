package hub_test

import (
	"sync"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/google/uuid"
)

func TestPublishToSubscriber(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub := h.Subscribe(orgID, "sessions")
	defer h.Unsubscribe(sub)

	evt := hub.Event{Type: "session.created", Payload: "test"}
	h.Publish(orgID, "sessions", evt)

	select {
	case received := <-sub.Ch:
		if received.Type != "session.created" {
			t.Errorf("got type %q, want %q", received.Type, "session.created")
		}
		if received.Payload != "test" {
			t.Errorf("got payload %v, want %q", received.Payload, "test")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishToMultipleSubscribers(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub1 := h.Subscribe(orgID, "sessions")
	sub2 := h.Subscribe(orgID, "sessions")
	defer h.Unsubscribe(sub1)
	defer h.Unsubscribe(sub2)

	evt := hub.Event{Type: "session.created", Payload: "multi"}
	h.Publish(orgID, "sessions", evt)

	for i, sub := range []*hub.Subscription{sub1, sub2} {
		select {
		case received := <-sub.Ch:
			if received.Type != "session.created" {
				t.Errorf("subscriber %d: got type %q, want %q", i, received.Type, "session.created")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestPublishToDifferentTopics(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	subA := h.Subscribe(orgID, "topicA")
	subB := h.Subscribe(orgID, "topicB")
	defer h.Unsubscribe(subA)
	defer h.Unsubscribe(subB)

	h.Publish(orgID, "topicA", hub.Event{Type: "for-a"})

	// subA should receive
	select {
	case received := <-subA.Ch:
		if received.Type != "for-a" {
			t.Errorf("subA: got type %q, want %q", received.Type, "for-a")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subA: timed out waiting for event")
	}

	// subB should NOT receive
	select {
	case evt := <-subB.Ch:
		t.Errorf("subB received unexpected event: %+v", evt)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestPublishToDifferentOrgs(t *testing.T) {
	h := hub.New()
	orgA := uuid.New()
	orgB := uuid.New()
	subA := h.Subscribe(orgA, "sessions")
	subB := h.Subscribe(orgB, "sessions")
	defer h.Unsubscribe(subA)
	defer h.Unsubscribe(subB)

	h.Publish(orgA, "sessions", hub.Event{Type: "for-orgA"})

	select {
	case received := <-subA.Ch:
		if received.Type != "for-orgA" {
			t.Errorf("subA: got type %q, want %q", received.Type, "for-orgA")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subA: timed out")
	}

	select {
	case evt := <-subB.Ch:
		t.Errorf("subB received unexpected event: %+v", evt)
	case <-time.After(50 * time.Millisecond):
		// expected — different org
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub := h.Subscribe(orgID, "test")

	h.Unsubscribe(sub)

	// Channel should be closed: receive returns zero value with ok=false
	_, ok := <-sub.Ch
	if ok {
		t.Error("expected channel to be closed after Unsubscribe")
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub := h.Subscribe(orgID, "test")

	// Calling Unsubscribe twice must not panic
	h.Unsubscribe(sub)
	h.Unsubscribe(sub)
}

func TestPublishAfterUnsubscribe(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub := h.Subscribe(orgID, "test")
	h.Unsubscribe(sub)

	// Publishing after unsubscribe must not panic
	h.Publish(orgID, "test", hub.Event{Type: "after-unsub"})
}

func TestPublishNonBlocking(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	sub := h.Subscribe(orgID, "test")
	defer h.Unsubscribe(sub)

	// Fill buffer (capacity 64)
	for i := 0; i < 64; i++ {
		h.Publish(orgID, "test", hub.Event{Type: "fill"})
	}

	// This must not block
	done := make(chan struct{})
	go func() {
		h.Publish(orgID, "test", hub.Event{Type: "overflow"})
		close(done)
	}()
	select {
	case <-done:
		// OK — publish was non-blocking
	case <-time.After(1 * time.Second):
		t.Fatal("Publish blocked on full channel")
	}
}

func TestConcurrentSubscribePublish(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	topic := "concurrent"

	// Phase 1: concurrent subscribes + publishes (no unsubscribe during publish
	// to avoid the known close/send race that Publish handles via recover).
	var subs []*hub.Subscription
	var subsMu sync.Mutex

	var wg sync.WaitGroup

	// 10 subscriber goroutines — subscribe many times concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sub := h.Subscribe(orgID, topic)
				subsMu.Lock()
				subs = append(subs, sub)
				subsMu.Unlock()
			}
		}()
	}

	// 10 publisher goroutines — publish concurrently with subscribes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Publish(orgID, topic, hub.Event{Type: "concurrent", Payload: j})
			}
		}()
	}

	wg.Wait()

	// Phase 2: clean up all subscriptions (sequential, after publishers done)
	for _, sub := range subs {
		h.Unsubscribe(sub)
	}
}

func TestSubscribeCleanup(t *testing.T) {
	h := hub.New()
	orgID := uuid.New()
	topic := "cleanup"

	// Subscribe and unsubscribe — internal map should be cleaned up
	sub1 := h.Subscribe(orgID, topic)
	h.Unsubscribe(sub1)

	// Subscribe again — should work with a fresh entry
	sub2 := h.Subscribe(orgID, topic)
	defer h.Unsubscribe(sub2)

	evt := hub.Event{Type: "after-cleanup"}
	h.Publish(orgID, topic, evt)

	select {
	case received := <-sub2.Ch:
		if received.Type != "after-cleanup" {
			t.Errorf("got type %q, want %q", received.Type, "after-cleanup")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event after cleanup")
	}
}
