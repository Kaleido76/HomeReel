package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/store"
)

// TestPlaybackPrefsFlow covers the full playback-selection cache lifecycle:
// absent → partial save → read back → cache manager lists → per-video and
// all-videos clearing. The selection cache must stay independent of the resume
// history (DELETE /api/videos/{id}/history only clears progress).
func TestPlaybackPrefsFlow(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

	// Absent row reads back as null.
	resp, body := doJSON(t, "GET", ts.URL+"/api/videos/v1/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"prefs":null`) {
		t.Fatalf("prefs absent = %d (body %s)", resp.StatusCode, body)
	}

	// Partial save: audio track + subtitle selection + volume.
	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/v1/prefs",
		`{"audio_track":2,"subtitle_id":"e3","volume":0.7,"muted":false}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put prefs = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/v1/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prefs = %d (body %s)", resp.StatusCode, body)
	}
	for _, want := range []string{`"audio_track":2`, `"subtitle_id":"e3"`, `"volume":0.7`} {
		if !strings.Contains(body, want) {
			t.Fatalf("get prefs missing %s in %s", want, body)
		}
	}

	// Partial update only touches the provided field; the rest is preserved.
	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/v1/prefs", `{"volume":0.3}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put prefs volume = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/v1/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prefs after volume = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"audio_track":2`) || !strings.Contains(body, `"volume":0.3`) {
		t.Fatalf("partial update broke prefs: %s", body)
	}

	// The cache manager lists the pref row grouped under the video.
	resp, body = doJSON(t, "GET", ts.URL+"/api/cache", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cache stats = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"prefs"`) || !strings.Contains(body, `"audio_track":2`) {
		t.Fatalf("cache stats missing prefs: %s", body)
	}

	// Per-video clear removes the row.
	resp, body = doJSON(t, "DELETE", ts.URL+"/api/cache/prefs/v1", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"cleared":1`) {
		t.Fatalf("clear video prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/v1/prefs", "", cookie)
	if !strings.Contains(body, `"prefs":null`) {
		t.Fatalf("prefs not cleared: %s", body)
	}
}

// TestPlaybackPrefsValidation rejects negative audio tracks and out-of-range
// volumes.
func TestPlaybackPrefsValidation(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

	resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/v1/prefs", `{"audio_track":-1}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative audio = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/v1/prefs", `{"volume":1.5}`, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("volume>1 = %d (body %s)", resp.StatusCode, body)
	}
}

// TestPlaybackPrefsCascade ensures deleting a video also drops its selection
// cache (FK ON DELETE CASCADE) and that clearing the resume history keeps the
// selection cache intact.
func TestPlaybackPrefsCascade(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)

	if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/v1/prefs", `{"audio_track":1}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put prefs = %d (body %s)", resp.StatusCode, body)
	}
	if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/v1/history", `{"progress":50}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put history = %d (body %s)", resp.StatusCode, body)
	}

	// Clearing playback history must NOT remove the selection cache.
	if resp, body := doJSON(t, "DELETE", ts.URL+"/api/videos/v1/history", "", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("clear history = %d (body %s)", resp.StatusCode, body)
	}
	if resp, body := doJSON(t, "GET", ts.URL+"/api/videos/v1/prefs", "", cookie); !strings.Contains(body, `"audio_track":1`) {
		t.Fatalf("prefs lost after clear history = %d (body %s)", resp.StatusCode, body)
	}

	// Deleting the video cascades the pref row away.
	if resp, body := doJSON(t, "DELETE", ts.URL+"/api/videos/v1", "", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete video = %d (body %s)", resp.StatusCode, body)
	}
	assertNoPrefsRow(t, database, "v1")
}

// TestPlaybackPrefsClearAll removes every pref row through the cache manager.
func TestPlaybackPrefsClearAll(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "v1", "a.mp4", "a.mp4", "Alpha", 100)
	seedVideo(t, database, sourceID, "v2", "b.mp4", "b.mp4", "Beta", 100)

	for _, id := range []string{"v1", "v2"} {
		if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/"+id+"/prefs", `{"volume":0.5}`, cookie); resp.StatusCode != http.StatusOK {
			t.Fatalf("put prefs %s = %d (body %s)", id, resp.StatusCode, body)
		}
	}
	resp, body := doJSON(t, "DELETE", ts.URL+"/api/cache/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"cleared":2`) {
		t.Fatalf("clear all prefs = %d (body %s)", resp.StatusCode, body)
	}
	assertNoPrefsRow(t, database, "v1")
	assertNoPrefsRow(t, database, "v2")
}

func assertNoPrefsRow(t *testing.T, database *sql.DB, videoID string) {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM playback_prefs WHERE video_id = ?`, videoID).Scan(&n); err != nil {
		t.Fatalf("count prefs: %v", err)
	}
	if n != 0 {
		t.Fatalf("video %s still has %d pref rows", videoID, n)
	}
}

func assertNoSeriesPrefsRow(t *testing.T, database *sql.DB, seriesID string) {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM series_playback_prefs WHERE series_id = ?`, seriesID).Scan(&n); err != nil {
		t.Fatalf("count series prefs: %v", err)
	}
	if n != 0 {
		t.Fatalf("series %s still has %d pref rows", seriesID, n)
	}
}

// seedSeriesVideo creates a series at a fresh root and binds the given video as
// its single episode, so the video resolves to a series for prefs routing.
func seedSeriesVideo(t *testing.T, database *sql.DB, videoID string) string {
	t.Helper()
	ctx := context.Background()
	series := store.NewSeriesRepo(database)
	s, err := series.CreateAtRoot(ctx, "My Show", t.TempDir())
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := series.BindMembers(ctx, s.ID, []domain.EpisodeAssign{{VideoID: videoID, EpisodeNumber: 1, Title: videoID}}); err != nil {
		t.Fatalf("bind member: %v", err)
	}
	return s.ID
}

// TestSeriesPlaybackPrefsFlow covers the series-scoped playback selection cache:
// a member video's manual choice is stored on the series (by track name), read
// back through both the video and series endpoints, listed by the cache manager,
// and cleared via DELETE /api/series/{id}/prefs. A standalone video keeps its
// own concrete per-video row.
func TestSeriesPlaybackPrefsFlow(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "e1", "s01e01.mkv", "s01e01.mkv", "Episode 1", 100)
	seedVideo(t, database, sourceID, "m1", "movie.mp4", "movie.mp4", "Movie", 100)
	seriesID := seedSeriesVideo(t, database, "e1")

	// A series member reads back prefs:null but carries its series context.
	resp, body := doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"prefs":null`) {
		t.Fatalf("series member prefs absent = %d (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"series_id":"`+seriesID+`"`) {
		t.Fatalf("series member prefs missing series_id %s in %s", seriesID, body)
	}

	// A manual selection on the member is stored on the SERIES by track name.
	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs",
		`{"audio_track_name":"国语","subtitle_name":"简体中文","volume":0.7,"muted":false}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put series prefs = %d (body %s)", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get series member prefs = %d (body %s)", resp.StatusCode, body)
	}
	for _, want := range []string{`"scope":"series"`, `"audio_track_name":"国语"`, `"subtitle_name":"简体中文"`, `"volume":0.7`} {
		if !strings.Contains(body, want) {
			t.Fatalf("series member prefs missing %s in %s", want, body)
		}
	}

	// The series endpoint exposes the same row.
	resp, body = doJSON(t, "GET", ts.URL+"/api/series/"+seriesID+"/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"audio_track_name":"国语"`) {
		t.Fatalf("series prefs = %d (body %s)", resp.StatusCode, body)
	}

	// Volume-only updates preserve the recorded track names.
	resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs", `{"volume":0.3}`, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put series volume = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if !strings.Contains(body, `"audio_track_name":"国语"`) || !strings.Contains(body, `"volume":0.3`) {
		t.Fatalf("series partial update broke prefs: %s", body)
	}

	// The cache manager lists series-level memories separately.
	resp, body = doJSON(t, "GET", ts.URL+"/api/cache", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"series_prefs"`) ||
		!strings.Contains(body, `"audio_track_name":"国语"`) {
		t.Fatalf("cache stats missing series prefs: %s", body)
	}

	// A standalone video stays on its own concrete row (no series context).
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/m1/prefs", "", cookie)
	if !strings.Contains(body, `"series_id":""`) {
		t.Fatalf("standalone prefs must carry empty series_id: %s", body)
	}
	if resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/m1/prefs", `{"audio_track":1}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put video prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/m1/prefs", "", cookie)
	if !strings.Contains(body, `"scope":"video"`) || !strings.Contains(body, `"audio_track":1`) {
		t.Fatalf("standalone video prefs = %s", body)
	}

	// Clearing the series record drops the shared memory.
	resp, body = doJSON(t, "DELETE", ts.URL+"/api/series/"+seriesID+"/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"cleared":1`) {
		t.Fatalf("clear series prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if !strings.Contains(body, `"prefs":null`) {
		t.Fatalf("series prefs not cleared: %s", body)
	}
}

// TestSeriesPlaybackPrefsPriority checks that a series record wins over the
// member's own video record (「系列有记录时忽略单集记录」).
func TestSeriesPlaybackPrefsPriority(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "e1", "s01e01.mkv", "s01e01.mkv", "Episode 1", 100)
	seriesID := seedSeriesVideo(t, database, "e1")

	// The member's own concrete record is the fallback while no series record
	// exists...
	if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs", `{"audio_track":1}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put video prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body := doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if !strings.Contains(body, `"scope":"video"`) || !strings.Contains(body, `"audio_track":1`) {
		t.Fatalf("fallback video prefs = %s", body)
	}

	// ...but once the series records a choice, it is the effective source.
	if resp, body = doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs", `{"audio_track_name":"粤语"}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put series prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if !strings.Contains(body, `"scope":"series"`) || !strings.Contains(body, `"audio_track_name":"粤语"`) {
		t.Fatalf("series prefs must win: %s", body)
	}

	// Clearing the series record falls back to the member's own row again.
	if resp, body := doJSON(t, "DELETE", ts.URL+"/api/series/"+seriesID+"/prefs", "", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("clear series prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body = doJSON(t, "GET", ts.URL+"/api/videos/e1/prefs", "", cookie)
	if !strings.Contains(body, `"scope":"video"`) || !strings.Contains(body, `"audio_track":1`) {
		t.Fatalf("fallback after series clear = %s", body)
	}
}

// TestSeriesPlaybackPrefsClearAll ensures the cache manager「清空全部」clears
// series-scoped rows too.
func TestSeriesPlaybackPrefsClearAll(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "e1", "s01e01.mkv", "s01e01.mkv", "Episode 1", 100)
	seriesID := seedSeriesVideo(t, database, "e1")

	if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs", `{"audio_track_name":"国语"}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put series prefs = %d (body %s)", resp.StatusCode, body)
	}
	resp, body := doJSON(t, "DELETE", ts.URL+"/api/cache/prefs", "", cookie)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"cleared":1`) {
		t.Fatalf("clear all prefs = %d (body %s)", resp.StatusCode, body)
	}
	assertNoSeriesPrefsRow(t, database, seriesID)
}

// TestSeriesPlaybackPrefsCascade ensures deleting a series' last member drops
// the series (show/season cascade) and with it the shared prefs row.
func TestSeriesPlaybackPrefsCascade(t *testing.T) {
	ts, _, database, _ := newTestServerDB(t, "secret")
	cookie := loginCookie(t, ts, "secret")
	sourceID := newTestSource(t, ts, cookie)
	seedVideo(t, database, sourceID, "e1", "s01e01.mkv", "s01e01.mkv", "Episode 1", 100)
	seriesID := seedSeriesVideo(t, database, "e1")

	if resp, body := doJSON(t, "PUT", ts.URL+"/api/videos/e1/prefs", `{"audio_track_name":"国语"}`, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("put series prefs = %d (body %s)", resp.StatusCode, body)
	}
	if resp, body := doJSON(t, "DELETE", ts.URL+"/api/videos/e1", "", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete video = %d (body %s)", resp.StatusCode, body)
	}
	assertNoSeriesPrefsRow(t, database, seriesID)
}
