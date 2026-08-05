package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

// seedVideo inserts a video row into the given test database.
func seedVideo(t *testing.T, database *sql.DB, storageID, id, rel, path, title string, dur float64) {
	t.Helper()
	repo := store.NewVideoRepo(database)
	now := "2026-01-01T00:00:00.000000000Z"
	if err := repo.Create(context.Background(), domain.Video{
		ID: id, StorageID: storageID, FileID: id, RelativePath: rel,
		Path: path, Size: 1, MTime: 1, Title: title, Duration: dur,
		Codec: "h264", Container: "mp4",
		CreatedAt: now, UpdatedAt: now, LastScannedAt: now,
	}); err != nil {
		t.Fatalf("seed video: %v", err)
	}
}

func TestVideosList(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, _ := newTestStorage(t, ts, cookie)
	seedVideo(t, database, storageID, "v1", "a.mp4", "a.mp4", "Alpha", 100)
	seedVideo(t, database, storageID, "v2", "b.mp4", "b.mp4", "Beta", 200)

	resp, body := doJSON(t, "GET", ts.URL+"/api/videos?sort=title&order=asc", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"Alpha"`) || !strings.Contains(body, `"total":2`) {
		t.Fatalf("list body unexpected: %s", body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/videos?q=beta", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"Beta"`) || strings.Contains(body, `"Alpha"`) {
		t.Fatalf("search body unexpected: %d %s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, "GET", ts.URL+"/api/videos", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("videos without session = %d, want 401", resp.StatusCode)
	}
}

func TestVideoDetail(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, _ := newTestStorage(t, ts, cookie)
	seedVideo(t, database, storageID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

	resp, body := doJSON(t, "GET", ts.URL+"/api/videos/v1", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"Alpha"`) {
		t.Fatalf("detail = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/nope", "", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing video = %d (body %s)", resp.StatusCode, body)
	}
}

func TestHistoryUpsertAndRead(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	storageID, _ := newTestStorage(t, ts, cookie)
	seedVideo(t, database, storageID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

	resp, body := doJSON(t, "GET", ts.URL+"/api/videos/v1/history", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"history":null`) {
		t.Fatalf("history absent = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/v1/history", `{"progress":123.5}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put history = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/v1/history", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"progress":123.5`) {
		t.Fatalf("history read = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/v1/history", `{"progress":200}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put history2 = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/v1/history", "", cookie)
	if !strings.Contains(body, `"progress":200`) {
		t.Fatalf("upsert overwrite failed: %s", body)
	}

	resp, _ = doJSON(t, "PUT", ts.URL+"/api/videos/v1/history", `{"progress":-1}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative progress = %d, want 400", resp.StatusCode)
	}
}

func TestStreamDirectAndCover(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	root := t.TempDir()
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
	storageID := created.Storage.ID

	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedVideo(t, database, storageID, "v1", "clip.mp4", root+string(filepath.Separator)+"clip.mp4", "Clip", 10)

	req, _ := http.NewRequest("GET", ts.URL+"/api/stream/v1", nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Range", "bytes=2-5")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	raw := make([]byte, 4)
	_, _ = res.Body.Read(raw)
	res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || string(raw) != "2345" {
		t.Fatalf("stream = %d %q", res.StatusCode, raw)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/stream/v1/cover", "", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing cover = %d (body %s)", resp.StatusCode, body)
	}
}

func TestRemuxEndpoints(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	root := t.TempDir()
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
	storageID := created.Storage.ID

	// One segmented video under a subfolder and one normal video outside it.
	seedVideo(t, database, storageID, "v1", "shows/ep1.mp4", "shows/ep1.mp4", "Seg", 10)
	seedVideo(t, database, storageID, "v2", "other.mp4", "other.mp4", "Plain", 10)
	repo := store.NewVideoRepo(database)
	if err := repo.UpdateProbe(context.Background(), domain.Video{
		ID: "v1", Title: "Seg", Codec: "h264", Container: "mp4", Segmented: true,
	}); err != nil {
		t.Fatalf("mark segmented: %v", err)
	}

	// Status lists only the segmented video, not remuxed (no cache in test).
	resp, body = doJSON(t, "GET", ts.URL+"/api/remux/status", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remux status = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"v1"`) || strings.Contains(body, `"v2"`) {
		t.Fatalf("remux status body unexpected: %s", body)
	}
	if !strings.Contains(body, `"remuxed":false`) {
		t.Fatalf("remux status should report not remuxed: %s", body)
	}

	// Single-video remux accepts.
	resp, body = doJSON(t, "POST", ts.URL+"/api/videos/v1/remux", "", cookie)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("single remux = %d (body %s)", resp.StatusCode, body)
	}

	// Folder remux targets only the segmented video under that folder.
	resp, body = doJSON(t, "POST", ts.URL+"/api/fs/remux", `{"storageId":"`+storageID+`","path":"shows"}`, cookie)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("folder remux = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"accepted":1`) {
		t.Fatalf("folder remux accepted count unexpected: %s", body)
	}
}
