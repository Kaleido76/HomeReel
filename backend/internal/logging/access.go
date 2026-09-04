package logging

import (
	"bufio"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// AccessLog returns middleware that logs one line per completed HTTP request.
// The level follows outcome and path class so the default info log stays
// clean (详略得当):
//
//	5xx               → Error  (always)
//	4xx               → Warn   (always)
//	2xx/3xx, business → Info   (/api/* except streaming)
//	2xx/3xx, media    → Debug  (/api/stream/* and SPA static assets)
//
// Playback traffic (Direct Range, HLS segments, covers, static assets) is
// high-frequency: logged at info it would drown the log, so its successes only
// appear at debug while any 4xx/5xx still surfaces.
func AccessLog() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			lvl := accessLevel(rec.status, accessLogQuiet(r.URL.Path))
			if !slog.Default().Enabled(r.Context(), lvl) {
				return
			}
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"remote", clientHost(r.RemoteAddr),
				"status", rec.status,
				"bytes", rec.bytes,
				"duration", time.Since(start).Round(time.Millisecond).String(),
			}
			if rng := r.Header.Get("Range"); rng != "" {
				attrs = append(attrs, "range", rng)
			}
			slog.Default().Log(r.Context(), lvl, "http", attrs...)
		})
	}
}

// accessLogQuiet reports whether path belongs to a high-frequency media or
// static path whose successful responses are logged at Debug instead of Info.
func accessLogQuiet(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return true // SPA static assets
	}
	return strings.HasPrefix(path, "/api/stream/")
}

// accessLevel picks the log level for one completed request.
func accessLevel(status int, quiet bool) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	case quiet:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// clientHost strips the port from a RemoteAddr so every access line carries
// the calling device's IP (multi-terminal debugging, ADR-002).
func clientHost(remote string) string {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}

// statusRecorder captures the response status and byte count for the access
// log while forwarding every optional interface (streaming flush, WebSocket
// hijack) to the underlying writer.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying response writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Flush/Hijack forward to the underlying writer so the access log does not
// break streaming responses or the WebSocket upgrade.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("response writer does not support hijacking")
}