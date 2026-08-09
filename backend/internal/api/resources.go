package api

import (
	"errors"
	"log/slog"
	"net/http"

	"homereel/backend/internal/domain"
)

// handleFilesResourcesMark enqueues mark_resource jobs for the given paths
// (文件页签「标记为单集/系列」). Discrete resources are not actively maintained:
// source-file disappearance is surfaced lazily by the video detail page, and
// release happens per-video there (DELETE /api/videos/:id only removes the
// binding, never the file).
func (s *Server) handleFilesResourcesMark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Kind  string   `json:"kind"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "没有需要标记的路径")
		return
	}
	jobIDs, err := s.fsvc.MarkResources(r.Context(), req.Paths, req.Kind)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalid):
			writeError(w, http.StatusBadRequest, "invalid_input", "kind 不合法（仅支持 series）")
		default:
			slog.Error("mark resources", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_ids": jobIDs})
}
