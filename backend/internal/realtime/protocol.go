package realtime

import "encoding/json"

// Message types sent over the WebSocket channel (ADR-021). Type names use a
// "domain.action" namespace shared with the frontend so both sides can route
// and code against the same registry.
const (
	TypePing  = "ping"
	TypePong  = "pong"
	TypeHello = "hello"
)

// Envelope is the single wire format for every WebSocket frame (ADR-021). A
// non-empty ID marks a request/response (RPC): the response echoes the ID with
// type "<requestType>.result" (success) or "<requestType>.error" (failure). A
// frame without an ID is a fire-and-forget notification and is never answered.
type Envelope struct {
	ID   string          `json:"id,omitempty"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ErrorBody mirrors the REST error contract so clients can handle both
// transports uniformly.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// request carries a decoded inbound frame plus the connection it arrived on so
// a handler can both answer and (if it wants) push side effects.
type request struct {
	env Envelope
	c   *Client
}

// reply sends a result for an RPC request. It no-ops when the request had no
// ID (fire-and-forget).
func (r request) reply(v any) error {
	if r.env.ID == "" {
		return nil
	}
	r.c.send(Envelope{ID: r.env.ID, Type: r.env.Type + ".result", Data: mustJSON(v)})
	return nil
}

// replyError sends an error result for an RPC request.
func (r request) replyError(code, message string) error {
	if r.env.ID == "" {
		return nil
	}
	r.c.send(Envelope{ID: r.env.ID, Type: r.env.Type + ".error", Data: mustJSON(ErrorBody{Code: code, Message: message})})
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
