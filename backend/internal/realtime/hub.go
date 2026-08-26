package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// upgrader promotes HTTP requests to WebSocket connections. Same-origin is the
// only sensible topology here (the SPA is served by this same backend), but
// CheckOrigin is kept permissive because the browser may connect from the LAN
// IP form of the same server; the session cookie (SameSite=Lax) still gates the
// handshake through requireAuth, so a cross-site request is rejected before we
// ever upgrade.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(_ *http.Request) bool { return true },
}

// Handler answers an inbound RPC request. It receives the request payload and
// returns either a value (sent as "<type>.result") or an error (sent as
// "<type>.error" with the message). Fire-and-forget notifications (no ID)
// never produce a response regardless of the return value.
type Handler func(ctx context.Context, payload json.RawMessage) (any, error)

const (
	// sendQueue is the per-connection outbound buffer. A slow client that fills
	// it is dropped so one stuck terminal never stalls the whole hub.
	sendQueue = 64
	// pongWait / pingPeriod keep dead connections reaped (RFC 6455 heartbeats).
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	// writeWait bounds a single frame write so a wedged socket cannot block a
	// goroutine forever.
	writeWait = 10 * time.Second
)

// Hub is the central registry of live connections and request handlers. It
// owns broadcast fan-out and dispatches inbound messages to registered
// handlers. Created once at startup and shared with the HTTP layer.
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	// handlers routes an inbound request type to its registered function.
	handlers map[string]Handler
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{
		clients:  map[*Client]struct{}{},
		handlers: map[string]Handler{},
	}
}

// Handle registers a request handler for the given message type. Later
// registrations replace earlier ones for the same type.
func (h *Hub) Handle(typ string, fn Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[typ] = fn
}

// Broadcast pushes a fire-and-forget notification to every connected client.
// It is safe to call from any goroutine and never blocks on a slow client.
func (h *Hub) Broadcast(typ string, payload any) {
	env := Envelope{Type: typ, Data: mustJSON(payload)}
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.send(env)
	}
}

// Count returns the number of currently connected clients (useful for tests).
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// HandleConnection upgrades an authenticated HTTP request to a WebSocket and
// runs the connection until it closes. It is registered behind requireAuth so
// the session cookie gates access (ADR-002 multi-terminal independent
// sessions map to independent connections).
func (h *Hub) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade", "err", err)
		return
	}
	c := newClient(h, conn)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		c.close()
	}()
	// readPump blocks this handler goroutine for the whole session, so the
	// request context is only canceled when the session ends (handler returns) —
	// writePump observes that and exits; it never outlives the handler.
	go c.writePump(r.Context())
	c.readPump(r.Context())
}

// register looks up a handler for a type.
func (h *Hub) handler(typ string) (Handler, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn, ok := h.handlers[typ]
	return fn, ok
}

// dispatch routes an inbound request to its handler and answers it.
func (h *Hub) dispatch(r request) {
	fn, ok := h.handler(r.env.Type)
	if !ok {
		if r.env.ID != "" {
			_ = r.replyError("unknown_type", "未注册的消息类型: "+r.env.Type)
		}
		return
	}
	res, err := fn(context.Background(), r.env.Data)
	if err != nil {
		_ = r.replyError("handler_error", err.Error())
		return
	}
	_ = r.reply(res)
}
