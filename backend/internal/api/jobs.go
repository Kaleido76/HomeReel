package api

import (
	"log/slog"
	"net/http"
	"strconv"
)

// handleJobsList returns recent queue activity for the task panel. Internal
// jobs (probe/thumbnail) are included so the panel can filter, but the UI
// generally only shows user-facing long tasks.
func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	list, err := s.jobs.List(r.Context(), limit)
	if err != nil {
		slog.Error("list jobs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	s.jobs.AttachLive(list)
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list})
}

// handleFsScan schedules a full re-scan of a storage volume. A volume that
// already has a queued/running rescan refuses a second submission.
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
	if !s.storageBusyOrError(w, r, req.StorageID) {
		return
	}
	if err := s.scanner.EnqueueRescan(r.Context(), req.StorageID); err != nil {
		slog.Error("enqueue rescan", "storage_id", req.StorageID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}
