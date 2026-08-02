package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStaticServing covers the single-service layout: the backend hosts the
// built frontend, deep links fall back to index.html, and unknown /api paths
// stay JSON instead of returning the SPA.
func TestStaticServing(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(staticDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<html>HomeReel</html>")
	write(filepath.Join("assets", "app.js"), "console.log(1)")

	handler, _ := newTestHandler(t, "secret", staticDir)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	cases := []struct {
		name string
		url  string
		want int
		body string
	}{
		{"root", "/", 200, "<html>HomeReel</html>"},
		{"deep link", "/library/video/123", 200, "<html>HomeReel</html>"},
		{"existing asset", "/assets/app.js", 200, "console.log(1)"},
		{"missing asset", "/assets/missing.js", 404, ""},
		{"unknown api", "/api/nope", 404, "not_found"},
		{"api root stays json", "/api", 404, "not_found"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.url)
		if err != nil {
			t.Fatalf("%s: get: %v", c.name, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, resp.StatusCode, c.want)
		}
		if c.body != "" && !strings.Contains(string(body), c.body) {
			t.Errorf("%s: body = %q, want substring %q", c.name, body, c.body)
		}
	}
}

// TestNoStaticWhenDisabled ensures API-only mode never serves the SPA.
func TestNoStaticWhenDisabled(t *testing.T) {
	handler, _ := newTestHandler(t, "secret", "")
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET / without static dir = %d, want 404", resp.StatusCode)
	}
}
