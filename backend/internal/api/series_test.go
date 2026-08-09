package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

func seedSeries(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	sources := store.NewSourceRepo(database)
	if err := sources.Create(ctx, domain.MediaSource{
		ID: "s1", Path: root, CreatedAt: ts2026,
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	videos := store.NewVideoRepo(database)
	series := store.NewSeriesRepo(database)
	mk := func(id, rel string) {
		if err := videos.Create(ctx, domain.Video{
			ID: id, SourceID: "s1", FileID: "f" + id, RelativePath: rel,
			Path: root + "\\" + rel, Title: titleOf(rel),
			CreatedAt: ts2026, UpdatedAt: ts2026, LastScannedAt: ts2026,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("e1", "Breaking.Bad.S01E01.mkv")
	mk("e2", "Breaking.Bad.S02E01.mkv")
	mk("e3", "Better.Call.Saul.S01E01.mkv")

	// 三个独立系列（每个根目录一个 show）：同名目录也是独立系列。
	bcRoot := filepath.Join(root, "Better Call Saul")
	bbARoot := filepath.Join(root, "Breaking Bad A")
	bbBRoot := filepath.Join(root, "Breaking Bad B")
	for _, d := range []string{bcRoot, bbARoot, bbBRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bc := mustSeries(t, series, ctx, "Better Call Saul", bcRoot)
	bb1 := mustSeries(t, series, ctx, "Breaking Bad", bbARoot)
	bb2 := mustSeries(t, series, ctx, "Breaking Bad", bbBRoot)
	if err := series.BindMembers(ctx, bc.ID, []domain.EpisodeAssign{{VideoID: "e3", EpisodeNumber: 1, Title: "e3"}}); err != nil {
		t.Fatal(err)
	}
	if err := series.BindMembers(ctx, bb1.ID, []domain.EpisodeAssign{{VideoID: "e1", EpisodeNumber: 1, Title: "e1"}}); err != nil {
		t.Fatal(err)
	}
	if err := series.BindMembers(ctx, bb2.ID, []domain.EpisodeAssign{{VideoID: "e2", EpisodeNumber: 1, Title: "e2"}}); err != nil {
		t.Fatal(err)
	}
	_ = bb2
}

func mustSeries(t *testing.T, series domain.SeriesRepo, ctx context.Context, name, root string) domain.Series {
	t.Helper()
	s, err := series.CreateAtRoot(ctx, name, root)
	if err != nil {
		t.Fatalf("create series %s: %v", name, err)
	}
	return s
}

func TestSeriesListAndDetail(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	resp, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("series list = %d (body %s)", resp.StatusCode, body)
	}
	var out struct {
		Series []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Name         string `json:"name"`
			Kind         string `json:"kind"`
			SeasonNumber int    `json:"season_number"`
			MemberCount  int    `json:"member_count"`
		} `json:"series"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Series) != 3 {
		t.Fatalf("series = %d, want 3", len(out.Series))
	}
	// Series are ordered by show name then season number; same-named roots are
	// independent series (no merged show).
	if out.Series[0].Title != "Better Call Saul" || out.Series[0].MemberCount != 1 {
		t.Errorf("series[0] = %+v", out.Series[0])
	}
	if out.Series[1].Title != "Breaking Bad" || out.Series[1].SeasonNumber != 1 {
		t.Errorf("series[1] = %+v", out.Series[1])
	}

	// Detail includes members and the on-demand disk check.
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+out.Series[1].ID, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("series detail = %d (body %s)", resp.StatusCode, body)
	}
	var detail struct {
		Series struct {
			Name string `json:"name"`
		} `json:"series"`
		Members []struct {
			VideoID string `json:"video_id"`
		} `json:"members"`
		Check struct {
			RootExists bool `json:"root_exists"`
		} `json:"check"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 1 {
		t.Errorf("members = %d, want 1", len(detail.Members))
	}
	if !detail.Check.RootExists {
		t.Errorf("seeded root folder should exist on disk")
	}
}

func TestSeriesListFilter(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")
	if err := store.NewVideoRepo(database).SetTags(context.Background(), "e1", []string{"犯罪"}); err != nil {
		t.Fatalf("set tags: %v", err)
	}

	resp, body := doJSON(t, "GET", ts.URL+"/api/series?q="+url.QueryEscape("Breaking"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Breaking Bad") || strings.Contains(body, "Better Call Saul") {
		t.Fatalf("series q = %d %s", resp.StatusCode, body)
	}
	// Only season 1 of Breaking Bad carries the tagged episode e1; the series
	// display name is just the title (no "第 N 季" suffix).
	resp, body = doJSON(t, "GET", ts.URL+"/api/series?tag="+url.QueryEscape("犯罪"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Breaking Bad") || strings.Contains(body, "Better Call Saul") || strings.Contains(body, "第") {
		t.Fatalf("series tag = %d %s", resp.StatusCode, body)
	}
}

func TestSeriesLinkCRUD(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	var out struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	_, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// Link Better Call Saul's season to Breaking Bad season 1 manually.
	a := out.Series[0].ID
	b := out.Series[1].ID
	resp, _ := doJSON(t, "POST", ts.URL+"/api/series/"+a+"/links",
		`{"series_id":"`+b+`"}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add link = %d", resp.StatusCode)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+a+"/links", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, b) {
		t.Fatalf("links after add = %d (body %s)", resp.StatusCode, body)
	}
	resp, _ = doJSON(t, "DELETE", ts.URL+"/api/series/"+a+"/links/"+b, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove link = %d", resp.StatusCode)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+a+"/links", "", cookie)
	if resp.StatusCode != http.StatusOK || strings.Contains(body, b) {
		t.Fatalf("links after remove = %d (body %s)", resp.StatusCode, body)
	}
}

func TestSeriesPosterFallback(t *testing.T) {
	ts, _, database := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	var out struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	_, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// No poster and no thumb files → 404, not 500.
	resp, body := doJSON(t, "GET", ts.URL+"/api/series/"+out.Series[0].ID+"/poster", "", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("poster = %d, want 404 (body %s)", resp.StatusCode, body)
	}
}
