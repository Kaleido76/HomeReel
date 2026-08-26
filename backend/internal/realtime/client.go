package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client wraps one WebSocket connection. Reads (dispatch) and writes
// (outbound queue drained by writePump) run in separate goroutines so a slow
// receiver never blocks inbound message handling or hub broadcasts.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	// out is the outbound queue; the writePump drains it. Filling it is the
	// slow-client signal: Broadcast and reply fall back to a detached goroutine
	// send and the connection is dropped if it stays behind.
	out  chan Envelope
	once sync.Once
}

func newClient(h *Hub, conn *websocket.Conn) *Client {
	return &Client{hub: h, conn: conn, out: make(chan Envelope, sendQueue)}
}

// send queues an outbound frame without blocking the caller. A full queue means
// the client is too slow to keep up; the frame is sent from a detached
// goroutine (dropped if that also blocks), so a stuck terminal cannot stall the
// hub or other clients.
func (c *Client) send(env Envelope) {
	select {
	case c.out <- env:
	default:
		go func() {
			select {
			case c.out <- env:
			default:
				c.close()
			}
		}()
	}
}

// close tears down the connection once.
func (c *Client) close() {
	c.once.Do(func() { _ = c.conn.Close() })
}

// readPump reads inbound frames, dispatches requests, and enforces a read
// deadline refreshed by pong frames. It runs until the peer closes or the
// context is cancelled.
func (c *Client) readPump(ctx context.Context) {
	defer c.close()
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		switch env.Type {
		case TypePing:
			// Answer protocol pings immediately; the browser pong is handled by
			// the library's built-in handler, so only respond to explicit pings.
			c.send(Envelope{ID: env.ID, Type: TypePong, Data: env.Data})
			continue
		case TypeHello:
			c.send(Envelope{ID: env.ID, Type: TypeHello + ".result", Data: json.RawMessage(`{}`)})
			continue
		}
		c.hub.dispatch(request{env: env, c: c})
	}
}

// writePump serializes writes and sends periodic heartbeat pings. It exits when
// the send queue closes (connection torn down) or the context is cancelled.
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case env, ok := <-c.out:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteJSON(env); err != nil {
				return
			}
		}
	}
}
