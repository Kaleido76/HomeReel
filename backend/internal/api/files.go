package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/fservice"
	"homereel/backend/internal/jobs"
)

// handleDisksList lists the host's local drives for the generic file browser.
func (s *Server) handleDisksList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"disks": s.fsvc.ListDisks(r.Context())})
}

// handleFilesList lists a directory by absolute path (read once, no indexing).
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	entries, err := s.fsvc.ListDir(r.Context(), path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			writeError(w, http.StatusNotFound, "not_found", "目录不存在")
		case errors.Is(err, os.ErrPermission):
			writeError(w, http.StatusForbidden, "forbidden", "没有访问权限")
		default:
			slog.Error("files list", "path", path, "err", err)
			writeError(w, http.StatusBadRequest, "invalid_path", "无法读取该路径")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "entries": entries})
}

// handleFilesCopy enqueues a background copy job.
func (s *Server) handleFilesCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	jobID, err := s.fsvc.EnqueueCopy(r.Context(), req.Paths, req.Dest)
	if err != nil {
		filesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID})
}

// handleFilesMove enqueues a background move job.
func (s *Server) handleFilesMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	jobID, err := s.fsvc.EnqueueMove(r.Context(), req.Paths, req.Dest)
	if err != nil {
		filesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID})
}

// handleFilesRename renames an entry within its directory.
func (s *Server) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.fsvc.Rename(r.Context(), req.Path, req.NewName); err != nil {
		filesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleFilesRenames renames multiple entries in place (batch). It returns the
// same OpResult shape as delete so partial failures surface per-item.
func (s *Server) handleFilesRenames(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Renames []fservice.Rename `json:"renames"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Renames) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "没有需要重命名的项")
		return
	}
	writeJSON(w, http.StatusOK, s.fsvc.RenameMany(r.Context(), req.Renames))
}

// handleFilesDelete permanently deletes the given paths. Library rows whose
// source file was just deleted are evicted right away (the fservice Delete
// reports the removed paths to the unified evict pipeline, ADR-017), so the
// file browser and the video library stay consistent.
func (s *Server) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, s.fsvc.Delete(r.Context(), req.Paths))
}

// handleFilesPinsList returns the pinned favorite paths.
func (s *Server) handleFilesPinsList(w http.ResponseWriter, r *http.Request) {
	pins, err := s.fsvc.GetPins(r.Context())
	if err != nil {
		slog.Error("get pins", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins})
}

// handleFilesPinsAdd pins a path.
func (s *Server) handleFilesPinsAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.fsvc.AddPin(r.Context(), req.Path); err != nil {
		if errors.Is(err, domain.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_input", "路径不合法")
			return
		}
		slog.Error("add pin", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleFilesPinsRemove unpins a path.
func (s *Server) handleFilesPinsRemove(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if err := s.fsvc.RemovePin(r.Context(), path); err != nil {
		slog.Error("remove pin", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// filesError maps a generic file-browser error to an HTTP response.
func filesError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fservice.ErrInvalidName), errors.Is(err, domain.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_input", "参数不合法")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", "路径不存在")
	default:
		slog.Error("files operation", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}

// handleFilesSourcesList lists multimedia source markers, each annotated with
// whether its root is currently reachable (a source whose root is gone is
// temporarily offline — its library rows are left untouched) and whether a scan
// is queued/running for it.
func (s *Server) handleFilesSourcesList(w http.ResponseWriter, r *http.Request) {
	list, err := s.fsvc.ListSources(r.Context())
	if err != nil {
		slog.Error("list sources", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, src := range list {
		scanning, _ := s.jobs.HasActive(r.Context(), jobs.TypeScanSource, src.ID)
		_, statErr := os.Stat(src.Path)
		out = append(out, map[string]any{
			"id":           src.ID,
			"path":         src.Path,
			"created_at":   src.CreatedAt,
			"last_scan_at": src.LastScanAt,
			"available":    statErr == nil,
			"scanning":     scanning,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out})
}

// handleFilesSourcesAdd marks a directory as a multimedia source and enqueues its
// first full scan.
func (s *Server) handleFilesSourcesAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	src, err := s.fsvc.AddSource(r.Context(), req.Path)
	if err != nil {
		switch {
		case errors.Is(err, fservice.ErrNestedSource):
			writeError(w, http.StatusBadRequest, "nested_source", err.Error())
		case errors.Is(err, domain.ErrInvalid):
			writeError(w, http.StatusBadRequest, "invalid_input", "路径不合法或不是目录")
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "目录不存在")
		default:
			slog.Error("add source", "path", req.Path, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		}
		return
	}
	jobID := ""
	if id, err := s.scanner.EnqueueScanSource(r.Context(), src.ID); err != nil {
		slog.Warn("enqueue scan source", "source_id", src.ID, "err", err)
	} else {
		jobID = id
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": src, "job_id": jobID})
}

// handleFilesSourcesRemove removes a source marker and deletes every video it
// owned from the library (the whole subtree's single episodes and series
// disappear with it — VideoDeleted events clear the caches). Nested child
// sources keep their own rows.
func (s *Server) handleFilesSourcesRemove(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	src, err := s.fsvc.SourceByPath(r.Context(), path)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "该目录不是多媒体源")
			return
		}
		slog.Error("get source", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	ids, delErr := s.videos.DeleteBySource(r.Context(), src.ID)
	if delErr != nil {
		slog.Error("delete source videos", "source_id", src.ID, "err", delErr)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if err := s.fsvc.RemoveSource(r.Context(), path); err != nil {
		slog.Error("remove source", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	for _, id := range ids {
		s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": id}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true, "videos_removed": len(ids)})
}

// handleFilesSourcesScan enqueues a manual re-scan of a multimedia source.
func (s *Server) handleFilesSourcesScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	src, err := s.fsvc.SourceByPath(r.Context(), req.Path)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "该目录不是多媒体源")
			return
		}
		slog.Error("get source", "path", req.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	jobID, err := s.scanner.EnqueueScanSource(r.Context(), src.ID)
	if err != nil {
		slog.Error("enqueue scan source", "source_id", src.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}
