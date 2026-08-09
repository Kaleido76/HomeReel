package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(b)
}

// TestDisksList verifies the drive enumeration endpoint returns a well-formed
// list (on Windows this reflects the actual host; on other platforms it is the
// root entry).
func TestDisksList(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")
	resp, body := doJSON(t, "GET", ts.URL+"/api/disks", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disks status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Disks []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"disks"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode disks body: %v", err)
	}
	if len(out.Disks) == 0 {
		t.Fatal("expected at least one disk entry")
	}
	if out.Disks[0].Path == "" || out.Disks[0].Type == "" {
		t.Fatalf("disk entry missing fields: %+v", out.Disks[0])
	}
}

// TestFilesList verifies absolute-path directory listing with size/mtime.
func TestFilesList(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := ts.URL + "/api/files/list?path=" + url.QueryEscape(root)
	resp, body := doJSON(t, "GET", u, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("files list status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Entries []struct {
			Name    string `json:"name"`
			Path    string `json:"path"`
			IsDir   bool   `json:"is_dir"`
			Size    int64  `json:"size"`
			IsVideo bool   `json:"is_video"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode files list: %v", err)
	}
	foundFile, foundDir := false, false
	for _, e := range out.Entries {
		if e.Name == "a.txt" && !e.IsDir && e.Size == 5 {
			foundFile = true
		}
		if e.Name == "sub" && e.IsDir {
			foundDir = true
		}
	}
	if !foundFile || !foundDir {
		t.Fatalf("list missing expected entries: %+v", out.Entries)
	}
}

// TestFilesRenameDelete verifies synchronous rename and permanent delete.
// TestRemoveSourceWipesLibrary verifies that removing a media source marker
// deletes every video it owned from the library — the whole subtree's single
// episodes and series disappear with it, while other sources keep their rows.
func TestRemoveSourceWipesLibrary(t *testing.T) {
	ts, cookie, database := newTestServerDB(t, "secret")
	cookie = loginCookie(t, ts, "secret")
	ctx := context.Background()
	root := t.TempDir()
	otherRoot := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	sources := store.NewSourceRepo(database)
	if err := sources.Create(ctx, domain.MediaSource{ID: "s1", Path: root, CreatedAt: ts2026}); err != nil {
		t.Fatal(err)
	}
	if err := sources.Create(ctx, domain.MediaSource{ID: "s2", Path: otherRoot, CreatedAt: ts2026}); err != nil {
		t.Fatal(err)
	}
	videos := store.NewVideoRepo(database)
	mk := func(id, srcID, rel string) {
		if err := videos.Create(ctx, domain.Video{
			ID: id, SourceID: srcID, FileID: "f" + id, RelativePath: rel,
			Path: filepath.Join(srcID, rel), Size: 1, MTime: 1, Title: titleOf(rel),
			CreatedAt: ts2026, UpdatedAt: ts2026, LastScannedAt: ts2026,
		}); err != nil {
			t.Fatalf("seed video %s: %v", id, err)
		}
	}
	mk("e1", "s1", "Show/Show.S01E01.mkv")
	mk("e2", "s1", "Show/Show.S01E02.mkv")
	mk("m1", "s2", "Other.mkv")

	series := store.NewSeriesRepo(database)
	s, err := series.CreateAtRoot(ctx, "Show", filepath.Join(root, "Show"))
	if err != nil {
		t.Fatal(err)
	}
	if err := series.BindMembers(ctx, s.ID, []domain.EpisodeAssign{
		{VideoID: "e1", EpisodeNumber: 1, Title: "Show.S01E01"},
		{VideoID: "e2", EpisodeNumber: 2, Title: "Show.S01E02"},
	}); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "DELETE", ts.URL+"/api/files/sources?path="+url.QueryEscape(root), "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove source = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Removed       bool `json:"removed"`
		VideosRemoved int  `json:"videos_removed"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Removed || out.VideosRemoved != 2 {
		t.Fatalf("remove source response = %+v, want removed + 2 videos", out)
	}

	all, err := videos.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "m1" {
		t.Fatalf("remaining videos = %+v, want only m1 (other source kept)", all)
	}
	// The emptied series disappears with its videos.
	slist, err := series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(slist) != 0 {
		t.Fatalf("series after source removal = %+v, want none", slist)
	}
}

func TestFilesRenameDelete(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	root := t.TempDir()
	orig := filepath.Join(root, "old.txt")
	if err := os.WriteFile(orig, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/files/rename",
		mustJSON(t, map[string]string{"path": orig, "newName": "new.txt"}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/files/delete",
		mustJSON(t, map[string][]string{"paths": []string{filepath.Join(root, "new.txt")}}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
}

// TestFilesRenamesBatch verifies the batch rename endpoint renames every entry
// and reports partial failures per-item (OpResult).
func TestFilesRenamesBatch(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/files/renames",
		mustJSON(t, map[string]any{"renames": []map[string]string{
			{"path": a, "newName": "renamed-a.txt"},
			{"path": b, "newName": "bad/name.txt"}, // invalid name → per-item error
		}}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("batch rename status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Done   int `json:"done"`
		Errors []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode batch rename body: %v", err)
	}
	if out.Done != 1 || len(out.Errors) != 1 {
		t.Fatalf("batch rename result = %+v, want done=1 + 1 error", out)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed-a.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("invalid rename must not touch original: %v", err)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/files/renames",
		mustJSON(t, map[string]any{"renames": []map[string]string{}}), cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty batch rename status = %d, want 400 (body %s)", resp.StatusCode, body)
	}
}

// TestFilesPins verifies pin add/list/remove against the settings table.
func TestFilesPins(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "POST", ts.URL+"/api/files/pins",
		mustJSON(t, map[string]string{"path": `C:\Videos`}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin add status = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/files/pins", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin list status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Pins []string `json:"pins"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode pins: %v", err)
	}
	if len(out.Pins) != 1 || out.Pins[0] != `C:\Videos` {
		t.Fatalf("pins = %+v, want [C:\\Videos]", out.Pins)
	}

	resp, body = doJSON(t, "DELETE", ts.URL+"/api/files/pins?path="+url.QueryEscape(`C:\Videos`), "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin remove status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/files/pins", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin list after remove status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode pins after remove: %v", err)
	}
	if len(out.Pins) != 0 {
		t.Fatalf("pins after remove = %+v, want empty", out.Pins)
	}
}

// TestFilesCopyEnqueue verifies copy enqueues a background job.
func TestFilesCopyEnqueue(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	src := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, body := doJSON(t, "POST", ts.URL+"/api/files/copy",
		mustJSON(t, map[string]any{"paths": []string{src}, "dest": t.TempDir()}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("copy enqueue status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		OK    bool   `json:"ok"`
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode copy response: %v", err)
	}
	if !out.OK || out.JobID == "" {
		t.Fatalf("copy response = %+v, want ok+job_id", out)
	}
}
