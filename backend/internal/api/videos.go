package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/streaming"
)

// videoOrError loads a video, writing an error response when missing.
func (s *Server) videoOrError(w http.ResponseWriter, r *http.Request, id string) (*domain.Video, bool) {
	v, err := s.videos.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "视频不存在")
			return nil, false
		}
		slog.Error("get video", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return nil, false
	}
	return &v, true
}

func (s *Server) handleVideosList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	res, err := s.videos.List(r.Context(), domain.VideoQuery{
		Q:         r.URL.Query().Get("q"),
		Desc:      r.URL.Query().Get("desc"),
		Genre:     r.URL.Query().Get("genre"),
		Year:      year,
		Kind:      r.URL.Query().Get("kind"),
		Tags:      r.URL.Query()["tag"],
		ShowID:    r.URL.Query().Get("showId"),
		Ungrouped: r.URL.Query().Get("ungrouped") == "1",
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		slog.Error("list videos", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"videos":    res.Videos,
		"total":     res.Total,
		"page":      max(page, 1),
		"page_size": max(pageSize, 24),
	})
}

func (s *Server) handleVideoDetail(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	tags, err := s.videos.Tags(r.Context(), v.ID)
	if err != nil {
		slog.Error("get video tags", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	seriesID := ""
	if v.ShowID != "" && v.SeasonNumber > 0 {
		seriesID, err = s.series.FindID(r.Context(), v.ShowID, v.SeasonNumber)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			slog.Warn("find video series", "video_id", v.ID, "err", err)
		}
	}
	// 单集详情按需检查源文件：路径在 → ok；路径不在但能在媒体源内按 file_id
	// 找到（改名/移动）→ moved（可同步）；找不到 → missing（可移除）。
	status, checkErr := s.scanner.CheckVideo(r.Context(), v.ID)
	sourceStatus := "ok"
	if checkErr == nil {
		sourceStatus = status.Status
	} else {
		slog.Warn("check video source", "video_id", v.ID, "err", checkErr)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"video":           v,
		"tags":            tags,
		"series_id":       seriesID,
		"direct_playable": s.streaming.DirectPlayable(*v),
		"source_status":   sourceStatus,
		"new_path":        status.Path,
	})
}

// handleVideoSync re-locates a video's source file by file_id in its media
// source (rename/move) and converges its series membership. A file that cannot
// be found returns 404 so the UI can offer removing the record.
func (s *Server) handleVideoSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.videoOrError(w, r, id); !ok {
		return
	}
	if err := s.scanner.SyncVideo(r.Context(), id); err != nil {
		if errors.Is(err, scanner.ErrVideoMissing) {
			writeError(w, http.StatusNotFound, "not_found", "源文件不存在，请移除该单集")
			return
		}
		slog.Error("sync video", "video_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "同步失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": true})
}

// streamOrError resolves the video for any source-dependent streaming work.
func (s *Server) streamOrError(w http.ResponseWriter, r *http.Request) (*domain.Video, bool) {
	return s.videoOrError(w, r, r.PathValue("id"))
}

func (s *Server) handleStreamDirect(w http.ResponseWriter, r *http.Request) {
	v, ok := s.streamOrError(w, r)
	if !ok {
		return
	}
	if err := s.streaming.Direct(w, r, *v); err != nil {
		if errors.Is(err, streaming.ErrUnavailable) {
			writeError(w, http.StatusConflict, "storage_unavailable", "源文件不可用（存储离线）")
			return
		}
		slog.Error("stream direct", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}

func (s *Server) handleStreamCover(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	thumb := r.URL.Query().Get("thumb") == "1"
	if err := s.streaming.Cover(w, r, *v, thumb); err != nil {
		if errors.Is(err, streaming.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "封面不存在")
			return
		}
		slog.Error("stream cover", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}

func (s *Server) handleStreamSubtitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.streamOrError(w, r)
	if !ok {
		return
	}
	if err := s.streaming.Subtitle(w, r, *v); err != nil {
		if errors.Is(err, streaming.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "未找到字幕")
			return
		}
		slog.Error("stream subtitle", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}
