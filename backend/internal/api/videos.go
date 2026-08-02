package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"videomesh/backend/internal/domain"
	"videomesh/backend/internal/streaming"
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
	res, err := s.videos.List(r.Context(), domain.VideoQuery{
		Q:        r.URL.Query().Get("q"),
		Sort:     r.URL.Query().Get("sort"),
		Order:    r.URL.Query().Get("order"),
		Page:     page,
		PageSize: pageSize,
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
	writeJSON(w, http.StatusOK, map[string]any{
		"video":           v,
		"direct_playable": s.streaming.DirectPlayable(*v),
		"hls_enabled":     s.streaming.HLSEnabled(*v),
	})
}

// streamOrError resolves the video and checks its storage is online before
// any source-dependent streaming work.
func (s *Server) streamOrError(w http.ResponseWriter, r *http.Request) (*domain.Video, bool) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return nil, false
	}
	if _, ok := s.storageOrError(w, r, v.StorageID); !ok {
		return nil, false
	}
	return v, true
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

func (s *Server) handleStreamMaster(w http.ResponseWriter, r *http.Request) {
	v, ok := s.streamOrError(w, r)
	if !ok {
		return
	}
	if err := s.streaming.MasterM3U8(w, r, *v); err != nil {
		if errors.Is(err, streaming.ErrUnavailable) {
			writeError(w, http.StatusConflict, "storage_unavailable", "源文件不可用（存储离线）")
			return
		}
		if r.Context().Err() != nil {
			return
		}
		slog.Error("stream hls master", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}

func (s *Server) handleStreamSegment(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.streaming.Segment(w, r, *v, r.PathValue("segment")); err != nil {
		if errors.Is(err, streaming.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "分片不存在")
			return
		}
		slog.Error("stream hls segment", "video_id", v.ID, "err", err)
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
