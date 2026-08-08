package api

import (
	"log/slog"
	"net/http"
)

// handleVideoRemux schedules a remux job for a single video. The remux
// re-wraps a segmented MP4 into a standard faststart MP4 (stream copy), after
// which direct playback becomes fast and fully seekable.
func (s *Server) handleVideoRemux(w http.ResponseWriter, r *http.Request) {
	v, ok := s.videoOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.scanner.EnqueueRemux(r.Context(), v.ID); err != nil {
		slog.Error("enqueue remux", "video_id", v.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

// handleRemuxStatus lists every segmented video with its remux state so the
// management page can show what is pending / queued / done.
func (s *Server) handleRemuxStatus(w http.ResponseWriter, r *http.Request) {
	vs, err := s.videos.ListSegmented(r.Context())
	if err != nil {
		slog.Error("list segmented videos", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	items := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		items = append(items, map[string]any{
			"video_id":      v.ID,
			"title":         v.Title,
			"relative_path": v.RelativePath,
			"segmented":     true,
			"remuxed":       s.streaming.Remuxed(v.ID),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
