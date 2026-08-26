package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	ts, _, database, _ := newTestServerDB(t, "secret")
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
	ts, _, database, _ := newTestServerDB(t, "secret")
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

func TestSeriesLinkGroupMutualVisibility(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
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
	if len(out.Series) < 3 {
		t.Fatalf("need at least 3 series, got %d", len(out.Series))
	}
	a, b, c := out.Series[0].ID, out.Series[1].ID, out.Series[2].ID

	// 从 A 关联 B、C（全量替换：A 与 B、C 同组）。B、C 之前无关联，直接生效。
	resp, _ := doJSON(t, "PUT", ts.URL+"/api/series/"+a+"/links",
		`{"series_ids":["`+b+`","`+c+`"]}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set links = %d", resp.StatusCode)
	}

	// 组内互相可见：A 看到 B、C；B 看到 A、C；C 看到 A、B。
	for _, tc := range []struct {
		id    string
		other string
	}{
		{a, b}, {a, c}, {b, a}, {b, c}, {c, a}, {c, b},
	} {
		resp, body := doJSON(t, "GET", ts.URL+"/api/series/"+tc.id+"/links", "", cookie)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, tc.other) {
			t.Fatalf("links of %s should contain %s (body %s)", tc.id, tc.other, body)
		}
	}

	// 取消勾选 C（B 重新管理，勾选只剩 A）→ C 离开组，A↔B 保留。
	resp, _ = doJSON(t, "PUT", ts.URL+"/api/series/"+b+"/links",
		`{"series_ids":["`+a+`"]}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-set links = %d", resp.StatusCode)
	}
	for _, tc := range []struct {
		id    string
		other string
		has   bool
	}{
		{a, b, true}, {a, c, false}, {b, a, true}, {b, c, false}, {c, a, false}, {c, b, false},
	} {
		resp, body := doJSON(t, "GET", ts.URL+"/api/series/"+tc.id+"/links", "", cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("links of %s = %d", tc.id, resp.StatusCode)
		}
		if strings.Contains(body, tc.other) != tc.has {
			t.Fatalf("links of %s should contain %s = %v (body %s)", tc.id, tc.other, tc.has, body)
		}
	}

	// × 按钮移除单条关联。
	resp, _ = doJSON(t, "DELETE", ts.URL+"/api/series/"+a+"/links/"+b, "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove link = %d", resp.StatusCode)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+a+"/links", "", cookie)
	if resp.StatusCode != http.StatusOK || strings.Contains(body, b) {
		t.Fatalf("links after remove = %d (body %s)", resp.StatusCode, body)
	}
	// 双向同样生效：B 也看不到 A。
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+b+"/links", "", cookie)
	if resp.StatusCode != http.StatusOK || strings.Contains(body, a) {
		t.Fatalf("B links after remove = %d (body %s)", resp.StatusCode, body)
	}
}

func TestSeriesClearHistory(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	ctx := context.Background()
	history := store.NewHistoryRepo(database)
	// 两个成员各写历史；独立单集（不属于任何系列）的历史不应被清。
	for _, vid := range []string{"e1", "e2"} {
		if err := history.Upsert(ctx, domain.History{VideoID: vid, User: "local", Progress: 40, UpdatedAt: ts2026}); err != nil {
			t.Fatal(err)
		}
	}
	if err := history.Upsert(ctx, domain.History{VideoID: "e3", User: "local", Progress: 50, UpdatedAt: ts2026}); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	_, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// 系列按 show 名排序：Better Call Saul(e3) / Breaking Bad A(e1) / Breaking
	// Bad B(e2)。e1 在 Breaking Bad A（bb1），清它的历史只该影响 e1。
	seriesID := out.Series[1].ID
	resp, body := doJSON(t, "DELETE", ts.URL+"/api/series/"+seriesID+"/history", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear history = %d (body %s)", resp.StatusCode, body)
	}

	if _, err := history.Get(ctx, "e1", "local"); err == nil {
		t.Fatal("e1 history should be cleared")
	} else if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get e1 history: %v", err)
	}
	for _, vid := range []string{"e2", "e3"} {
		if _, err := history.Get(ctx, vid, "local"); err != nil {
			t.Fatalf("member %s of another series must keep its history: %v", vid, err)
		}
	}

	// 不存在的系列 → 404。
	resp, _ = doJSON(t, "DELETE", ts.URL+"/api/series/nope/history", "", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown series clear = %d, want 404", resp.StatusCode)
	}
}

func TestSeriesReorder(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	ctx := context.Background()
	series := store.NewSeriesRepo(database)

	var out struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	_, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// Breaking Bad A 系列（e1 单成员），重排前后一致即可验证请求通过。
	seriesID := out.Series[1].ID
	resp, body := doJSON(t, "POST", ts.URL+"/api/series/"+seriesID+"/order",
		`{"video_ids":["e1"]}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder = %d (body %s)", resp.StatusCode, body)
	}
	got, err := series.Get(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SortManual {
		t.Fatalf("series should be marked sort_manual after reorder, got %+v", got)
	}

	// 数量不匹配 / 含外来成员 / 未知系列 → 400/404。
	if resp, _ := doJSON(t, "POST", ts.URL+"/api/series/"+seriesID+"/order",
		`{"video_ids":[]}`, cookie); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty reorder = %d, want 400", resp.StatusCode)
	}
	if resp, _ := doJSON(t, "POST", ts.URL+"/api/series/"+seriesID+"/order",
		`{"video_ids":["e1","e3"]}`, cookie); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("foreign member reorder = %d, want 400", resp.StatusCode)
	}
	if resp, _ := doJSON(t, "POST", ts.URL+"/api/series/nope/order",
		`{"video_ids":["e1"]}`, cookie); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown series reorder = %d, want 404", resp.StatusCode)
	}
}

func TestSeriesResort(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	seedSeries(t, database)
	cookie := loginCookie(t, ts, "secret")

	ctx := context.Background()
	series := store.NewSeriesRepo(database)

	var out struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	_, body := doJSON(t, "GET", ts.URL+"/api/series", "", cookie)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	// 先手动重排（sort_manual=1），再调用 resort 应恢复自动模式。
	seriesID := out.Series[1].ID
	if resp, _ := doJSON(t, "POST", ts.URL+"/api/series/"+seriesID+"/order",
		`{"video_ids":["e1"]}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder = %d", resp.StatusCode)
	}
	got, err := series.Get(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SortManual {
		t.Fatalf("series should be sort_manual before resort, got %+v", got)
	}

	resp, body := doJSON(t, "POST", ts.URL+"/api/series/"+seriesID+"/resort", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resort = %d (body %s)", resp.StatusCode, body)
	}
	got, err = series.Get(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SortManual {
		t.Fatalf("resort must clear sort_manual, got %+v", got)
	}

	// 未知系列 → 404。
	if resp, _ := doJSON(t, "POST", ts.URL+"/api/series/nope/resort", "", cookie); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown series resort = %d, want 404", resp.StatusCode)
	}
}

func TestSeriesPosterFallback(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
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
