package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/search"
)

// ---- Videos: PATCH / DELETE / refresh / cover ----

func (s *Server) handleVideoPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.videoOrError(w, r, id); !ok {
		return
	}
	var body struct {
		Title         *string   `json:"title"`
		Description   *string   `json:"description"`
		Kind          *string   `json:"kind"`
		Year          *int      `json:"year"`
		Rating        *float64  `json:"rating"`
		Genre         *string   `json:"genre"`
		Overview      *string   `json:"overview"`
		Studio        *string   `json:"studio"`
		CastText      *string   `json:"cast_text"`
		Tags          *[]string `json:"tags"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	patch := domain.VideoPatch{
		Title:        body.Title,
		Description:  body.Description,
		Kind:         body.Kind,
		Year:         body.Year,
		Rating:       body.Rating,
		Genre:        body.Genre,
		Overview:     body.Overview,
		Studio:       body.Studio,
		CastText:     body.CastText,
	}
	if err := s.videos.UpdateMetadata(r.Context(), id, patch); err != nil {
		slog.Error("update video metadata", "video_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if body.Tags != nil {
		if err := s.videos.SetTags(r.Context(), id, *body.Tags); err != nil {
			slog.Error("set tags", "video_id", id, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
			return
		}
	}
	v, err := s.videos.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"video": v})
}

func (s *Server) handleVideoDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.videoOrError(w, r, id); !ok {
		return
	}
	if err := s.videos.Delete(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "视频不存在")
			return
		}
		slog.Error("delete video", "video_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	s.bus.Publish(events.Event{Type: events.VideoDeleted, Data: map[string]string{"video_id": id}})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleVideoRefresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, ok := s.videoOrError(w, r, id)
	if !ok {
		return
	}
	if err := s.scanner.EnqueueProbe(r.Context(), id); err != nil {
		slog.Error("enqueue probe", "video_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if err := s.scanner.EnqueueThumbnail(r.Context(), id); err != nil {
		slog.Warn("enqueue thumbnail", "video_id", id, "err", err)
	}
	_ = v
	writeJSON(w, http.StatusOK, map[string]any{"queued": true})
}

func (s *Server) handleVideoCover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.videoOrError(w, r, id); !ok {
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的上传")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "缺少 file 字段")
		return
	}
	defer func() { _ = file.Close() }()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "仅支持 jpg/png/webp 图片")
		return
	}
	dir := filepath.Join(s.dataDir, "covers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	dest := filepath.Join(dir, id+ext)
	out, err := os.Create(dest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	_ = out.Close()
	if ext != ".jpg" {
		_ = os.Remove(filepath.Join(dir, id+".jpg"))
	}
	rel := filepath.ToSlash(filepath.Join("covers", id+ext))
	if err := s.videos.UpdateCovers(r.Context(), id, rel, ""); err != nil {
		slog.Error("update cover", "video_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cover_path": rel})
}

// ---- Shows ----

func (s *Server) handleShowsList(w http.ResponseWriter, r *http.Request) {
	list, err := s.shows.List(r.Context())
	if err != nil {
		slog.Error("list shows", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shows": list})
}

func (s *Server) handleShowDetail(w http.ResponseWriter, r *http.Request) {
	show, ok := s.showOrError(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	seasons, err := s.shows.GetSeasons(r.Context(), show.ID)
	if err != nil {
		slog.Error("get seasons", "show_id", show.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"show": show, "seasons": seasons})
}

func (s *Server) handleShowPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	show, ok := s.showOrError(w, r, id)
	if !ok {
		return
	}
	var body struct {
		Name         *string  `json:"name"`
		Overview     *string  `json:"overview"`
		Year         *int     `json:"year"`
		Rating       *float64 `json:"rating"`
		Genre        *string  `json:"genre"`
		PosterPath   *string  `json:"poster_path"`
		BackdropPath *string  `json:"backdrop_path"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Name != nil {
		show.Name = *body.Name
	}
	if body.Overview != nil {
		show.Overview = *body.Overview
	}
	if body.Year != nil {
		show.Year = *body.Year
	}
	if body.Rating != nil {
		show.Rating = *body.Rating
	}
	if body.Genre != nil {
		show.Genre = *body.Genre
	}
	if body.PosterPath != nil {
		show.PosterPath = *body.PosterPath
	}
	if body.BackdropPath != nil {
		show.BackdropPath = *body.BackdropPath
	}
	if err := s.shows.UpdateMetadata(r.Context(), *show); err != nil {
		slog.Error("update show", "show_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	updated, err := s.shows.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"show": updated})
}

func (s *Server) handleShowSeasonsEpisodes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.showOrError(w, r, id); !ok {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	episodes, err := s.shows.GetEpisodes(r.Context(), id, num)
	if err != nil {
		slog.Error("get episodes", "show_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": episodes})
}

func (s *Server) handleShowPoster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	show, err := s.shows.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "剧集不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if show.PosterPath != "" && s.serveDataFile(w, r, show.PosterPath) {
		return
	}
	// Fallback: first episode's generated thumb as a poster placeholder.
	eps, err := s.shows.GetEpisodes(r.Context(), id, 0)
	if err == nil {
		for _, ep := range eps {
			if ep.ThumbPath != "" && s.serveDataFile(w, r, ep.ThumbPath) {
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "海报不存在")
}

func (s *Server) showOrError(w http.ResponseWriter, r *http.Request, id string) (*domain.Show, bool) {
	show, err := s.shows.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "剧集不存在")
			return nil, false
		}
		slog.Error("get show", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return nil, false
	}
	return &show, true
}

// ---- Tags ----

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.videos.AllTags(r.Context())
	if err != nil {
		slog.Error("list tags", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// ---- Home & Search ----

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	continueWatching, err := s.videos.ContinueWatching(r.Context(), 12)
	if err != nil {
		slog.Error("continue watching", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	recent, err := s.videos.List(r.Context(), domain.VideoQuery{Sort: "date", Order: "desc", Page: 1, PageSize: 12})
	if err != nil {
		slog.Error("home recent", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"continue_watching": continueWatching,
		"recent":            recent.Videos,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if strings.TrimSpace(q) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []domain.Video{}})
		return
	}
	results, err := s.search.Search(r.Context(), q, search.Options{Limit: 50})
	if err != nil {
		slog.Error("search", "q", q, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": results})
}

// ---- helpers ----

// serveDataFile serves a file relative to data_dir, guarding against path
// traversal. It returns true when the file was served.
func (s *Server) serveDataFile(w http.ResponseWriter, r *http.Request, rel string) bool {
	if rel == "" {
		return false
	}
	full := filepath.Join(s.dataDir, filepath.FromSlash(rel))
	if !pathWithin(s.dataDir, full) {
		return false
	}
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		return false
	}
	http.ServeFile(w, r, full)
	return true
}

func pathWithin(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}
