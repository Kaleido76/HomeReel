package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

func seedSeries(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx := context.Background()
	sources := store.NewSourceRepo(database)
	if err := sources.Create(ctx, domain.MediaSource{
		ID: "s1", Path: t.TempDir(), CreatedAt: ts2026,
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	videos := store.NewVideoRepo(database)
	shows := store.NewShowRepo(database)
	series := store.NewSeriesRepo(database)

	for _, show := range []domain.Show{
		{ID: "show1", Name: "Breaking Bad", MetadataSource: "manual", CreatedAt: ts2026, UpdatedAt: ts2026},
		{ID: "show2", Name: "Better Call Saul", MetadataSource: "manual", CreatedAt: ts2026, UpdatedAt: ts2026},
	} {
		if err := shows.Create(ctx, show); err != nil {
			t.Fatalf("seed show: %v", err)
		}
	}
	for _, ep := range []struct {
		id, show string
		season   int
		rel      string
	}{
		{"e1", "show1", 1, "Breaking.Bad.S01E01.mkv"},
		{"e2", "show1", 2, "Breaking.Bad.S02E01.mkv"},
		{"e3", "show2", 1, "Better.Call.Saul.S01E01.mkv"},
	} {
		if err := videos.Create(ctx, domain.Video{
			ID: ep.id, SourceID: "s1", FileID: "f" + ep.id, RelativePath: ep.rel,
			Path: t.TempDir() + "\\" + ep.rel, Title: titleOf(ep.rel),
			CreatedAt: ts2026, UpdatedAt: ts2026, LastScannedAt: ts2026,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := shows.AssignSeason(ctx, map[string]string{"show1": "Breaking Bad", "show2": "Better Call Saul"}[ep.show],
			ep.season, []domain.EpisodeAssign{{VideoID: ep.id, EpisodeNumber: 1, Title: ep.rel}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := series.SyncShowLinks(ctx, "show1"); err != nil {
		t.Fatal(err)
	}
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
	// Series are ordered by show name then season number.
	if out.Series[0].Title != "Better Call Saul" || out.Series[0].MemberCount != 1 {
		t.Errorf("series[0] = %+v", out.Series[0])
	}
	if out.Series[2].Title != "Breaking Bad" || out.Series[2].SeasonNumber != 2 {
		t.Errorf("series[2] = %+v", out.Series[2])
	}

	// Detail includes members and the auto link to season 2.
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
		Links []struct {
			LinkedID string `json:"linked_id"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 1 {
		t.Errorf("members = %d, want 1", len(detail.Members))
	}
	if len(detail.Links) != 1 || detail.Links[0].LinkedID != out.Series[2].ID {
		t.Errorf("links = %+v", detail.Links)
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
	// Only season 1 of Breaking Bad carries the tagged episode e1.
	resp, body = doJSON(t, "GET", ts.URL+"/api/series?tag="+url.QueryEscape("犯罪"), "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "第1季") || strings.Contains(body, "第2季") || strings.Contains(body, "Better Call Saul") {
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
