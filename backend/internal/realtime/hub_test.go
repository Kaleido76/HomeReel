package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestServer hosts a hub behind a raw HTTP server so tests can dial it like
// a real client.
func newTestServer(t *testing.T, hub *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleConnection))
	t.Cleanup(srv.Close)
	return srv
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readEnvelope reads one frame, failing the test on any error or timeout.
func readEnvelope(t *testing.T, conn *websocket.Conn) Envelope {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var env Envelope
	if err := conn.ReadJSON(&env); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return env
}

func waitCount(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Count() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hub count = %d, want %d", hub.Count(), want)
}

func TestBroadcastReachesClients(t *testing.T) {
	hub := New()
	srv := newTestServer(t, hub)
	c1 := dial(t, srv)
	c2 := dial(t, srv)
	waitCount(t, hub, 2)

	hub.Broadcast("events.test", map[string]string{"k": "v"})

	env1 := readEnvelope(t, c1)
	env2 := readEnvelope(t, c2)
	for _, env := range []Envelope{env1, env2} {
		if env.Type != "events.test" {
			t.Fatalf("type = %s, want events.test", env.Type)
		}
		var data map[string]string
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if data["k"] != "v" {
			t.Fatalf("data = %v, want k=v", data)
		}
	}
}

func TestHandlerDispatch(t *testing.T) {
	hub := New()
	var calls atomic.Int32
	hub.Handle("echo", func(_ context.Context, _ json.RawMessage) (any, error) {
		calls.Add(1)
		return map[string]string{"pong": "yes"}, nil
	})
	srv := newTestServer(t, hub)
	conn := dial(t, srv)

	if err := conn.WriteJSON(Envelope{ID: "r1", Type: "echo", Data: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reply := readEnvelope(t, conn)
	if reply.ID != "r1" || reply.Type != "echo.result" {
		t.Fatalf("reply = %s %s, want r1 echo.result", reply.ID, reply.Type)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestHandlerErrorReply(t *testing.T) {
	hub := New()
	hub.Handle("boom", func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, &handlerErr{msg: "exploded"}
	})
	srv := newTestServer(t, hub)
	conn := dial(t, srv)

	if err := conn.WriteJSON(Envelope{ID: "r2", Type: "boom", Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reply := readEnvelope(t, conn)
	if reply.Type != "boom.error" {
		t.Fatalf("reply type = %s, want boom.error", reply.Type)
	}
	var body ErrorBody
	if err := json.Unmarshal(reply.Data, &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Message != "exploded" {
		t.Fatalf("error message = %s, want exploded", body.Message)
	}
}

func TestUnknownTypeErrorReply(t *testing.T) {
	hub := New()
	srv := newTestServer(t, hub)
	conn := dial(t, srv)

	if err := conn.WriteJSON(Envelope{ID: "r3", Type: "no.such", Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reply := readEnvelope(t, conn)
	if reply.Type != "no.such.error" {
		t.Fatalf("reply type = %s, want no.such.error", reply.Type)
	}
}

// TestPingPong checks explicit app-level ping frames are answered.
func TestPingPong(t *testing.T) {
	hub := New()
	srv := newTestServer(t, hub)
	conn := dial(t, srv)

	if err := conn.WriteJSON(Envelope{ID: "p1", Type: TypePing, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	reply := readEnvelope(t, conn)
	if reply.ID != "p1" || reply.Type != TypePong {
		t.Fatalf("reply = %s %s, want p1 pong", reply.ID, reply.Type)
	}
}

// TestSlowClientNotBlocked fills a client's outbound queue past capacity and
// asserts a subsequent broadcast still returns promptly (the slow client's
// frames go to a detached goroutine / the connection is dropped).
func TestSlowClientNotBlocked(t *testing.T) {
	hub := New()
	srv := newTestServer(t, hub)
	// Never read from this connection; the write pump fills its queue.
	conn := dial(t, srv)
	waitCount(t, hub, 1)

	// Overflow the queue without reading.
	for i := 0; i < sendQueue+10; i++ {
		hub.Broadcast("events.flood", nil)
	}

	done := make(chan struct{})
	go func() {
		hub.Broadcast("events.after", nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked behind a slow client")
	}
	_ = conn
}

type handlerErr struct{ msg string }

func (e *handlerErr) Error() string { return e.msg }
