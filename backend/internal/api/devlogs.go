package api

import (
	"errors"
	"log/slog"
	"net/http"

	"homereel/backend/internal/domain"
)

// DevTools 日志归档：前端「开发者工具」在移动端等无开发者工具的终端采集日志后，
// 提交为一份归档记录（POST），PC 端开发者工具可浏览、读取（含非 GUI 的 raw 文本
// 端点，便于开发时按 ID 快速抓取某次归档的日志）。

// handleDevLogCreate stores one submitted frontend log archive. The entries are
// kept verbatim so the archive reproduces exactly what the device captured.
func (s *Server) handleDevLogCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source  string               `json:"source"`
		Note    string               `json:"note"`
		Entries []domain.DevLogEntry `json:"entries"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	log := &domain.DevLog{Source: req.Source, Note: req.Note, Entries: req.Entries}
	if err := s.devlogs.Create(r.Context(), log); err != nil {
		slog.Error("create dev log", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "保存日志归档失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": log.ID})
}

// handleDevLogList returns the lightweight list of every archive (no entries).
func (s *Server) handleDevLogList(w http.ResponseWriter, r *http.Request) {
	list, err := s.devlogs.List(r.Context())
	if err != nil {
		slog.Error("list dev logs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "读取日志归档列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// handleDevLogGet returns one full archive including its entries.
func (s *Server) handleDevLogGet(w http.ResponseWriter, r *http.Request) {
	log, err := s.devlogs.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "日志归档不存在")
		return
	}
	if err != nil {
		slog.Error("get dev log", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "读取日志归档失败")
		return
	}
	writeJSON(w, http.StatusOK, log)
}

// handleDevLogRaw returns one archive as plain text, one log line per entry —
// the non-GUI way to grab a log for a given id (used during development).
func (s *Server) handleDevLogRaw(w http.ResponseWriter, r *http.Request) {
	log, err := s.devlogs.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "日志归档不存在")
		return
	}
	if err != nil {
		slog.Error("get dev log", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "读取日志归档失败")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, e := range log.Entries {
		_, _ = w.Write([]byte(e.Timestamp + " [" + e.Level + "] [" + e.Module + "] " + e.Message + "\n"))
	}
}

// handleDevLogDelete removes one archive.
func (s *Server) handleDevLogDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.devlogs.Delete(r.Context(), r.PathValue("id")); err != nil {
		slog.Error("delete dev log", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "删除日志归档失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
