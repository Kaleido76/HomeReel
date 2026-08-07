package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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

// TestFs2List verifies absolute-path directory listing with size/mtime.
func TestFs2List(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := ts.URL + "/api/fs2/list?path=" + url.QueryEscape(root)
	resp, body := doJSON(t, "GET", u, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fs2 list status = %d, want 200 (body %s)", resp.StatusCode, body)
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
		t.Fatalf("decode fs2 list: %v", err)
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

// TestFs2RenameDelete verifies synchronous rename and permanent delete.
func TestFs2RenameDelete(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	root := t.TempDir()
	orig := filepath.Join(root, "old.txt")
	if err := os.WriteFile(orig, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/fs2/rename",
		mustJSON(t, map[string]string{"path": orig, "newName": "new.txt"}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	resp, body = doJSON(t, "POST", ts.URL+"/api/fs2/delete",
		mustJSON(t, map[string][]string{"paths": []string{filepath.Join(root, "new.txt")}}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
}

// TestFs2Pins verifies pin add/list/remove against the settings table.
func TestFs2Pins(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "POST", ts.URL+"/api/fs2/pins",
		mustJSON(t, map[string]string{"path": `C:\Videos`}), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin add status = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/fs2/pins", "", cookie)
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

	resp, body = doJSON(t, "DELETE", ts.URL+"/api/fs2/pins?path="+url.QueryEscape(`C:\Videos`), "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin remove status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/fs2/pins", "", cookie)
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

// TestFs2CopyEnqueue verifies copy enqueues a background job.
func TestFs2CopyEnqueue(t *testing.T) {
	ts, cookie := newTestServer(t, "secret")
	cookie = loginCookie(t, ts, "secret")

	src := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, body := doJSON(t, "POST", ts.URL+"/api/fs2/copy",
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
