package logging

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"garbage": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAccessLevels(t *testing.T) {
	cases := []struct {
		path   string
		status int
		want   slog.Level
	}{
		{"/api/videos", http.StatusOK, slog.LevelInfo},
		{"/api/videos", http.StatusNotFound, slog.LevelWarn},
		{"/api/videos", http.StatusInternalServerError, slog.LevelError},
		{"/api/ws", http.StatusOK, slog.LevelInfo},
		{"/api/stream/abc", http.StatusOK, slog.LevelDebug},
		{"/api/stream/abc/hls/seg-1.ts", http.StatusOK, slog.LevelDebug},
		{"/api/stream/abc", http.StatusBadRequest, slog.LevelWarn},
		{"/assets/app.js", http.StatusOK, slog.LevelDebug},
		{"/", http.StatusOK, slog.LevelDebug},
	}
	for _, c := range cases {
		if got := accessLevel(c.status, accessLogQuiet(c.path)); got != c.want {
			t.Errorf("accessLevel(%s, %d) = %v, want %v", c.path, c.status, got, c.want)
		}
	}
}

// withTestLogger swaps in a buffer logger at the given level for the duration
// of fn, restoring the previous default afterwards.
func withTestLogger(t *testing.T, level slog.Level, fn func(buf *bytes.Buffer)) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	defer slog.SetDefault(prev)
	fn(&buf)
}

func TestAccessLogInfoSuppressesMedia(t *testing.T) {
	withTestLogger(t, slog.LevelInfo, func(buf *bytes.Buffer) {
		h := AccessLog()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Business API success is logged at info.
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://x/api/videos", nil))
		if !strings.Contains(buf.String(), "api/videos") {
			t.Fatalf("loud 200 not logged:\n%s", buf.String())
		}

		// Media/static success stays silent at info.
		buf.Reset()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://x/api/stream/abc/hls/seg-1.ts", nil))
		if buf.Len() != 0 {
			t.Fatalf("quiet 200 should be silent at info:\n%s", buf.String())
		}
	})
}

func TestAccessLogMediaErrorsStillLogged(t *testing.T) {
	withTestLogger(t, slog.LevelInfo, func(buf *bytes.Buffer) {
		h := AccessLog()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://x/api/stream/abc", nil))
		if !strings.Contains(buf.String(), "ERROR") {
			t.Fatalf("media 500 should be logged at error:\n%s", buf.String())
		}
	})
}

func TestAccessLogDebugShowsMedia(t *testing.T) {
	withTestLogger(t, slog.LevelDebug, func(buf *bytes.Buffer) {
		h := AccessLog()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://x/api/stream/abc", nil))
		if !strings.Contains(buf.String(), "api/stream/abc") {
			t.Fatalf("media 200 should appear at debug:\n%s", buf.String())
		}
	})
}

func TestSetupRotatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "homereel.log")
	if err := os.WriteFile(path, []byte("old content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, closeLog, err := Setup(Config{Level: "info", Format: "text", File: path})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Info("hello")
	closeLog()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var rotated, fresh bool
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "homereel-") && strings.HasSuffix(e.Name(), ".log"):
			rotated = true
		case e.Name() == "homereel.log":
			fresh = true
		}
	}
	if !rotated || !fresh {
		t.Fatalf("expected one rotated + one fresh log, got %v", entries)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old content") {
		t.Error("fresh file must not contain rotated content")
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("fresh file missing the new record:\n%s", data)
	}
}

func TestSetupFormatJSON(t *testing.T) {
	// Setup with json must not error and must install a default that writes
	// JSON-shaped lines to a file.
	dir := t.TempDir()
	path := filepath.Join(dir, "h.log")
	_, closeLog, err := Setup(Config{Level: "info", Format: "json", File: path})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Info("json record")
	closeLog()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		t.Fatalf("expected JSON log line, got:\n%s", data)
	}
}