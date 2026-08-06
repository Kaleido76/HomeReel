package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"homereel/backend/internal/auth"
	"homereel/backend/internal/domain"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/search"
	"homereel/backend/internal/storage"
	"homereel/backend/internal/streaming"
)

const sessionCookie = "homereel_session"

// Server wires routes and shared dependencies.
type Server struct {
	auth      *auth.Service
	storages  *storage.Service
	files     *files.Service
	jobs      *jobs.Service
	scanner   *scanner.Service
	videos    domain.VideoRepo
	shows     domain.ShowRepo
	series    domain.SeriesRepo
	history   domain.HistoryRepo
	streaming *streaming.Service
	search    search.Provider
	bus       *events.Bus
	dataDir   string
}

// New builds the root handler for all /api routes. When staticDir is
// non-empty it also hosts the built frontend there (single-service layout):
// GET requests that match no /api route are served from disk, with
// extension-less paths falling back to index.html so SPA deep links survive a
// refresh.
func New(authSvc *auth.Service, storageSvc *storage.Service, filesSvc *files.Service,
	jobsSvc *jobs.Service, scannerSvc *scanner.Service, videosRepo domain.VideoRepo,
	showsRepo domain.ShowRepo, seriesRepo domain.SeriesRepo,
	historyRepo domain.HistoryRepo, streamingSvc *streaming.Service,
	searchProvider search.Provider, bus *events.Bus,
	dataDir string, staticDir string) http.Handler {
	s := &Server{
		auth: authSvc, storages: storageSvc, files: filesSvc,
		jobs: jobsSvc, scanner: scannerSvc,
		videos: videosRepo, shows: showsRepo, series: seriesRepo,
		history: historyRepo, streaming: streamingSvc,
		search: searchProvider, bus: bus, dataDir: dataDir,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/status", s.handleStatus)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/storages", s.requireAuth(http.HandlerFunc(s.handleStoragesList)))
	mux.Handle("POST /api/storages", s.requireAuth(http.HandlerFunc(s.handleStorageCreate)))
	mux.Handle("PATCH /api/storages/{id}", s.requireAuth(http.HandlerFunc(s.handleStoragePatch)))
	mux.Handle("DELETE /api/storages/{id}", s.requireAuth(http.HandlerFunc(s.handleStorageDelete)))
	mux.Handle("POST /api/storages/{id}/refresh", s.requireAuth(http.HandlerFunc(s.handleStorageRefresh)))
	mux.Handle("GET /api/fs/list", s.requireAuth(http.HandlerFunc(s.handleFsList)))
	mux.Handle("GET /api/fs/download", s.requireAuth(http.HandlerFunc(s.handleFsDownload)))
	mux.Handle("POST /api/fs/mkdir", s.requireAuth(http.HandlerFunc(s.handleFsMkdir)))
	mux.Handle("POST /api/fs/rename", s.requireAuth(http.HandlerFunc(s.handleFsRename)))
	mux.Handle("POST /api/fs/move", s.requireAuth(http.HandlerFunc(s.handleFsMove)))
	mux.Handle("POST /api/fs/delete", s.requireAuth(http.HandlerFunc(s.handleFsDelete)))
	mux.Handle("POST /api/fs/scan", s.requireAuth(http.HandlerFunc(s.handleFsScan)))
	mux.Handle("POST /api/upload", s.requireAuth(http.HandlerFunc(s.handleUpload)))
	mux.Handle("GET /api/jobs", s.requireAuth(http.HandlerFunc(s.handleJobsList)))
	mux.Handle("GET /api/videos", s.requireAuth(http.HandlerFunc(s.handleVideosList)))
	mux.Handle("GET /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoDetail)))
	mux.Handle("PATCH /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoPatch)))
	mux.Handle("DELETE /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoDelete)))
	mux.Handle("POST /api/videos/{id}/refresh", s.requireAuth(http.HandlerFunc(s.handleVideoRefresh)))
	mux.Handle("POST /api/videos/{id}/cover", s.requireAuth(http.HandlerFunc(s.handleVideoCover)))
	mux.Handle("GET /api/videos/{id}/history", s.requireAuth(http.HandlerFunc(s.handleHistoryGet)))
	mux.Handle("PUT /api/videos/{id}/history", s.requireAuth(http.HandlerFunc(s.handleHistoryPut)))
	mux.Handle("GET /api/shows", s.requireAuth(http.HandlerFunc(s.handleShowsList)))
	mux.Handle("GET /api/shows/{id}", s.requireAuth(http.HandlerFunc(s.handleShowDetail)))
	mux.Handle("PATCH /api/shows/{id}", s.requireAuth(http.HandlerFunc(s.handleShowPatch)))
	mux.Handle("GET /api/shows/{id}/seasons/{num}/episodes", s.requireAuth(http.HandlerFunc(s.handleShowSeasonsEpisodes)))
	mux.Handle("GET /api/shows/{id}/poster", s.requireAuth(http.HandlerFunc(s.handleShowPoster)))
	mux.Handle("GET /api/series", s.requireAuth(http.HandlerFunc(s.handleSeriesList)))
	mux.Handle("GET /api/series/{id}", s.requireAuth(http.HandlerFunc(s.handleSeriesDetail)))
	mux.Handle("GET /api/series/{id}/members", s.requireAuth(http.HandlerFunc(s.handleSeriesMembers)))
	mux.Handle("GET /api/series/{id}/links", s.requireAuth(http.HandlerFunc(s.handleSeriesLinks)))
	mux.Handle("POST /api/series/{id}/links", s.requireAuth(http.HandlerFunc(s.handleSeriesAddLink)))
	mux.Handle("DELETE /api/series/{id}/links/{linkedId}", s.requireAuth(http.HandlerFunc(s.handleSeriesRemoveLink)))
	mux.Handle("GET /api/series/{id}/poster", s.requireAuth(http.HandlerFunc(s.handleSeriesPoster)))
	mux.Handle("GET /api/tags", s.requireAuth(http.HandlerFunc(s.handleTags)))
	mux.Handle("GET /api/home", s.requireAuth(http.HandlerFunc(s.handleHome)))
	mux.Handle("GET /api/search", s.requireAuth(http.HandlerFunc(s.handleSearch)))
	mux.Handle("GET /api/remux/status", s.requireAuth(http.HandlerFunc(s.handleRemuxStatus)))
	mux.Handle("POST /api/fs/remux", s.requireAuth(http.HandlerFunc(s.handleFsRemux)))
	mux.Handle("POST /api/videos/{id}/remux", s.requireAuth(http.HandlerFunc(s.handleVideoRemux)))
	mux.Handle("GET /api/stream/{id}", s.requireAuth(http.HandlerFunc(s.handleStreamDirect)))
	mux.Handle("GET /api/stream/{id}/cover", s.requireAuth(http.HandlerFunc(s.handleStreamCover)))
	mux.Handle("GET /api/stream/{id}/hls/master.m3u8", s.requireAuth(http.HandlerFunc(s.handleStreamMaster)))
	mux.Handle("GET /api/stream/{id}/hls/{segment}", s.requireAuth(http.HandlerFunc(s.handleStreamSegment)))
	mux.Handle("GET /api/stream/{id}/subtitle", s.requireAuth(http.HandlerFunc(s.handleStreamSubtitle)))
	if staticDir != "" {
		mux.Handle("GET /", staticHandler(staticDir))
	}
	return s.withMiddleware(mux)
}

// staticHandler serves the built frontend. It only sees GET requests that
// matched no /api route, so unknown API paths must stay JSON.
func staticHandler(staticDir string) http.Handler {
	index := filepath.Join(staticDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not_found", "接口不存在")
			return
		}
		full := filepath.Join(staticDir, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			http.ServeFile(w, r, full)
			return
		}
		if filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": s.authenticated(r)})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	token, err := s.auth.Login(r.Context(), req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidPassword) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "口令错误")
			return
		}
		slog.Error("login failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.auth.SessionDays() * 86400,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

// handleMe is a protected placeholder proving the auth middleware returns 401
// when no valid session is present.
func (s *Server) handleMe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": "local"})
}

func (s *Server) handleStoragesList(w http.ResponseWriter, r *http.Request) {
	list, err := s.storages.List(r.Context())
	if err != nil {
		slog.Error("list storages", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	for i := range list {
		busy, _ := s.jobs.HasActive(r.Context(), jobs.TypeRescan, list[i].ID)
		list[i].Busy = busy
	}
	writeJSON(w, http.StatusOK, map[string]any{"storages": list})
}

func (s *Server) handleStorageCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		RootPath string `json:"root_path"`
		DeviceID string `json:"device_id"`
		Readonly bool   `json:"readonly"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	st, err := s.storages.Create(r.Context(), domain.Storage{
		Name:     req.Name,
		Type:     domain.StorageType(req.Type),
		RootPath: req.RootPath,
		DeviceID: req.DeviceID,
		Readonly: req.Readonly,
		Enabled:  req.Enabled,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_input", "参数不合法")
			return
		}
		slog.Error("create storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"storage": st})
}

func (s *Server) handleStoragePatch(w http.ResponseWriter, r *http.Request) {
	if !s.storageBusyOrError(w, r, r.PathValue("id")) {
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Type     *string `json:"type"`
		Readonly *bool   `json:"readonly"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	var patch storage.Patch
	if req.Name != nil {
		patch.Name = req.Name
	}
	if req.Type != nil {
		typ := domain.StorageType(*req.Type)
		patch.Type = &typ
	}
	patch.Readonly = req.Readonly
	patch.Enabled = req.Enabled

	st, err := s.storages.Update(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "存储卷不存在")
			return
		}
		if errors.Is(err, domain.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_input", "参数不合法")
			return
		}
		slog.Error("patch storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"storage": st})
}

func (s *Server) handleStorageDelete(w http.ResponseWriter, r *http.Request) {
	if !s.storageBusyOrError(w, r, r.PathValue("id")) {
		return
	}
	if err := s.storages.Delete(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "存储卷不存在")
			return
		}
		slog.Error("delete storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleStorageRefresh(w http.ResponseWriter, r *http.Request) {
	if !s.storageBusyOrError(w, r, r.PathValue("id")) {
		return
	}
	st, err := s.storages.Refresh(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "存储卷不存在")
			return
		}
		slog.Error("refresh storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if st.Available {
		if err := s.scanner.EnqueueRescan(r.Context(), st.ID); err != nil {
			slog.Warn("enqueue rescan after refresh", "storage_id", st.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"storage": st})
}

// handleFsList lists a directory inside a storage volume.
func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request) {
	st, err := s.storages.Get(r.Context(), r.URL.Query().Get("storageId"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "存储卷不存在")
			return
		}
		slog.Error("get storage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if !st.Available {
		writeError(w, http.StatusConflict, "storage_unavailable", "存储卷当前离线")
		return
	}
	rel := r.URL.Query().Get("path")
	entries, err := s.files.ListDir(st.RootPath, rel)
	if err != nil {
		if errors.Is(err, files.ErrOutsideRoot) {
			writeError(w, http.StatusBadRequest, "invalid_path", "路径越界")
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not_found", "目录不存在")
			return
		}
		slog.Error("list dir", "storage_id", st.ID, "path", rel, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	busy, _ := s.jobs.HasActive(r.Context(), jobs.TypeRescan, st.ID)
	st.Busy = busy
	writeJSON(w, http.StatusOK, map[string]any{"storage": st, "path": rel, "entries": entries})
}

func (s *Server) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	ok, err := s.auth.Valid(r.Context(), cookie.Value)
	return err == nil && ok
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "未登录或会话已过期")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return s.recoverer(s.logger(next))
}

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"range", r.Header.Get("Range"),
			"status", rec.status,
			"bytes", rec.bytes,
			"duration", time.Since(start).Round(time.Millisecond).String(),
		)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic", "err", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeBody decodes a JSON request body into v, writing a 400 response and
// returning false when the body is malformed.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return false
	}
	return true
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}
