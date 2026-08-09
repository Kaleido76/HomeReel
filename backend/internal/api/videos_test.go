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
	"homereel/backend/internal/fservice"
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

func TestConvertEndpoints(t *testing.T) {
	ts, _, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")

	root := t.TempDir()
	file := filepath.Join(root, "movie.mkv")
	dir := filepath.Join(root, "show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{file, filepath.Join(dir, "ep1.mkv")} {
		if err := os.WriteFile(f, []byte("fake-video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	body := mustJSON(t, map[string]any{
		"paths": []string{file, dir, filepath.Join(root, "readme.txt"), filepath.Join(root, "nope.mkv")},
	})
	resp, raw := doJSON(t, "POST", ts.URL+"/api/convert", body, cookie)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("convert = %d (body %s)", resp.StatusCode, raw)
	}
	var res struct {
		Jobs   []fservice.ConvertJob `json:"jobs"`
		Errors []fservice.OpError    `json:"errors"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("decode convert response: %v", err)
	}
	if len(res.Jobs) != 2 {
		t.Fatalf("convert jobs = %d, want 2: %s", len(res.Jobs), raw)
	}
	if res.Jobs[0].Kind != "file" || res.Jobs[1].Kind != "dir" {
		t.Fatalf("convert kinds = %s/%s, want file/dir", res.Jobs[0].Kind, res.Jobs[1].Kind)
	}
	if res.Jobs[0].Output != filepath.Join(root, "movie.mp4") ||
		res.Jobs[1].Output != filepath.Join(root, "show (MP4)") {
		t.Fatalf("convert outputs = %q/%q", res.Jobs[0].Output, res.Jobs[1].Output)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("convert errors = %d, want 2 (non-video + missing path): %s", len(res.Errors), raw)
	}

	// Empty selection is rejected.
	resp, _ = doJSON(t, "POST", ts.URL+"/api/convert", `{"paths":[]}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty convert = %d, want 400", resp.StatusCode)
	}

	// An operations-panel preset (h264 re-encode) is accepted and the job
	// payload carries the chosen parameters through to the worker.
	body = mustJSON(t, map[string]any{
		"paths": []string{file},
		"params": map[string]any{
			"video": "h264", "vcrf": 20, "audio": "aac", "akbps": 128, "burn": true,
		},
	})
	resp, raw = doJSON(t, "POST", ts.URL+"/api/convert", body, cookie)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("convert with params = %d (body %s)", resp.StatusCode, raw)
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("decode convert response: %v", err)
	}
	if len(res.Jobs) != 1 || res.Jobs[0].Output != filepath.Join(root, "movie.mp4") {
		t.Fatalf("convert with params jobs = %v, want 1 file job: %s", res.Jobs, raw)
	}
	// All-valid selections must still return errors as an empty array, never
	// JSON null (a nil slice would crash array-typed consumers).
	if res.Errors == nil || len(res.Errors) != 0 {
		t.Fatalf("convert errors = %#v, want empty non-nil array", res.Errors)
	}
}

// TestConvertProbeEndpoints verifies the probe endpoint returns one entry per
// expanded video (a directory expands to its direct-level videos) and that
// codec lists are always arrays.
func TestConvertProbeEndpoints(t *testing.T) {
	ts, _, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")

	root := t.TempDir()
	file := filepath.Join(root, "movie.mkv")
	dir := filepath.Join(root, "show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{file, filepath.Join(dir, "ep1.mkv")} {
		if err := os.WriteFile(f, []byte("fake-video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	body := mustJSON(t, map[string]any{"paths": []string{file, dir}})
	resp, raw := doJSON(t, "POST", ts.URL+"/api/convert/probe", body, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe = %d (body %s)", resp.StatusCode, raw)
	}
	var res struct {
		Results []fservice.ConvertProbe `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("decode probe response: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("probe results = %d, want 2: %s", len(res.Results), raw)
	}
	if res.Results[0].Kind != "file" || res.Results[1].Kind != "dir" {
		t.Fatalf("probe kinds = %s/%s", res.Results[0].Kind, res.Results[1].Kind)
	}
	for _, r := range res.Results {
		if r.AudioCodecs == nil || r.SubtitleCodecs == nil {
			t.Fatalf("probe codec lists must be arrays, got %#v", r)
		}
	}

	resp, _ = doJSON(t, "POST", ts.URL+"/api/convert/probe", `{"paths":[]}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty probe = %d, want 400", resp.StatusCode)
	}
}
