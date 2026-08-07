package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/fservice"
)

// handleDisksList lists the host's local drives for the generic file browser.
func (s *Server) handleDisksList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"disks": s.fsvc.ListDisks(r.Context())})
}

// handleFs2List lists a directory by absolute path (read once, no indexing).
func (s *Server) handleFs2List(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	entries, err := s.fsvc.ListDir(r.Context(), path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			writeError(w, http.StatusNotFound, "not_found", "目录不存在")
		case errors.Is(err, os.ErrPermission):
			writeError(w, http.StatusForbidden, "forbidden", "没有访问权限")
		default:
			slog.Error("fs2 list", "path", path, "err", err)
			writeError(w, http.StatusBadRequest, "invalid_path", "无法读取该路径")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "entries": entries})
}

// handleFs2Copy enqueues a background copy job.
func (s *Server) handleFs2Copy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	jobID, err := s.fsvc.EnqueueCopy(r.Context(), req.Paths, req.Dest)
	if err != nil {
		fs2Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID})
}

// handleFs2Move enqueues a background move job.
func (s *Server) handleFs2Move(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	jobID, err := s.fsvc.EnqueueMove(r.Context(), req.Paths, req.Dest)
	if err != nil {
		fs2Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID})
}

// handleFs2Rename renames an entry within its directory.
func (s *Server) handleFs2Rename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.fsvc.Rename(r.Context(), req.Path, req.NewName); err != nil {
		fs2Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleFs2Delete permanently deletes the given paths.
func (s *Server) handleFs2Delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, s.fsvc.Delete(r.Context(), req.Paths))
}

// handleFs2PinsList returns the pinned favorite paths.
func (s *Server) handleFs2PinsList(w http.ResponseWriter, r *http.Request) {
	pins, err := s.fsvc.GetPins(r.Context())
	if err != nil {
		slog.Error("get pins", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins})
}

// handleFs2PinsAdd pins a path.
func (s *Server) handleFs2PinsAdd(w http.ResponseWriter, r *http.Request) {
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

// handleFs2PinsRemove unpins a path.
func (s *Server) handleFs2PinsRemove(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if err := s.fsvc.RemovePin(r.Context(), path); err != nil {
		slog.Error("remove pin", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// fs2Error maps a generic file-browser error to an HTTP response.
func fs2Error(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fservice.ErrInvalidName), errors.Is(err, domain.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_input", "参数不合法")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", "路径不存在")
	default:
		slog.Error("fs2 operation", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}
