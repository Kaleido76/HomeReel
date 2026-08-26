package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestResourcesMarkEnqueuesJob verifies marking enqueues a user-facing
// mark_resource job and returns its id.
func TestResourcesMarkEnqueuesJob(t *testing.T) {
	ts, cookie, _ := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "POST", ts.URL+"/api/files/resources",
		mustJSON(t, map[string]any{"paths": []string{`D:\Clips`}, "kind": "series"}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mark status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		JobIDs []string `json:"job_ids"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode mark body: %v", err)
	}
	if len(out.JobIDs) != 1 {
		t.Fatalf("job_ids = %+v, want 1", out.JobIDs)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/jobs", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "mark_resource") {
		t.Fatalf("jobs body missing mark_resource job: %s", body)
	}
}

// Discrete marking no longer exists (管理面定稿 2026-08) — rejected.
func TestResourcesMarkRejectsDiscrete(t *testing.T) {
	ts, cookie, _ := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "POST", ts.URL+"/api/files/resources",
		mustJSON(t, map[string]any{"paths": []string{`D:\Clips`}, "kind": "discrete"}), cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("mark discrete = %d, want 400 (body %s)", resp.StatusCode, body)
	}
}

// TestResourcesMarkRequiresAuth verifies the resource route is protected.
func TestResourcesMarkRequiresAuth(t *testing.T) {
	ts, _, _ := newTestServer(t, "secret")
	resp, body := doJSON(t, "POST", ts.URL+"/api/files/resources",
		`{"paths":["D:\\Clips"],"kind":"series"}`, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mark without session = %d, want 401 (body %s)", resp.StatusCode, body)
	}
}
