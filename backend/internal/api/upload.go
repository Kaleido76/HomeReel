package api

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"videomesh/backend/internal/files"
)

// handleUpload accepts one chunk of a multipart upload per request
// (ADR-004 / 6.5). Form fields: uploadId, filename, chunkIndex, chunkTotal;
// the chunk bytes are the "file" part. The final chunk triggers assembly
// into the target storage directory.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	st, ok := s.storageOrError(w, r, r.URL.Query().Get("storageId"))
	if !ok {
		return
	}
	if st.Readonly {
		writeError(w, http.StatusForbidden, "readonly", "只读存储卷，拒绝写入")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的上传请求")
		return
	}
	uploadID := r.FormValue("uploadId")
	filename := r.FormValue("filename")
	chunkIndex, errIndex := strconv.Atoi(r.FormValue("chunkIndex"))
	chunkTotal, errTotal := strconv.Atoi(r.FormValue("chunkTotal"))
	if uploadID == "" || filename == "" || errIndex != nil || errTotal != nil ||
		chunkIndex < 0 || chunkTotal <= 0 || chunkIndex >= chunkTotal {
		writeError(w, http.StatusBadRequest, "invalid_input", "上传参数不合法")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "缺少文件分片")
		return
	}
	defer func() { _ = file.Close() }()

	if err := s.files.SaveChunk(uploadID, chunkIndex, file); err != nil {
		slog.Error("save chunk", "upload_id", uploadID, "index", chunkIndex, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}

	if chunkIndex == chunkTotal-1 {
		if err := s.files.CompleteUpload(uploadID, filename, st.RootPath,
			r.URL.Query().Get("path"), chunkTotal); err != nil {
			switch {
			case errors.Is(err, files.ErrInvalidName):
				writeError(w, http.StatusBadRequest, "invalid_input", "文件名不合法")
			case errors.Is(err, files.ErrMissingChunk):
				writeError(w, http.StatusBadRequest, "incomplete_upload", "分片不完整，请重试")
			default:
				slog.Error("complete upload", "upload_id", uploadID, "err", err)
				writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
			}
			return
		}
		destRel := r.URL.Query().Get("path")
		relPath := filepath.ToSlash(filepath.Join(destRel, filename))
		absPath := filepath.Join(st.RootPath, destRel, filename)
		if err := s.scanner.ImportUploaded(r.Context(), *st, absPath, relPath); err != nil {
			slog.Warn("import uploaded", "path", absPath, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": chunkIndex, "complete": chunkIndex == chunkTotal-1})
}
