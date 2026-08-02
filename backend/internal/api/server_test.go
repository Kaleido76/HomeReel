package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"homereel/backend/internal/auth"
	"homereel/backend/internal/db"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/scrape"
	"homereel/backend/internal/search"
	"homereel/backend/internal/storage"
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
	storageSvc := storage.New(store.NewStorageRepo(database))
	filesSvc := files.NewService(t.TempDir())
	jobsSvc := jobs.NewService(store.NewJobRepo(database))
	videosRepo := store.NewVideoRepo(database)
	showsRepo := store.NewShowRepo(database)
	seriesRepo := store.NewSeriesRepo(database)
	historyRepo := store.NewHistoryRepo(database)
	scannerSvc := scanner.New(
		videosRepo,
		store.NewStorageRepo(database),
		showsRepo,
		seriesRepo,
		jobsSvc,
		filesSvc,
		events.New(),
		"ffprobe",
		"ffmpeg",
		t.TempDir(),
	)
	streamingSvc := streaming.New(videosRepo, t.TempDir(), "ffmpeg", "auto", "fast")
	dataDir := t.TempDir()
	scrapeSvc := scrape.New(videosRepo, showsRepo, dataDir, scrape.TMDBConfig{})
	bus := events.New()
	handler := New(authSvc, storageSvc, filesSvc, jobsSvc, scannerSvc,
		videosRepo, showsRepo, seriesRepo, historyRepo, streamingSvc,
		scrapeSvc, search.NewFTS5(database, videosRepo), bus, dataDir, staticDir)
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

func TestStoragesRequireAuth(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	for _, path := range []string{"/api/storages", "/api/fs/list"} {
		resp, body := doJSON(t, "GET", ts.URL+path, "", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without session = %d, want 401 (body %s)", path, resp.StatusCode, body)
		}
	}
}

func TestEmptyStoragesListIsArray(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	resp, body := doJSON(t, "GET", ts.URL+"/api/storages", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"storages":[]`) {
		t.Fatalf("empty storages should be [] not null: %s", body)
	}
}

func TestStorageLifecycle(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	root := t.TempDir()

	resp, body := doJSON(t, "POST", ts.URL+"/api/storages",
		`{"name":"电影","type":"internal","root_path":`+strconv.Quote(root)+`}`, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Storage struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			RootPath  string `json:"root_path"`
			Available bool   `json:"available"`
		} `json:"storage"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode create body: %v", err)
	}
	if created.Storage.ID == "" || created.Storage.Name != "电影" {
		t.Fatalf("unexpected created storage: %+v", created.Storage)
	}
	if !created.Storage.Available {
		t.Fatalf("created storage should be available, got %+v", created.Storage)
	}
	id := created.Storage.ID

	resp, body = doJSON(t, "GET", ts.URL+"/api/storages", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"id":`+strconv.Quote(id)) {
		t.Fatalf("list missing created storage: %s", body)
	}

	resp, body = doJSON(t, "PATCH", ts.URL+"/api/storages/"+id,
		`{"name":"电影库","readonly":true}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"name":"电影库"`) || !strings.Contains(body, `"readonly":true`) {
		t.Fatalf("patch result unexpected: %s", body)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/storages/"+id+"/refresh", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "DELETE", ts.URL+"/api/storages/"+id, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/storages", "", cookie)
	if resp.StatusCode != http.StatusOK || strings.Contains(body, `"id":`+strconv.Quote(id)) {
		t.Fatalf("deleted storage still listed: %d %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "PATCH", ts.URL+"/api/storages/"+id, `{"name":"x"}`, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("patch missing = %d, want 404 (body %s)", resp.StatusCode, body)
	}
}

func TestStorageValidation(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")

	cases := []string{
		`{"name":"","type":"internal","root_path":"C:\\x"}`,
		`{"name":"x","type":"internal","root_path":""}`,
		`{"name":"x","type":"blob","root_path":"C:\\x"}`,
		`{"name":"x","type":"external","root_path":"C:\\x","device_id":""}`,
	}
	for _, body := range cases {
		resp, _ := doJSON(t, "POST", ts.URL+"/api/storages", body, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("create %s = %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestFsList(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "c.mkv"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/storages",
		`{"name":"root","type":"internal","root_path":`+strconv.Quote(root)+`}`, cookie)
	var created struct {
		Storage struct {
			ID string `json:"id"`
		} `json:"storage"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/fs/list?storageId="+created.Storage.ID, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list root = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var listed struct {
		Entries []struct {
			Name    string `json:"name"`
			IsDir   bool   `json:"is_dir"`
			IsVideo bool   `json:"is_video"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Entries) != 3 {
		t.Fatalf("entries = %d, want 3: %s", len(listed.Entries), body)
	}
	if listed.Entries[0].Name != "sub" || !listed.Entries[0].IsDir {
		t.Fatalf("dirs should sort first: %+v", listed.Entries)
	}
	foundVideo, foundText := false, false
	for _, e := range listed.Entries {
		if e.Name == "a.mp4" {
			foundVideo = e.IsVideo
		}
		if e.Name == "b.txt" {
			foundText = !e.IsVideo
		}
	}
	if !foundVideo || !foundText {
		t.Fatalf("is_video detection wrong: %s", body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/fs/list?storageId="+created.Storage.ID+"&path=sub", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"c.mkv"`) {
		t.Fatalf("list subdir = %d (body %s)", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, "GET", ts.URL+"/api/fs/list?storageId="+created.Storage.ID+"&path=..", "", cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("path traversal = %d, want 400", resp.StatusCode)
	}
}

func TestFsListUnavailableStorage(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	resp, body := doJSON(t, "POST", ts.URL+"/api/storages",
		`{"name":"离线","type":"network","root_path":"\\\\unreachable-host\\share"}`, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Storage struct {
			ID string `json:"id"`
		} `json:"storage"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/fs/list?storageId="+created.Storage.ID, "", cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("list offline = %d, want 409 (body %s)", resp.StatusCode, body)
	}
}

func newTestStorage(t *testing.T, ts *httptest.Server, cookie string) (string, string) {
	t.Helper()
	root := t.TempDir()
	resp, body := doJSON(t, "POST", ts.URL+"/api/storages",
		`{"name":"root","type":"internal","root_path":`+strconv.Quote(root)+`}`, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Storage struct {
			ID string `json:"id"`
		} `json:"storage"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	return created.Storage.ID, root
}

func TestFsWriteOps(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, root := newTestStorage(t, ts, cookie)
	base := ts.URL + "/api/fs"

	mustWriteFile(t, filepath.Join(root, "a.txt"), "hello")

	resp, body := doJSON(t, "POST", base+"/mkdir",
		`{"storageId":"`+storageID+`","path":"","name":"sub"}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mkdir = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "POST", base+"/rename",
		`{"storageId":"`+storageID+`","path":"a.txt","newName":"b.txt"}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	resp, body = doJSON(t, "POST", base+"/move",
		`{"storageId":"`+storageID+`","paths":["b.txt"],"dest":"sub"}`, cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"done":1`) {
		t.Fatalf("move = %d (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "b.txt")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}

	resp, body = doJSON(t, "POST", base+"/delete",
		`{"storageId":"`+storageID+`","paths":["sub"]}`, cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"done":1`) {
		t.Fatalf("delete = %d (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "sub")); !os.IsNotExist(err) {
		t.Fatalf("deleted dir should be gone: %v", err)
	}
}

func TestFsWriteOpsReadonlyRejected(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, _ := newTestStorage(t, ts, cookie)

	if resp, _ := doJSON(t, "PATCH", ts.URL+"/api/storages/"+storageID,
		`{"readonly":true}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("patch readonly = %d", resp.StatusCode)
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/fs/mkdir",
		`{"storageId":"`+storageID+`","path":"","name":"x"}`, cookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mkdir on readonly = %d, want 403 (body %s)", resp.StatusCode, body)
	}
}

func TestFsDownloadWithRange(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, root := newTestStorage(t, ts, cookie)
	mustWriteFile(t, filepath.Join(root, "clip.mp4"), "0123456789")

	url := ts.URL + "/api/fs/download?storageId=" + storageID + "&path=clip.mp4"

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range request: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", resp.StatusCode)
	}
	if string(raw) != "2345" {
		t.Fatalf("range body = %q, want 2345", raw)
	}

	req2, _ := http.NewRequest("GET", url, nil)
	req2.Header.Set("Cookie", cookie)
	resp2, _ := http.DefaultClient.Do(req2)
	full, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || string(full) != "0123456789" {
		t.Fatalf("full download = %d %q", resp2.StatusCode, full)
	}
}

func TestUploadMultiChunk(t *testing.T) {
	ts, _ := newTestServer(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, root := newTestStorage(t, ts, cookie)

	data := "hello-homereel"
	const total = 3
	for i := 0; i < total; i++ {
		start := i * 5
		end := start + 5
		if end > len(data) {
			end = len(data)
		}
		body := multipartBody(t, data[start:end], i)
		req, _ := http.NewRequest("POST",
			ts.URL+"/api/upload?storageId="+storageID+"&path=", strings.NewReader(body))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=homereel")
		req.Header.Set("Cookie", cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("upload chunk %d: %v", i, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("upload chunk %d = %d (%s)", i, resp.StatusCode, raw)
		}
	}

	got, err := os.ReadFile(filepath.Join(root, "movie.mp4"))
	if err != nil {
		t.Fatalf("assembled file missing: %v", err)
	}
	if string(got) != data {
		t.Fatalf("assembled = %q, want %q", got, data)
	}
}

// multipartBody builds a multipart form body using the fixed boundary
// "homereel" with fields uploadId/filename/chunkIndex/chunkTotal plus file.
func multipartBody(t *testing.T, chunk string, chunkIndex int) string {
	t.Helper()
	var b strings.Builder
	writePart := func(name, value string) {
		b.WriteString("--homereel\r\n")
		b.WriteString("Content-Disposition: form-data; name=\"" + name + "\"\r\n\r\n")
		b.WriteString(value)
		b.WriteString("\r\n")
	}
	writePart("uploadId", "up-test-1")
	writePart("filename", "movie.mp4")
	writePart("chunkIndex", strconv.Itoa(chunkIndex))
	writePart("chunkTotal", "3")
	b.WriteString("--homereel\r\n")
	b.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"f\"\r\n")
	b.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	b.WriteString(chunk)
	b.WriteString("\r\n--homereel--\r\n")
	return b.String()
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
