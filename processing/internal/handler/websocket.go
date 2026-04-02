package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentspan/processing/internal/db"
	"github.com/agentspan/processing/internal/hub"
	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// WSHandler handles WebSocket connections for real-time dashboard updates.
// Mounted at GET /cable. Auth via first message: {"type":"auth","token":"<JWT>"}.
type WSHandler struct {
	hub            *hub.Hub
	jwtSecret      string
	queries        *db.Queries
	connCount      atomic.Int64
	maxConns       int64
	originPatterns []string
}

// authMessage is the first message a client must send to authenticate.
type authMessage struct {
	Type  string `json:"type"`  // must be "auth"
	Token string `json:"token"` // JWT
}

// NewWSHandler creates a new WSHandler. maxConns limits concurrent WebSocket connections (0 = 1000).
// allowedOrigins is a comma-separated list of origins (same format as CORS config).
// Empty string means same-origin only (the coder/websocket default).
func NewWSHandler(h *hub.Hub, jwtSecret string, queries *db.Queries, allowedOrigins string) *WSHandler {
	var patterns []string
	if allowedOrigins != "" {
		for _, o := range strings.Split(allowedOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				patterns = append(patterns, o)
			}
		}
	}
	return &WSHandler{hub: h, jwtSecret: jwtSecret, queries: queries, maxConns: 1000, originPatterns: patterns}
}

// clientMessage is the JSON envelope sent by the browser client.
type clientMessage struct {
	Command    string            `json:"command"`    // "subscribe" | "unsubscribe"
	Identifier channelIdentifier `json:"identifier"`
}

// channelIdentifier identifies the channel and optional resource.
type channelIdentifier struct {
	Channel   string `json:"channel"`              // "SessionsChannel" | "SessionChannel"
	OrgID     string `json:"org_id,omitempty"`      // required for SessionsChannel
	SessionID string `json:"session_id,omitempty"`  // required for SessionChannel
}

// serverMessage is the JSON envelope sent to the browser client.
type serverMessage struct {
	Identifier channelIdentifier `json:"identifier"`
	Type       string            `json:"type"`
	Payload    interface{}       `json:"payload,omitempty"`
}

// connState tracks per-connection state: user identity, active subscriptions, merged event channel.
type connState struct {
	mu         sync.Mutex
	userID     uuid.UUID
	subs       map[string]*hub.Subscription // topic -> subscription
	merged     chan hub.Event               // all subscriptions forward here
	topics     map[string]channelIdentifier // topic -> identifier (for envelope)
	forwarders sync.WaitGroup               // tracks active forwarder goroutines
}

// ServeHTTP upgrades the connection to WebSocket, then authenticates via first message.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enforce global connection limit using CAS to prevent TOCTOU race.
	for {
		current := h.connCount.Load()
		if current >= h.maxConns {
			http.Error(w, "too many WebSocket connections", http.StatusServiceUnavailable)
			return
		}
		if h.connCount.CompareAndSwap(current, current+1) {
			break
		}
	}
	defer h.connCount.Add(-1)

	// 1. Upgrade to WebSocket (no auth required yet).
	// OriginPatterns restricts which origins can connect; empty = same-origin only.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		slog.Error("ws: accept error", "error", err)
		return
	}
	defer conn.CloseNow() //nolint:errcheck // best-effort close on exit

	// 2. Authenticate — try httpOnly cookie first, fall back to first message.
	var userID uuid.UUID
	if cookie, cookieErr := r.Cookie("agentspan_token"); cookieErr == nil && cookie.Value != "" {
		uid, parseErr := h.parseJWT(r.Context(), cookie.Value)
		if parseErr != nil {
			conn.Close(websocket.StatusPolicyViolation, "invalid cookie token")
			return
		}
		userID = uid
	} else {
		// Legacy: authenticate via first message (5s deadline).
		authCtx, authCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer authCancel()

		_, data, readErr := conn.Read(authCtx)
		if readErr != nil {
			conn.Close(websocket.StatusPolicyViolation, "auth timeout")
			return
		}

		var authMsg authMessage
		if err := json.Unmarshal(data, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			conn.Close(websocket.StatusPolicyViolation, "invalid auth message")
			return
		}

		uid, parseErr := h.parseJWT(r.Context(), authMsg.Token)
		if parseErr != nil {
			conn.Close(websocket.StatusPolicyViolation, "invalid token")
			return
		}
		userID = uid
	}

	slog.Info("ws: connected", "user", userID)

	// 3. Set up connection state.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	state := &connState{
		userID: userID,
		subs:   make(map[string]*hub.Subscription),
		merged: make(chan hub.Event, 128),
		topics: make(map[string]channelIdentifier),
	}

	// 4. Read loop in a goroutine; write loop blocks in main goroutine.
	go h.readLoop(ctx, conn, state, cancel)
	h.writeLoop(ctx, conn, state)

	// 5. Cleanup: wait for all forwarder goroutines to exit before unsubscribing.
	// ctx is already cancelled (readLoop defers cancel), so forwarders will exit promptly.
	state.forwarders.Wait()
	state.mu.Lock()
	for _, s := range state.subs {
		h.hub.Unsubscribe(s)
	}
	state.mu.Unlock()

	conn.Close(websocket.StatusNormalClosure, "")
}

// readLoop reads client subscribe/unsubscribe messages.
func (h *WSHandler) readLoop(ctx context.Context, conn *websocket.Conn, state *connState, cancel context.CancelFunc) {
	defer cancel()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // client disconnected or context cancelled
		}

		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue // ignore malformed messages
		}

		switch msg.Command {
		case "subscribe":
			h.handleSubscribe(ctx, conn, state, msg.Identifier)
		case "unsubscribe":
			h.handleUnsubscribe(state, msg.Identifier)
		}
	}
}

// writeLoop forwards hub events to the WebSocket connection.
func (h *WSHandler) writeLoop(ctx context.Context, conn *websocket.Conn, state *connState) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-state.merged:
			if !ok {
				return
			}
			// Marshal and send.
			data, err := json.Marshal(evt.Payload)
			if err != nil {
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// handleSubscribe processes a subscribe command.
func (h *WSHandler) handleSubscribe(ctx context.Context, conn *websocket.Conn, state *connState, id channelIdentifier) {
	var orgID uuid.UUID
	var topic string

	switch id.Channel {
	case "SessionsChannel":
		// Client must specify org_id; verify membership.
		parsed, err := uuid.Parse(id.OrgID)
		if err != nil {
			h.sendReject(ctx, conn, id, "invalid org_id")
			return
		}
		orgID = parsed

		// Verify membership (WS-04).
		_, err = h.queries.GetMembership(ctx, db.GetMembershipParams{
			OrganizationID: orgID,
			UserID:         state.userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				h.sendReject(ctx, conn, id, "not a member of this organization")
				return
			}
			h.sendReject(ctx, conn, id, "internal error")
			return
		}

		topic = "sessions_list:" + orgID.String()

	case "SessionChannel":
		// Client must specify session_id; verify membership in session's org.
		sessionID, err := uuid.Parse(id.SessionID)
		if err != nil {
			h.sendReject(ctx, conn, id, "invalid session_id")
			return
		}

		// Look up session's org_id.
		sessionOrgID, err := h.queries.GetSessionOrgID(ctx, sessionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				h.sendReject(ctx, conn, id, "not authorized")
				return
			}
			h.sendReject(ctx, conn, id, "internal error")
			return
		}
		orgID = sessionOrgID

		// Verify membership (WS-04).
		_, err = h.queries.GetMembership(ctx, db.GetMembershipParams{
			OrganizationID: orgID,
			UserID:         state.userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				h.sendReject(ctx, conn, id, "not authorized")
				return
			}
			h.sendReject(ctx, conn, id, "internal error")
			return
		}

		topic = "session:" + sessionID.String()

	default:
		h.sendReject(ctx, conn, id, "unknown channel")
		return
	}

	// Check if already subscribed to this topic.
	state.mu.Lock()
	if _, exists := state.subs[topic]; exists {
		state.mu.Unlock()
		return // already subscribed, no-op
	}

	// Enforce per-connection subscription limit to prevent resource exhaustion.
	const maxSubsPerConn = 10
	if len(state.subs) >= maxSubsPerConn {
		state.mu.Unlock()
		h.sendReject(ctx, conn, id, "too many subscriptions")
		return
	}

	// Subscribe to hub and store.
	sub := h.hub.Subscribe(orgID, topic)
	state.subs[topic] = sub
	state.topics[topic] = id
	state.mu.Unlock()

	// Launch forwarder goroutine: reads from sub.Ch, wraps with identifier, sends to merged.
	// Exits when sub.Ch is closed (unsubscribe) or ctx is cancelled (connection closed).
	state.forwarders.Add(1)
	go func() {
		defer state.forwarders.Done()
		droppedCount := 0
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sub.Ch:
				if !ok {
					return
				}
				envelope := hub.Event{
					Type: evt.Type,
					Payload: serverMessage{
						Identifier: id,
						Type:       evt.Type,
						Payload:    evt.Payload,
					},
				}
				select {
				case state.merged <- envelope:
					droppedCount = 0
				case <-ctx.Done():
					return
				default:
					droppedCount++
					slog.Warn("ws: dropping event, buffer full", "type", evt.Type, "user", state.userID, "dropped", droppedCount)
					// Notify client that events were dropped so it can refresh.
					// Use a non-blocking send — if merged is still full, the client
					// is too slow and will get this notification on the next drain.
					if droppedCount == 1 {
						dropNotice := hub.Event{
							Type: "events_dropped",
							Payload: serverMessage{
								Identifier: id,
								Type:       "events_dropped",
							},
						}
						select {
						case state.merged <- dropNotice:
						default:
						}
					}
				}
			}
		}
	}()

	// Send confirmation.
	h.sendConfirm(ctx, conn, id)
}

// handleUnsubscribe processes an unsubscribe command.
func (h *WSHandler) handleUnsubscribe(state *connState, id channelIdentifier) {
	topic := h.topicForIdentifier(id)
	if topic == "" {
		return
	}

	state.mu.Lock()
	if sub, exists := state.subs[topic]; exists {
		h.hub.Unsubscribe(sub)
		delete(state.subs, topic)
		delete(state.topics, topic)
	}
	state.mu.Unlock()
}

// topicForIdentifier derives the hub topic from a channel identifier.
func (h *WSHandler) topicForIdentifier(id channelIdentifier) string {
	switch id.Channel {
	case "SessionsChannel":
		if id.OrgID != "" {
			parsed, err := uuid.Parse(id.OrgID)
			if err != nil {
				return ""
			}
			return "sessions_list:" + parsed.String()
		}
	case "SessionChannel":
		if id.SessionID != "" {
			parsed, err := uuid.Parse(id.SessionID)
			if err != nil {
				return ""
			}
			return "session:" + parsed.String()
		}
	}
	return ""
}

// sendConfirm sends a subscription confirmation to the client.
func (h *WSHandler) sendConfirm(ctx context.Context, conn *websocket.Conn, id channelIdentifier) {
	msg := serverMessage{
		Identifier: id,
		Type:       "confirm_subscription",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		conn.Close(websocket.StatusInternalError, "write error")
		return
	}
}

// sendReject sends a subscription rejection to the client.
func (h *WSHandler) sendReject(ctx context.Context, conn *websocket.Conn, id channelIdentifier, reason string) {
	msg := struct {
		Identifier channelIdentifier `json:"identifier"`
		Type       string            `json:"type"`
		Reason     string            `json:"reason"`
	}{
		Identifier: id,
		Type:       "reject_subscription",
		Reason:     reason,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		conn.Close(websocket.StatusInternalError, "write error")
		return
	}
}

// parseJWT validates a JWT string and returns the user UUID from the subject claim.
// It also checks password_changed_at to invalidate JWTs issued before a password change.
func (h *WSHandler) parseJWT(ctx context.Context, tokenStr string) (uuid.UUID, error) {
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.jwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid {
		return uuid.Nil, jwt.ErrSignatureInvalid
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil || sub == "" {
		return uuid.Nil, jwt.ErrSignatureInvalid
	}
	userID, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user ID in token: %w", err)
	}

	// Check password_changed_at to invalidate JWTs issued before password change.
	pwChangedAt, err := h.queries.GetUserPasswordChangedAt(ctx, userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("password check failed: %w", err)
	}
	if pwChangedAt.Valid {
		iat, iatErr := parsed.Claims.GetIssuedAt()
		if iatErr != nil || iat == nil {
			return uuid.Nil, jwt.ErrSignatureInvalid
		}
		if pwChangedAt.Time.After(iat.Time) {
			return uuid.Nil, jwt.ErrSignatureInvalid
		}
	}

	return userID, nil
}
