package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWSRequiresAuth asserts the WebSocket handshake is gated by the session
// cookie like every other /api route.
func TestWSRequiresAuth(t *testing.T) {
	ts, _, _ := newTestServer(t, "secret")

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err == nil {
		conn.Close()
		t.Fatal("ws handshake without session succeeded, want 401")
	}
}

// TestWSHandshakeAndRequest dials the endpoint with a valid session and verifies
// a registered RPC round-trip through the full HTTP stack.
func TestWSHandshakeAndRequest(t *testing.T) {
	ts, _, hub := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	hub.Handle("echo", func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]string{"pong": "yes"}, nil
	})

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/ws"
	header := http.Header{}
	header.Set("Cookie", cookie)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial with session: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"id":   "req-1",
		"type": "echo",
		"data": map[string]any{"hello": "world"},
	}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var reply struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if reply.ID != "req-1" || reply.Type != "echo.result" {
		t.Fatalf("reply = %s %s, want req-1 echo.result", reply.ID, reply.Type)
	}
}

// TestWSUnknownType ensures an unregistered RPC gets an error envelope rather
// than silence.
func TestWSUnknownType(t *testing.T) {
	ts, _, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/ws"
	header := http.Header{}
	header.Set("Cookie", cookie)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial with session: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"id": "req-2", "type": "nope.nothing", "data": nil}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var reply struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if reply.Type != "nope.nothing.error" {
		t.Fatalf("reply type = %s, want nope.nothing.error", reply.Type)
	}
}
