package hub

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Event represents a real-time event published through the Hub.
type Event struct {
	Type    string      `json:"type"`    // "session.created", "session.updated", "span.created", "alert.triggered"
	Payload interface{} `json:"payload"`
}

// Subscription represents a subscriber to events on a specific (orgID, topic) key.
// Consumers read from Ch until it is closed (which happens on Unsubscribe or Hub shutdown).
type Subscription struct {
	Ch     chan Event
	done   atomic.Bool
	cancel func()
	key    string // "orgID:topic" — used for removal from Hub map
}

// Hub is an in-process pub/sub that fans out events to multiple subscribers
// keyed by (orgID, topic). It is safe for concurrent use.
//
// Design decisions (D-04):
//   - Buffered channels (capacity 64) absorb short bursts.
//   - Publish is non-blocking: slow subscribers have events dropped.
//   - Unsubscribe sets an atomic done flag before closing the channel
//     to prevent send-on-closed-channel panics in Publish.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*Subscription]struct{}
}

// New creates a new Hub ready to accept subscriptions.
func New() *Hub {
	return &Hub{
		subscribers: make(map[string]map[*Subscription]struct{}),
	}
}

// Subscribe registers a new subscriber for the given (orgID, topic) combination.
// The returned Subscription contains a buffered channel (capacity 64) from which
// the caller reads events. Call Unsubscribe (or the cancel func) to clean up.
func (h *Hub) Subscribe(orgID uuid.UUID, topic string) *Subscription {
	key := fmt.Sprintf("%s:%s", orgID.String(), topic)

	sub := &Subscription{
		Ch:  make(chan Event, 64),
		key: key,
	}
	sub.cancel = func() {
		// Ensure cancel is idempotent — only the first call does work.
		if !sub.done.CompareAndSwap(false, true) {
			return
		}
		h.mu.Lock()
		if subs, ok := h.subscribers[key]; ok {
			delete(subs, sub)
			if len(subs) == 0 {
				delete(h.subscribers, key)
			}
		}
		h.mu.Unlock()
		close(sub.Ch)
	}

	h.mu.Lock()
	if h.subscribers[key] == nil {
		h.subscribers[key] = make(map[*Subscription]struct{})
	}
	h.subscribers[key][sub] = struct{}{}
	h.mu.Unlock()

	return sub
}

// Unsubscribe removes a subscription from the Hub and closes its channel.
// It is safe to call multiple times.
func (h *Hub) Unsubscribe(sub *Subscription) {
	sub.cancel()
}

// Publish sends an event to all subscribers on the (orgID, topic) key.
// Non-blocking: if a subscriber's channel is full, the event is dropped for that subscriber.
// Safe against send-on-closed-channel: checks sub.done before sending.
func (h *Hub) Publish(orgID uuid.UUID, topic string, event Event) {
	key := fmt.Sprintf("%s:%s", orgID.String(), topic)

	h.mu.RLock()
	subs := h.subscribers[key]
	// Copy the set under RLock to avoid holding the lock during sends.
	targets := make([]*Subscription, 0, len(subs))
	for sub := range subs {
		targets = append(targets, sub)
	}
	h.mu.RUnlock()

	for _, sub := range targets {
		// Check done flag to avoid send on closed channel (Pitfall 2).
		if sub.done.Load() {
			continue
		}
		// Non-blocking send — drop if buffer is full.
		// Recover from potential send-on-closed-channel if Unsubscribe races
		// between the done check and the send.
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic guard for send-on-closed-channel
			select {
			case sub.Ch <- event:
			default:
			}
		}()
	}
}
