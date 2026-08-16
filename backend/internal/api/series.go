package api

import (
	"errors"
	"log/slog"
	"net/http"

	"homereel/backend/internal/domain"
)

func (s *Server) seriesOrError(w http.ResponseWriter, r *http.Request, id string) (*domain.Series, bool) {
	series, err := s.series.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "系列不存在")
			return nil, false
		}
		slog.Error("get series", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return nil, false
	}
	return &series, true
}

func (s *Server) handleSeriesList(w http.ResponseWriter, r *http.Request) {
	list, err := s.series.List(r.Context(), domain.SeriesQuery{
		Q:    r.URL.Query().Get("q"),
		Tags: r.URL.Query()["tag"],
	})
	if err != nil {
		slog.Error("list series", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": list})
}

func (s *Server) handleSeriesDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	series, ok := s.seriesOrError(w, r, id)
	if !ok {
		return
	}
	members, err := s.series.GetMembers(r.Context(), id)
	if err != nil {
		slog.Error("get series members", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	s.fillMemberPlayable(members)
	links, err := s.series.GetLinks(r.Context(), id)
	if err != nil {
		slog.Error("get series links", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	// 系列详情按需对照磁盘：根目录是否存在、成员是否还在、根目录下是否有未
	// 入库的新文件。不一致时由前端显示警告并提供手动同步。
	check, checkErr := s.scanner.CheckSeries(r.Context(), id)
	if checkErr != nil {
		slog.Warn("check series", "id", id, "err", checkErr)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series":  series,
		"members": members,
		"links":   links,
		"check":   check,
	})
}

// handleSeriesSync re-syncs a series against its root folder (import new direct
// children, bind in file-name order, remove vanished members).
func (s *Server) handleSeriesSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	if err := s.scanner.SyncSeries(r.Context(), id); err != nil {
		slog.Error("sync series", "id", id, "err", err)
		writeError(w, http.StatusBadRequest, "sync_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": true})
}

// handleSeriesClearHistory clears the resume position of every member (系列详情
// 「清除全部观看进度」)。只删历史，不影响视频与文件。
func (s *Server) handleSeriesClearHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	if err := s.history.DeleteBySeries(r.Context(), id, "local"); err != nil {
		slog.Error("clear series history", "series_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

// handleSeriesReorder persists a manual member order (拖拽重排, ADR-015 修订):
// body { video_ids } must be a permutation of the series' current members; the
// order becomes episode_number 1..N and seasons.sort_manual=1 so later scans/
// syncs keep it.
func (s *Server) handleSeriesReorder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	var body struct {
		VideoIDs []string `json:"video_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	members, err := s.series.GetMembers(r.Context(), id)
	if err != nil {
		slog.Error("get series members for reorder", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if len(body.VideoIDs) != len(members) {
		writeError(w, http.StatusBadRequest, "invalid_input", "成员数量不匹配")
		return
	}
	seen := make(map[string]bool, len(members))
	for _, m := range members {
		seen[m.VideoID] = true
	}
	for _, vid := range body.VideoIDs {
		if !seen[vid] {
			writeError(w, http.StatusBadRequest, "invalid_input", "包含不属于本系列的成员")
			return
		}
	}
	if err := s.series.SetMemberOrder(r.Context(), id, body.VideoIDs); err != nil {
		slog.Error("set member order", "series_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSeriesResort restores the automatic file-name member order (系列详情
// 「按文件名字典序重新刷新排序」)：清除 sort_manual 并按文件名重绑成员 1..N，
// 手动编辑过的标题保留。无需同步磁盘。
func (s *Server) handleSeriesResort(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	if err := s.scanner.ResetSeriesSort(r.Context(), id); err != nil {
		slog.Error("reset series sort", "series_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSeriesMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.series.GetMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		slog.Error("get series members", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	s.fillMemberPlayable(members)
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// fillMemberPlayable annotates each member with the backend's conservative
// playability fallback and the remux/transcode dynamic-stream gates (the
// frontend re-checks direct playability at runtime via canPlayType).
func (s *Server) fillMemberPlayable(members []domain.SeriesMember) {
	for i := range members {
		v := domain.Video{
			Codec:      members[i].Codec,
			AudioCodec: members[i].AudioCodec,
			Container:  members[i].Container,
			Segmented:  members[i].Segmented,
			Duration:   members[i].Duration,
		}
		members[i].DirectPlayable = s.streaming.DirectPlayable(v)
		members[i].RemuxPlayable = s.streaming.RemuxPlayable(v)
		members[i].TranscodePlayable = s.streaming.TranscodePlayable(v)
	}
}

func (s *Server) handleSeriesLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.series.GetLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		slog.Error("get series links", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

// handleSeriesSetLinks replaces a series' link group with the given full
// desired set (方案 B)：series + every id end up in one group, mutually visible.
func (s *Server) handleSeriesSetLinks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	var body struct {
		SeriesIDs []string `json:"series_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	seen := map[string]bool{id: true}
	ids := []string{}
	for _, sid := range body.SeriesIDs {
		if sid == "" || seen[sid] {
			continue
		}
		seen[sid] = true
		if _, ok := s.seriesOrError(w, r, sid); !ok {
			return
		}
		ids = append(ids, sid)
	}
	if err := s.series.SetLinks(r.Context(), id, ids); err != nil {
		slog.Error("set series links", "series", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSeriesRemoveLink(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	linkedID := r.PathValue("linkedId")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	if err := s.series.RemoveLink(r.Context(), id, linkedID); err != nil {
		slog.Error("remove series link", "series", id, "linked", linkedID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSeriesPoster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	series, ok := s.seriesOrError(w, r, id)
	if !ok {
		return
	}
	if series.PosterPath != "" && s.serveDataFile(w, r, series.PosterPath) {
		return
	}
	members, err := s.series.GetMembers(r.Context(), id)
	if err == nil {
		for _, m := range members {
			if m.ThumbPath != "" && s.serveDataFile(w, r, m.ThumbPath) {
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "海报不存在")
}
