package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

const ts2026 = "2026-01-01T00:00:00.000000000Z"

func seedLibrary(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx := context.Background()
	storages := store.NewStorageRepo(database)
	if err := storages.Create(ctx, domain.Storage{
		ID: "s1", Name: "test", Type: domain.StorageTypeInternal,
		RootPath: t.TempDir(), Available: true, CreatedAt: ts2026,
	}); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	videos := store.NewVideoRepo(database)
	mk := func(id, rel string) domain.Video {
		return domain.Video{
			ID: id, StorageID: "s1", FileID: "f" + id, RelativePath: rel,
			Path: t.TempDir() + "\\" + rel, Size: 1, MTime: 1, Title: titleOf(rel),
			CreatedAt: ts2026, UpdatedAt: ts2026, LastScannedAt: ts2026,
		}
	}
	for id, rel := range map[string]string{"m1": "Interstellar.mkv", "m2": "Dunkirk.mkv"} {
		if err := videos.Create(ctx, mk(id, rel)); err != nil {
			t.Fatalf("seed video %s: %v", id, err)
		}
	}
	if err := videos.SetTags(ctx, "m1", []string{"科幻"}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	shows := store.NewShowRepo(database)
	if err := shows.Create(ctx, domain.Show{
		ID: "show1", Name: "Breaking Bad", MetadataSource: "manual",
		CreatedAt: ts2026, UpdatedAt: ts2026,
	}); err != nil {
		t.Fatalf("seed show: %v", err)
	}
	if _, err := shows.EnsureSeason(ctx, "show1", 1, "tv"); err != nil {
		t.Fatalf("seed season: %v", err)
	}
	episodes := map[string]domain.Video{
		"e1": mk("e1", "Breaking.Bad.S01E01.mkv"),
		"e2": mk("e2", "Breaking.Bad.S01E02.mkv"),
	}
	for id, v := range episodes {
		if err := videos.Create(ctx, v); err != nil {
			t.Fatalf("seed episode %s: %v", id, err)
		}
		if err := videos.AssignEpisode(ctx, id, "show1", 1, map[string]int{"e1": 1, "e2": 2}[id], "Ep "+id); err != nil {
			t.Fatalf("assign episode %s: %v", id, err)
		}
	}
	history := store.NewHistoryRepo(database)
	if err := history.Upsert(ctx, domain.History{VideoID: "m1", User: "local", Progress: 40, UpdatedAt: "2026-01-02T00:00:00.000000000Z"}); err != nil {
		t.Fatalf("seed history: %v", err)
	}
	if err := videos.UpdateProbe(ctx, domain.Video{ID: "m1", Title: "Interstellar", Duration: 100}); err != nil {
		t.Fatalf("seed probe: %v", err)
	}
}

func titleOf(rel string) string {
	base := rel
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	return base
}

func TestShowsWall(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "GET", ts.URL+"/api/shows", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("shows = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Shows []struct {
			ID             string `json:"id"`
			EpisodeCount   int    `json:"episode_count"`
			SeasonCount    int    `json:"season_count"`
			UnwatchedCount int    `json:"unwatched_count"`
		} `json:"shows"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Shows) != 1 {
		t.Fatalf("shows = %d, want 1", len(out.Shows))
	}
	s := out.Shows[0]
	if s.EpisodeCount != 2 || s.SeasonCount != 1 || s.UnwatchedCount != 2 {
		t.Errorf("show counts = %+v", s)
	}
}

func TestShowDetailAndEpisodes(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "GET", ts.URL+"/api/shows/show1", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("show detail = %d (body %s)", resp.StatusCode, body)
	}
	var detail struct {
		Show struct {
			Name string `json:"name"`
		} `json:"show"`
		Seasons []struct {
			Number       int `json:"number"`
			EpisodeCount int `json:"episode_count"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Show.Name != "Breaking Bad" || len(detail.Seasons) != 1 || detail.Seasons[0].Number != 1 {
		t.Errorf("detail = %+v", detail)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/shows/show1/seasons/1/episodes", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("episodes = %d (body %s)", resp.StatusCode, body)
	}
	var eps struct {
		Episodes []struct {
			EpisodeNumber int `json:"episode_number"`
			Progress      int `json:"progress"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal([]byte(body), &eps); err != nil {
		t.Fatal(err)
	}
	if len(eps.Episodes) != 2 || eps.Episodes[0].EpisodeNumber != 1 {
		t.Errorf("episodes = %+v", eps.Episodes)
	}
}

func TestTagsCollectionsHome(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "GET", ts.URL+"/api/tags", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "科幻") {
		t.Fatalf("tags = %d (body %s)", resp.StatusCode, body)
	}

	// Create collection + add video.
	resp, body = doJSON(t, "POST", ts.URL+"/api/collections", `{"name":"我的最爱"}`, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create collection = %d (body %s)", resp.StatusCode, body)
	}
	var created struct {
		Collection struct {
			ID string `json:"id"`
		} `json:"collection"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	cid := created.Collection.ID
	resp, _ = doJSON(t, "PUT", ts.URL+"/api/collections/"+cid+"/videos/m1", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add video = %d", resp.StatusCode)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/collections/"+cid+"/videos", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Interstellar") {
		t.Fatalf("collection videos = %d (body %s)", resp.StatusCode, body)
	}
	resp, _ = doJSON(t, "DELETE", ts.URL+"/api/collections/"+cid+"/videos/m1", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove video = %d", resp.StatusCode)
	}

	// Home rows.
	resp, body = doJSON(t, "GET", ts.URL+"/api/home", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("home = %d (body %s)", resp.StatusCode, body)
	}
	var home struct {
		ContinueWatching []struct {
			ID string `json:"id"`
		} `json:"continue_watching"`
		Recent []struct {
			ID string `json:"id"`
		} `json:"recent"`
	}
	if err := json.Unmarshal([]byte(body), &home); err != nil {
		t.Fatal(err)
	}
	if len(home.ContinueWatching) != 1 || home.ContinueWatching[0].ID != "m1" {
		t.Errorf("continue_watching = %+v", home.ContinueWatching)
	}
	if len(home.Recent) < 3 {
		t.Errorf("recent = %d rows", len(home.Recent))
	}
}

func TestSearchAndVideoPatch(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	// Search by title, tag, show name.
	for _, q := range []string{"Interstellar", "科幻", "Breaking"} {
		resp, body := doJSON(t, "GET", ts.URL+"/api/search?q="+q, "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("search %s = %d (body %s)", q, resp.StatusCode, body)
		}
		var out struct {
			Videos []struct {
				ID string `json:"id"`
			} `json:"videos"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Videos) == 0 {
			t.Errorf("search %q returned no results", q)
		}
	}

	// PATCH metadata + tags.
	resp, body := doJSON(t, "PATCH", ts.URL+"/api/videos/m1",
		`{"title":"星际穿越","description":"desc","tags":["太空","神作"]}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d (body %s)", resp.StatusCode, body)
	}
	var patched struct {
		Video struct {
			Title          string `json:"title"`
			Description    string `json:"description"`
			MetadataSource string `json:"metadata_source"`
		} `json:"video"`
	}
	if err := json.Unmarshal([]byte(body), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Video.Title != "星际穿越" || patched.Video.Description != "desc" {
		t.Errorf("patched = %+v", patched.Video)
	}

	// Tag filter on /api/videos.
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos?tag="+"太空", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "星际穿越") {
		t.Fatalf("video filter by tag = %d (body %s)", resp.StatusCode, body)
	}
}

func TestScrapeRequiresTMDB(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "POST", ts.URL+"/api/videos/m1/scrape", `{}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("scrape without key = %d, want 400 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "TMDB") {
		t.Errorf("scrape error should mention TMDB: %s", body)
	}
}

func TestVideoDeleteCleansShow(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedLibrary(t, database)
	cookie := loginCookie(t, ts, "secret")

	// Delete both episodes → show becomes empty and is removed.
	for _, id := range []string{"e1", "e2"} {
		resp, body := doJSON(t, "DELETE", ts.URL+"/api/videos/"+id, "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delete %s = %d (body %s)", id, resp.StatusCode, body)
		}
	}
	resp, body := doJSON(t, "GET", ts.URL+"/api/shows", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("shows after delete = %d", resp.StatusCode)
	}
	if strings.Contains(body, "Breaking Bad") {
		t.Errorf("empty show should have been removed: %s", body)
	}
}
