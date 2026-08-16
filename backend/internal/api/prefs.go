package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"homereel/backend/internal/domain"
)

// 播放选择记忆（ADR-006 player prefs 修订）：series + per-video 的音轨/字幕/音量
// 偏好缓存。系列剧集共享同一记忆（按**轨道名称**匹配，见 SeriesPlaybackPrefs）；
// 单集自己的记录作为关系重组后的兜底——系列有记录时优先（「忽略单集记录」）。
// 播放器进入时读取并自动应用，仅用户手动切换时刷新；作为「缓存」由工具页缓存
// 管理删除（DELETE /api/cache/prefs… / DELETE /api/series/{id}/prefs），与续播
// history 完全分离。

// videoSeriesID resolves the series a video belongs to (empty when standalone).
func (s *Server) videoSeriesID(ctx context.Context, v *domain.Video) string {
	if v.ShowID == "" || v.SeasonNumber <= 0 {
		return ""
	}
	id, err := s.series.FindID(ctx, v.ShowID, v.SeasonNumber)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			slog.Warn("find video series", "video_id", v.ID, "err", err)
		}
		return ""
	}
	return id
}

// handlePrefsGet returns a video's effective playback selection cache (null when
// absent). The response carries series_id whenever the video belongs to a series
// (the player then writes series-scoped on manual changes) and scope tells which
// record the values come from: "series" rows expose the remembered track NAMES
// (matched against this episode's own tracks by the frontend), "video" rows the
// concrete per-video selections.
func (s *Server) handlePrefsGet(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	seriesID := s.videoSeriesID(r.Context(), v)
	if seriesID != "" {
		p, err := s.prefs.GetSeries(r.Context(), seriesID, "local")
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"series_id": seriesID,
				"prefs": map[string]any{
					"scope":            "series",
					"audio_track_name": p.AudioTrackName,
					"subtitle_name":    p.SubtitleName,
					"volume":           p.Volume,
					"muted":            p.Muted,
					"updated_at":       p.UpdatedAt,
				},
			})
			return
		}
		if !errors.Is(err, domain.ErrNotFound) {
			slog.Error("get series playback prefs", "series_id", seriesID, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
			return
		}
	}
	p, err := s.prefs.Get(r.Context(), v.ID, "local")
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"prefs": nil, "series_id": seriesID})
			return
		}
		slog.Error("get playback prefs", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series_id": seriesID,
		"prefs": map[string]any{
			"scope":       "video",
			"audio_track": p.AudioTrack,
			"subtitle_id": p.SubtitleID,
			"volume":      p.Volume,
			"muted":       p.Muted,
			"updated_at":  p.UpdatedAt,
		},
	})
}

// handlePrefsPut writes a partial playback-selection update: only the fields the
// body carries are stored, the rest of the row keeps its previous value (or stays
// unset for a fresh video). Called by the player only when the user manually
// changes a selection, never when a remembered value is auto-applied.
//
// When the video belongs to a series the write is series-scoped: the player sends
// track NAMES (audio_track_name/subtitle_name) so the choice is shared across
// every episode by label, and volume/muted land in the same row. Standalone
// videos keep the concrete per-video fields (audio_track/subtitle_id).
func (s *Server) handlePrefsPut(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		AudioTrack     *int     `json:"audio_track"`
		SubtitleID     *string  `json:"subtitle_id"`
		AudioTrackName *string  `json:"audio_track_name"`
		SubtitleName   *string  `json:"subtitle_name"`
		Volume         *float64 `json:"volume"`
		Muted          *bool    `json:"muted"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.AudioTrack != nil && *req.AudioTrack < 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "音轨序号不能为负")
		return
	}
	if req.Volume != nil && (*req.Volume < 0 || *req.Volume > 1) {
		writeError(w, http.StatusBadRequest, "invalid_input", "音量必须在 0~1 之间")
		return
	}
	if seriesID := s.videoSeriesID(r.Context(), v); seriesID != "" {
		// A series member writes series-scoped when any series field (track name
		// or the shared volume) is carried; an empty patch must not mint an empty
		// row that would then shadow the video's own record.
		seriesPatch := domain.SeriesPlaybackPrefsPatch{
			AudioTrackName: req.AudioTrackName,
			SubtitleName:   req.SubtitleName,
			Volume:         req.Volume,
			Muted:          req.Muted,
		}
		if seriesPatch.AudioTrackName != nil || seriesPatch.SubtitleName != nil ||
			seriesPatch.Volume != nil || seriesPatch.Muted != nil {
			if err := s.prefs.PatchSeries(r.Context(), seriesID, "local", seriesPatch); err != nil {
				slog.Error("save series playback prefs", "series_id", seriesID, "err", err)
				writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
				return
			}
		}
		// Defensive: a concrete per-video selection on a series member still lands
		// in its own row (the series row wins on read, so this never shadows it).
		if req.AudioTrack != nil || req.SubtitleID != nil {
			if err := s.prefs.Patch(r.Context(), v.ID, "local", domain.PlaybackPrefsPatch{
				AudioTrack: req.AudioTrack,
				SubtitleID: req.SubtitleID,
			}); err != nil {
				slog.Error("save playback prefs", "video_id", v.ID, "err", err)
				writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"saved": true})
		return
	}
	if err := s.prefs.Patch(r.Context(), v.ID, "local", domain.PlaybackPrefsPatch{
		AudioTrack: req.AudioTrack,
		SubtitleID: req.SubtitleID,
		Volume:     req.Volume,
		Muted:      req.Muted,
	}); err != nil {
		slog.Error("save playback prefs", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// handleSeriesPrefsGet returns a series' playback selection cache (null when
// absent), the raw remembered track names shared by every episode.
func (s *Server) handleSeriesPrefsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	p, err := s.prefs.GetSeries(r.Context(), id, "local")
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"prefs": nil})
			return
		}
		slog.Error("get series playback prefs", "series_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prefs": p})
}

// handleSeriesPrefsDelete clears a series' playback selection cache (details
// fall back to each episode's own video row on the next play).
func (s *Server) handleSeriesPrefsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	n, err := s.prefs.DeleteSeries(r.Context(), id, "local")
	if err != nil {
		slog.Error("clear series playback prefs", "series_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "清理失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}
