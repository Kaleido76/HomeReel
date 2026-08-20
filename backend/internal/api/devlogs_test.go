package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestDevLogsFlow covers the dev-tools log archive lifecycle: submit → list →
// fetch (with raw text) → delete.
func TestDevLogsFlow(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")

	// Submit an archive.
	body := `{"source":"Android（移动端）","note":"手机播放卡顿",
		"entries":[
			{"timestamp":"2026-09-01T00:00:00.000Z","level":"warn","module":"player","message":"buffering slow"},
			{"timestamp":"2026-09-01T00:00:01.000Z","level":"error","module":"hls","message":"segment fetch failed"}
		]}`
	resp, respBody := doJSON(t, "POST", ts.URL+"/api/devlogs", body, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create devlog = %d (body %s)", resp.StatusCode, respBody)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(respBody), &created); err != nil || created.ID == "" {
		t.Fatalf("create devlog response %q: %v", respBody, err)
	}

	// List shows the summary with a count but no entries.
	resp, respBody = doJSON(t, "GET", ts.URL+"/api/devlogs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(respBody, `"count":2`) {
		t.Fatalf("list devlogs = %d (body %s)", resp.StatusCode, respBody)
	}
	if strings.Contains(respBody, `"message"`) {
		t.Fatalf("list devlogs leaked entries: %s", respBody)
	}

	// Fetch returns the entries verbatim.
	resp, respBody = doJSON(t, "GET", ts.URL+"/api/devlogs/"+created.ID, "", cookie)
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(respBody, `"buffering slow"`) ||
		!strings.Contains(respBody, `"segment fetch failed"`) {
		t.Fatalf("get devlog = %d (body %s)", resp.StatusCode, respBody)
	}

	// Raw returns plain text, one line per entry.
	resp, respBody = doJSON(t, "GET", ts.URL+"/api/devlogs/"+created.ID+"/raw", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("raw devlog = %d (body %s)", resp.StatusCode, respBody)
	}
	if !strings.Contains(respBody, "[warn] [player] buffering slow") ||
		!strings.Contains(respBody, "[error] [hls] segment fetch failed") {
		t.Fatalf("raw devlog missing lines: %q", respBody)
	}

	// Delete removes it; fetching again 404s.
	resp, _ = doJSON(t, "DELETE", ts.URL+"/api/devlogs/"+created.ID, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete devlog = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "GET", ts.URL+"/api/devlogs/"+created.ID, "", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted devlog = %d, want 404", resp.StatusCode)
	}
}

// TestDevLogsRequireAuth ensures the archive endpoints are protected.
func TestDevLogsRequireAuth(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	for _, path := range []string{"/api/devlogs", "/api/devlogs/x", "/api/devlogs/x/raw"} {
		resp, _ := doJSON(t, "GET", ts.URL+path, "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without session = %d, want 401", path, resp.StatusCode)
		}
	}
}
