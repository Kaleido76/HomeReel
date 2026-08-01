package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"videomesh/backend/internal/domain"
	"videomesh/backend/internal/files"
)

// storageOrError loads a storage volume, writing an error response and
// returning ok=false when it is missing or offline.
func (s *Server) storageOrError(w http.ResponseWriter, r *http.Request, id string) (*domain.Storage, bool) {
	st, err := s.storages.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "存储卷不存在")
			return nil, false
		}
		slog.Error("get storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return nil, false
	}
	if !st.Available {
		writeError(w, http.StatusConflict, "storage_unavailable", "存储卷当前离线")
		return nil, false
	}
	return &st, true
}

// fsError maps a filesystem operation error to an HTTP response.
func fsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, files.ErrOutsideRoot), errors.Is(err, files.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "invalid_path", "路径或名称不合法")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", "文件或目录不存在")
	default:
		slog.Error("filesystem operation", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
	}
}

func (s *Server) handleFsMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageID string `json:"storageId"`
		Path      string `json:"path"`
		Name      string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	st, ok := s.storageOrError(w, r, req.StorageID)
	if !ok {
		return
	}
	if st.Readonly {
		writeError(w, http.StatusForbidden, "readonly", "只读存储卷，拒绝写入")
		return
	}
	if err := s.files.Mkdir(st.RootPath, req.Path, req.Name); err != nil {
		fsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFsRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageID string `json:"storageId"`
		Path      string `json:"path"`
		NewName   string `json:"newName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	st, ok := s.storageOrError(w, r, req.StorageID)
	if !ok {
		return
	}
	if st.Readonly {
		writeError(w, http.StatusForbidden, "readonly", "只读存储卷，拒绝写入")
		return
	}
	if err := s.files.Rename(st.RootPath, req.Path, req.NewName); err != nil {
		fsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFsMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageID string   `json:"storageId"`
		Paths     []string `json:"paths"`
		Dest      string   `json:"dest"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	st, ok := s.storageOrError(w, r, req.StorageID)
	if !ok {
		return
	}
	if st.Readonly {
		writeError(w, http.StatusForbidden, "readonly", "只读存储卷，拒绝写入")
		return
	}
	writeJSON(w, http.StatusOK, s.files.Move(st.RootPath, req.Paths, req.Dest))
}

func (s *Server) handleFsDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageID string   `json:"storageId"`
		Paths     []string `json:"paths"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	st, ok := s.storageOrError(w, r, req.StorageID)
	if !ok {
		return
	}
	if st.Readonly {
		writeError(w, http.StatusForbidden, "readonly", "只读存储卷，拒绝写入")
		return
	}
	writeJSON(w, http.StatusOK, s.files.Delete(st.RootPath, req.Paths))
}

func (s *Server) handleFsDownload(w http.ResponseWriter, r *http.Request) {
	st, ok := s.storageOrError(w, r, r.URL.Query().Get("storageId"))
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	file, err := s.files.OpenFile(st.RootPath, rel)
	if err != nil {
		fsError(w, err)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		fsError(w, err)
		return
	}
	http.ServeContent(w, r, filepath.Base(rel), info.ModTime(), file)
}
