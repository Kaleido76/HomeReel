package api

import (
	"log/slog"
	"net/http"
)

// handleJobsList returns recent queue activity for progress display.
func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobs.List(r.Context(), 100)
	if err != nil {
		slog.Error("list jobs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list})
}

// handleFsScan schedules a full re-scan of a storage volume.
func (s *Server) handleFsScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageID string `json:"storageId"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if _, ok := s.storageOrError(w, r, req.StorageID); !ok {
		return
	}
	if err := s.scanner.EnqueueRescan(r.Context(), req.StorageID); err != nil {
		slog.Error("enqueue rescan", "storage_id", req.StorageID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}
