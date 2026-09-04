package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/streaming"
)

// Cache management (工具页「缓存管理」): statistics + manual clearing of the
// regenerable cover/thumb/subtitle caches under data_dir. Caches are purged
// automatically on video delete/update; these handlers let the user clean them
// manually. The granularity mirrors the UI: covers/thumbs are only ever cleared
// as orphans (in-use ones are never deleted), while subtitles can be cleared
// per video / per track since they are the most frequently stale.
func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	videos, err := s.videos.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "读取视频索引失败")
		return
	}
	ids := make(map[string]struct{}, len(videos))
	vByID := make(map[string]domain.Video, len(videos))
	for _, v := range videos {
		ids[v.ID] = struct{}{}
		vByID[v.ID] = v
	}
	shows, err := s.shows.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "读取系列索引失败")
		return
	}
	showTitles := make(map[string]string, len(shows))
	for _, sh := range shows {
		showTitles[sh.ID] = sh.Name
	}

	// Extracted subtitles of still-indexed videos, grouped per video (orphan
	// files are excluded here; they surface through the orphans overview).
	groups := map[string]*cacheSubtitleGroup{}
	var order []string
	for _, f := range s.streaming.ListSubtitleCache() {
		v, ok := vByID[f.VideoID]
		if !ok {
			continue
		}
		g, ok := groups[v.ID]
		if !ok {
			g = &cacheSubtitleGroup{VideoID: v.ID, Title: v.Title, ShowID: v.ShowID, ShowTitle: showTitles[v.ShowID]}
			groups[v.ID] = g
			order = append(order, v.ID)
		}
		g.Files = append(g.Files, cacheSubtitleFile{Track: f.Track, Name: f.Name, Bytes: f.Bytes})
		g.Bytes += f.Bytes
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := groups[order[i]], groups[order[j]]
		if a.ShowTitle != b.ShowTitle {
			return a.ShowTitle < b.ShowTitle
		}
		return a.Title < b.Title
	})
	out := make([]cacheSubtitleGroup, 0, len(order))
	for _, id := range order {
		out = append(out, *groups[id])
	}

	// Remuxed MP4 caches, grouped per video (ADR-006 remux tier) so the UI can
	// show where the (usually dominant) space lives — per series / per episode.
	remuxGroups := map[string]*cacheRemuxGroup{}
	var remuxOrder []string
	for _, f := range s.streaming.ListRemuxCache() {
		v, ok := vByID[f.VideoID]
		if !ok {
			continue
		}
		g, ok := remuxGroups[v.ID]
		if !ok {
			g = &cacheRemuxGroup{VideoID: v.ID, Title: v.Title, ShowID: v.ShowID, ShowTitle: showTitles[v.ShowID]}
			remuxGroups[v.ID] = g
			remuxOrder = append(remuxOrder, v.ID)
		}
		g.Files++
		g.Bytes += f.Bytes
	}
	sort.Slice(remuxOrder, func(i, j int) bool {
		a, b := remuxGroups[remuxOrder[i]], remuxGroups[remuxOrder[j]]
		if a.ShowTitle != b.ShowTitle {
			return a.ShowTitle < b.ShowTitle
		}
		return a.Title < b.Title
	})
	remuxOut := make([]cacheRemuxGroup, 0, len(remuxOrder))
	for _, id := range remuxOrder {
		remuxOut = append(remuxOut, *remuxGroups[id])
	}

	// 播放选择记忆（音轨/字幕/音量偏好缓存）：按视频列出，删除后播放器回到
	// 默认轨/默认字幕/默认音量（可重建缓存，与续播 history 无关）。
	prefsRows, err := s.prefs.ListAll(r.Context(), "local")
	if err != nil {
		slog.Warn("list playback prefs", "err", err)
	}
	prefs := make([]cachePrefsEntry, 0, len(prefsRows))
	for _, p := range prefsRows {
		v, ok := vByID[p.VideoID]
		if !ok {
			continue
		}
		entry := cachePrefsEntry{
			VideoID:   p.VideoID,
			Title:     v.Title,
			ShowID:    v.ShowID,
			ShowTitle: showTitles[v.ShowID],
			AudioTrack: p.AudioTrack,
			SubtitleID: p.SubtitleID,
			Volume:     p.Volume,
			Muted:      p.Muted,
			UpdatedAt:  p.UpdatedAt,
		}
		prefs = append(prefs, entry)
	}
	sort.Slice(prefs, func(i, j int) bool {
		if prefs[i].ShowTitle != prefs[j].ShowTitle {
			return prefs[i].ShowTitle < prefs[j].ShowTitle
		}
		return prefs[i].Title < prefs[j].Title
	})

	// 系列级播放选择记忆（ADR-006 player prefs 修订）：按系列列出共享的记忆
	// （音轨/字幕按名称），单条删除走 DELETE /api/series/{id}/prefs。
	seriesPrefsRows, err := s.prefs.ListAllSeries(r.Context(), "local")
	if err != nil {
		slog.Warn("list series playback prefs", "err", err)
	}
	seriesByName := make(map[string]string, len(seriesPrefsRows))
	if len(seriesPrefsRows) > 0 {
		if all, err := s.series.List(r.Context(), domain.SeriesQuery{}); err == nil {
			for _, se := range all {
				seriesByName[se.ID] = se.Name
			}
		} else {
			slog.Warn("list series for prefs", "err", err)
		}
	}
	seriesPrefs := make([]cacheSeriesPrefsEntry, 0, len(seriesPrefsRows))
	for _, p := range seriesPrefsRows {
		seriesPrefs = append(seriesPrefs, cacheSeriesPrefsEntry{
			SeriesID:       p.SeriesID,
			Title:          seriesByName[p.SeriesID],
			AudioTrackName: p.AudioTrackName,
			SubtitleName:   p.SubtitleName,
			Volume:         p.Volume,
			Muted:          p.Muted,
			UpdatedAt:      p.UpdatedAt,
		})
	}
	sort.Slice(seriesPrefs, func(i, j int) bool {
		return seriesPrefs[i].Title < seriesPrefs[j].Title
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"orphans":      s.streaming.CacheOverview(ids),
		"subtitles":    out,
		"remuxes":      remuxOut,
		"prefs":        prefs,
		"series_prefs": seriesPrefs,
	})
}

type cacheSubtitleFile struct {
	Track int    `json:"track"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type cacheSubtitleGroup struct {
	VideoID   string              `json:"video_id"`
	Title     string              `json:"title"`
	ShowID    string              `json:"show_id,omitempty"`
	ShowTitle string              `json:"show_title,omitempty"`
	Files     []cacheSubtitleFile `json:"files"`
	Bytes     int64               `json:"bytes"`
}

// cacheRemuxGroup is one video's cached remux MP4s (default + per-audio-track
// outputs). It is reported per video so the UI can attribute the remux space to
// the owning series / episode.
type cacheRemuxGroup struct {
	VideoID   string `json:"video_id"`
	Title     string `json:"title"`
	ShowID    string `json:"show_id,omitempty"`
	ShowTitle string `json:"show_title,omitempty"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
}

// cachePrefsEntry is one video's playback selection cache as shown by the cache
// manager. Pointer fields are omitted when unset, mirroring the prefs API.
type cachePrefsEntry struct {
	VideoID    string   `json:"video_id"`
	Title      string   `json:"title"`
	ShowID     string   `json:"show_id,omitempty"`
	ShowTitle  string   `json:"show_title,omitempty"`
	AudioTrack *int     `json:"audio_track,omitempty"`
	SubtitleID *string  `json:"subtitle_id,omitempty"`
	Volume     *float64 `json:"volume,omitempty"`
	Muted      *bool    `json:"muted,omitempty"`
	UpdatedAt  string   `json:"updated_at"`
}

// cacheSeriesPrefsEntry is one series' shared playback selection cache as shown
// by the cache manager. Tracks are stored by name (ADR-006 player prefs 修订).
type cacheSeriesPrefsEntry struct {
	SeriesID       string   `json:"series_id"`
	Title          string   `json:"title"`
	AudioTrackName *string  `json:"audio_track_name,omitempty"`
	SubtitleName   *string  `json:"subtitle_name,omitempty"`
	Volume         *float64 `json:"volume,omitempty"`
	Muted          *bool    `json:"muted,omitempty"`
	UpdatedAt      string   `json:"updated_at"`
}

// handleCacheSubtitleClear deletes every extracted-subtitle cache file of one
// video (used when a subtitle is stale or a track was re-extracted badly).
func (s *Server) handleCacheSubtitleClear(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.streaming.ClearSubtitles(r.PathValue("videoId"))})
}

// handleCacheSubtitleTrackClear deletes one extracted-subtitle cache file.
func (s *Server) handleCacheSubtitleTrackClear(w http.ResponseWriter, r *http.Request) {
	track, err := strconv.Atoi(r.PathValue("track"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的字幕轨序号")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.streaming.ClearSubtitleTrack(r.PathValue("videoId"), track)})
}

// clearSeriesCache runs one cache clear across every member of the series in
// the request path and returns the total number of removed files. It is the
// shared tail of the series-level「清理字幕」/「清理 Remux」handlers.
func (s *Server) clearSeriesCache(r *http.Request, clear func(videoID string) int) (int, error) {
	members, err := s.series.GetMembers(r.Context(), r.PathValue("seriesId"))
	if err != nil {
		return 0, err
	}
	var cleared int
	for _, m := range members {
		cleared += clear(m.VideoID)
	}
	return cleared, nil
}

// handleCacheSeriesSubtitleClear clears the extracted-subtitle cache of every
// member of a series in one request (cache-manager series-level「清理字幕」).
func (s *Server) handleCacheSeriesSubtitleClear(w http.ResponseWriter, r *http.Request) {
	cleared, err := s.clearSeriesCache(r, s.streaming.ClearSubtitles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "读取系列成员失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": cleared})
}

// handleCacheSeriesRemuxClear clears the cached remux MP4s of every member of a
// series in one request (cache-manager series-level「清理 Remux」).
func (s *Server) handleCacheSeriesRemuxClear(w http.ResponseWriter, r *http.Request) {
	cleared, err := s.clearSeriesCache(r, s.streaming.ClearRemux)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "读取系列成员失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": cleared})
}

// handleCacheRemuxVideoClear clears one video's cached remux MP4s (the
// standalone-cache detail「清理 Remux」).
func (s *Server) handleCacheRemuxVideoClear(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.streaming.ClearRemux(r.PathValue("videoId"))})
}

// handleCachePregen enqueues a background job that pre-extracts every target
// video's embedded text subtitles into the cache (cache-manager「预生成缓存」).
// The target is a series (series_id) or an explicit set of videos (video_ids,
// e.g. one standalone episode); the job runs through the standard queue so
// progress is visible in the task panel.
func (s *Server) handleCachePregen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SeriesID string   `json:"series_id"`
		VideoIDs []string `json:"video_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体格式错误")
		return
	}

	videoIDs := append([]string(nil), body.VideoIDs...)
	var title string
	if body.SeriesID != "" {
		se, err := s.series.Get(r.Context(), body.SeriesID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "系列不存在")
			return
		}
		members, err := s.series.GetMembers(r.Context(), body.SeriesID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "读取系列成员失败")
			return
		}
		for _, m := range members {
			videoIDs = append(videoIDs, m.VideoID)
		}
		title = se.Name
	}
	videoIDs = uniqueNonEmpty(videoIDs)
	if len(videoIDs) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "没有可预生成的视频")
		return
	}

	items := make([]streaming.PregenItem, 0, len(videoIDs))
	for _, id := range videoIDs {
		v, err := s.videos.Get(r.Context(), id)
		if err != nil {
			continue
		}
		items = append(items, streaming.PregenItem{VideoID: v.ID, Title: v.Title, Path: v.Path})
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "所选视频均不在库中")
		return
	}
	if len(items) == 1 {
		title = items[0].Title
	}

	// Dedup by the resource the job owns: a whole series, or a lone video.
	target := body.SeriesID
	if target == "" {
		target = items[0].VideoID
		if len(items) > 1 {
			target = "multi"
		}
	}
	active, err := s.jobs.HasActive(r.Context(), jobs.TypePregen, target)
	if err != nil {
		slog.Warn("check pregen active", "err", err)
	}
	if active {
		writeError(w, http.StatusConflict, "already_running", "该目标已有进行中的预生成任务")
		return
	}

	extra, err := json.Marshal(streaming.PregenMeta{Items: items})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "任务编码失败")
		return
	}
	id, err := s.jobs.Enqueue(r.Context(), jobs.TypePregen, target, "预生成字幕缓存 · "+title, string(extra))
	if err != nil {
		slog.Error("enqueue pregen", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "任务提交失败")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

// uniqueNonEmpty dedupes and drops empty ids from a video id list.
func uniqueNonEmpty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("kind") != "subtitle" {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的缓存类型")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.streaming.ClearAllSubtitles()})
}

func (s *Server) handleCacheOrphans(w http.ResponseWriter, r *http.Request) {
	ids, err := s.videoIDSet(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "读取视频索引失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.streaming.ClearOrphans(ids)})
}

// handleCachePrefsClear deletes every playback selection cache row (per-video
// AND series-scoped, so the cache manager「清空全部」clears the shared memories too).
func (s *Server) handleCachePrefsClear(w http.ResponseWriter, r *http.Request) {
	n, err := s.prefs.DeleteAll(r.Context(), "local")
	if err != nil {
		slog.Error("clear playback prefs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "清空失败")
		return
	}
	ns, err := s.prefs.DeleteAllSeries(r.Context(), "local")
	if err != nil {
		slog.Error("clear series playback prefs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "清空失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n + ns})
}

// handleCachePrefsVideoClear deletes one video's playback selection cache row.
func (s *Server) handleCachePrefsVideoClear(w http.ResponseWriter, r *http.Request) {
	n, err := s.prefs.Delete(r.Context(), r.PathValue("videoId"), "local")
	if err != nil {
		slog.Error("clear playback prefs", "video_id", r.PathValue("videoId"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "清理失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

// videoIDSet loads the ids of every indexed video, used to tell orphan caches
// (belonging to removed videos) apart from live ones.
func (s *Server) videoIDSet(ctx context.Context) (map[string]struct{}, error) {
	videos, err := s.videos.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(videos))
	for _, v := range videos {
		ids[v.ID] = struct{}{}
	}
	return ids, nil
}
