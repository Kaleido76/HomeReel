package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"videomesh/backend/internal/domain"
)

// timeLayout matches the fixed-width nanosecond timestamp used across the
// data layer (lexicographic order == chronological order).
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// handleHistoryGet returns the resume position for a video (null when absent).
func (s *Server) handleHistoryGet(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	h, err := s.history.Get(r.Context(), v.ID, "local")
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"history": nil})
			return
		}
		slog.Error("get history", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": h})
}

// handleHistoryPut saves a resume position (frontend throttles to ~10s).
func (s *Server) handleHistoryPut(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		Progress float64 `json:"progress"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Progress < 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "进度不能为负")
		return
	}
	h := domain.History{
		VideoID:   v.ID,
		User:      "local",
		Progress:  req.Progress,
		UpdatedAt: time.Now().UTC().Format(timeLayout),
	}
	if err := s.history.Upsert(r.Context(), h); err != nil {
		slog.Error("save history", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}
