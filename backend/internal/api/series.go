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
	links, err := s.series.GetLinks(r.Context(), id)
	if err != nil {
		slog.Error("get series links", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series":  series,
		"members": members,
		"links":   links,
	})
}

func (s *Server) handleSeriesMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.series.GetMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		slog.Error("get series members", "id", r.PathValue("id"), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
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
