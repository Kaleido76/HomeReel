package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	list, err := s.series.List(r.Context(), domain.SeriesQuery{
		Q:     r.URL.Query().Get("q"),
		Genre: r.URL.Query().Get("genre"),
		Year:  year,
		Tags:  r.URL.Query()["tag"],
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
// playability fallback (the frontend re-checks at runtime via canPlayType).
func (s *Server) fillMemberPlayable(members []domain.SeriesMember) {
	for i := range members {
		v := domain.Video{
			Codec:      members[i].Codec,
			AudioCodec: members[i].AudioCodec,
			Container:  members[i].Container,
			Segmented:  members[i].Segmented,
		}
		members[i].DirectPlayable = s.streaming.DirectPlayable(v)
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

func (s *Server) handleSeriesAddLink(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r, id); !ok {
		return
	}
	var body struct {
		SeriesID string `json:"series_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.SeriesID == "" || body.SeriesID == id {
		writeError(w, http.StatusBadRequest, "invalid_input", "无效的关联系列")
		return
	}
	if _, ok := s.seriesOrError(w, r, body.SeriesID); !ok {
		return
	}
	if err := s.series.AddLink(r.Context(), id, body.SeriesID, 0); err != nil {
		slog.Error("add series link", "series", id, "linked", body.SeriesID, "err", err)
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
