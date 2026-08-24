package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homereel/backend/internal/auth"
	"homereel/backend/internal/db"
	"homereel/backend/internal/events"
	"homereel/backend/internal/fservice"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/media"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/search"
	"homereel/backend/internal/store"
	"homereel/backend/internal/streaming"
)

func newTestServer(t *testing.T, password string) (*httptest.Server, string) {
	t.Helper()
	ts, cookie, _ := newTestServerDB(t, password)
	return ts, cookie
}

// newTestServerDB builds the test server and also returns the database handle
// so tests can seed records directly.
func newTestServerDB(t *testing.T, password string) (*httptest.Server, string, *sql.DB) {
	t.Helper()
	handler, database := newTestHandler(t, password, "")
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, "", database
}

// newTestHandler wires the full dependency graph and returns the root handler,
// optionally hosting a frontend static dir (static-serving tests pass one).
func newTestHandler(t *testing.T, password, staticDir string) (http.Handler, *sql.DB) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	authSvc := auth.New(database, 30)
	if _, err := authSvc.EnsurePassword(context.Background(), password); err != nil {
		t.Fatalf("ensure password: %v", err)
	}
	jobsSvc := jobs.NewService(store.NewJobRepo(database), jobs.NewLiveStatus())
	videosRepo := store.NewVideoRepo(database)
	sourcesRepo := store.NewSourceRepo(database)
	showsRepo := store.NewShowRepo(database)
	seriesRepo := store.NewSeriesRepo(database)
	historyRepo := store.NewHistoryRepo(database)
	prefsRepo := store.NewPlaybackPrefsRepo(database)
	scannerSvc := scanner.New(
		videosRepo,
		sourcesRepo,
		showsRepo,
		seriesRepo,
		jobsSvc,
		events.New(),
		media.Paths{FFmpeg: "ffmpeg", FFprobe: "ffprobe"},
		t.TempDir(),
	)
	streamingSvc := streaming.New(videosRepo, t.TempDir(), media.Paths{FFmpeg: "ffmpeg", FFprobe: "ffprobe"})
	dataDir := t.TempDir()
	bus := events.New()
	fsvc := fservice.New(jobsSvc, store.NewSettingsRepo(database), sourcesRepo, media.Paths{FFmpeg: "ffmpeg", FFprobe: "ffprobe"})
	fsvc.SetLibraryNotifier(scannerSvc.IngestPaths, scannerSvc.EvictPaths)
	handler := New(authSvc, jobsSvc, scannerSvc, fsvc,
		videosRepo, showsRepo, seriesRepo, historyRepo, prefsRepo,
		store.NewDevLogRepo(database), streamingSvc,
		search.NewFTS5(database, videosRepo), bus, dataDir, staticDir)
	return handler, database
}

func loginCookie(t *testing.T, ts *httptest.Server, password string) string {
	t.Helper()
	resp, _ := doJSON(t, "POST", ts.URL+"/api/auth/login", `{"password":"`+password+`"}`, "")
	return sessionCookieHeader(t, resp)
}

func doJSON(t *testing.T, method, url string, body string, cookie string) (*http.Response, string) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(raw)
}

func sessionCookieHeader(t *testing.T, resp *http.Response) string {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatal("no session cookie in response")
	return ""
}

func decodeBool(t *testing.T, body string) bool {
	t.Helper()
	var v struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode status body %q: %v", body, err)
	}
	return v.Authenticated
}

func TestHealthPublic(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	resp, body := doJSON(t, "GET", ts.URL+"/api/health", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
}

func TestProtectedReturns401WithoutSession(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	resp, body := doJSON(t, "GET", ts.URL+"/api/me", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me status = %d, want 401 (body %s)", resp.StatusCode, body)
	}
}

func TestAuthFlow(t *testing.T) {
	ts, _ := newTestServer(t, "secret")

	resp, body := doJSON(t, "GET", ts.URL+"/api/auth/status", "", "")
	if resp.StatusCode != http.StatusOK || decodeBool(t, body) {
		t.Fatalf("status before login = %d %s, want 200 false", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/auth/login", `{"password":"wrong"}`, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login wrong status = %d, want 401 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/auth/login", `{"password":"secret"}`, "")
	if resp.StatusCode != http.StatusOK || !decodeBool(t, body) {
		t.Fatalf("login ok = %d %s, want 200 true", resp.StatusCode, body)
	}
	cookie := sessionCookieHeader(t, resp)

	resp, body = doJSON(t, "GET", ts.URL+"/api/me", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me with session = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/auth/status", "", cookie)
	if resp.StatusCode != http.StatusOK || !decodeBool(t, body) {
		t.Fatalf("status with session = %d %s, want 200 true", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, "POST", ts.URL+"/api/auth/logout", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}

	resp, _ = doJSON(t, "GET", ts.URL+"/api/me", "", cookie)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", resp.StatusCode)
	}
}

func TestFilesRoutesRequireAuth(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	for _, path := range []string{"/api/files/list", "/api/files/pins", "/api/files/sources"} {
		resp, body := doJSON(t, "GET", ts.URL+path, "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without session = %d, want 401 (body %s)", path, resp.StatusCode, body)
		}
	}
}
