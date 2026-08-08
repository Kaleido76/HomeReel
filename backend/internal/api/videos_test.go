package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

// newTestSource marks a fresh temp directory as a multimedia source and
// returns its id.
func newTestSource(t *testing.T, ts *httptest.Server, cookie string) string {
	t.Helper()
	root := t.TempDir()
	resp, body := doJSON(t, "POST", ts.URL+"/api/files/sources",
		mustJSON(t, map[string]string{"path": root}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add source = %d (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode add source: %v", err)
	}
	if created.Source.ID == "" {
		t.Fatalf("no source id in %s", body)
	}
	return created.Source.ID
}

// seedVideo inserts a video row into the given test database.
func seedVideo(t *testing.T, database *sql.DB, sourceID, id, rel, path, title string, dur float64) {
	t.Helper()
	repo := store.NewVideoRepo(database)
	now := "2026-01-01T00:00:00.000000000Z"
	if err := repo.Create(context.Background(), domain.Video{
		ID: id, SourceID: sourceID, FileID: id, RelativePath: rel,
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
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)
	seedVideo(t, database, sourceID, "v2", "b.mp4", "b.mp4", "Beta", 200)

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

func TestVideosListAdvancedFilter(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)
	seedVideo(t, database, sourceID, "v2", "b.mp4", "b.mp4", "Beta", 200)
	ctx := context.Background()
	repo := store.NewVideoRepo(database)
	desc, genre, year := "太空冒险", "科幻", 2001
	if err := repo.UpdateMetadata(ctx, "v1", domain.VideoPatch{
		Description: &desc, Genre: &genre, Year: &year,
	}); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if err := repo.SetTags(ctx, "v1", []string{"科幻"}); err != nil {
		t.Fatalf("tags: %v", err)
	}

	resp, body := doJSON(t, "GET", ts.URL+"/api/videos?desc="+url.QueryEscape("冒险"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"Alpha"`) || strings.Contains(body, `"Beta"`) {
		t.Fatalf("desc filter = %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos?genre="+url.QueryEscape("科幻"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"Alpha"`) {
		t.Fatalf("genre filter = %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos?year=2001", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"Alpha"`) || strings.Contains(body, `"Beta"`) {
		t.Fatalf("year filter = %d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos?tag="+url.QueryEscape("科幻")+"&tag="+url.QueryEscape("太空"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"total":0`) {
		t.Fatalf("conflicting multi tag should match none = %d %s", resp.StatusCode, body)
	}
}

func TestVideoDetail(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

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
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

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
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, body := doJSON(t, "POST", ts.URL+"/api/files/sources",
		mustJSON(t, map[string]string{"path": root}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add source = %d (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode add source: %v", err)
	}
	sourceID := created.Source.ID
	seedVideo(t, database, sourceID, "v1", "clip.mp4", root+string(filepath.Separator)+"clip.mp4", "Clip", 10)

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
	sourceID := newTestSource(t, ts, cookie)

	// One segmented video and one normal video.
	seedVideo(t, database, sourceID, "v1", "shows/ep1.mp4", "shows/ep1.mp4", "Seg", 10)
	seedVideo(t, database, sourceID, "v2", "other.mp4", "other.mp4", "Plain", 10)
	repo := store.NewVideoRepo(database)
	if err := repo.UpdateProbe(context.Background(), domain.Video{
		ID: "v1", Title: "Seg", Codec: "h264", Container: "mp4", Segmented: true,
	}); err != nil {
		t.Fatalf("mark segmented: %v", err)
	}

	// Status lists only the segmented video, not remuxed (no cache in test).
	resp, body := doJSON(t, "GET", ts.URL+"/api/remux/status", "", cookie)
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
}
