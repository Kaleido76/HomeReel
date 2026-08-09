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
	"homereel/backend/internal/fservice"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/search"
	"homereel/backend/internal/streaming"
)

const sessionCookie = "homereel_session"

// Server wires routes and shared dependencies.
type Server struct {
	auth      *auth.Service
	fsvc      *fservice.Service
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
func New(authSvc *auth.Service, jobsSvc *jobs.Service, scannerSvc *scanner.Service, fsvc *fservice.Service,
	videosRepo domain.VideoRepo, showsRepo domain.ShowRepo, seriesRepo domain.SeriesRepo,
	historyRepo domain.HistoryRepo, streamingSvc *streaming.Service,
	searchProvider search.Provider, bus *events.Bus,
	dataDir string, staticDir string) http.Handler {
	s := &Server{
		auth: authSvc, fsvc: fsvc, jobs: jobsSvc, scanner: scannerSvc,
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
	mux.Handle("GET /api/disks", s.requireAuth(http.HandlerFunc(s.handleDisksList)))
	mux.Handle("GET /api/files/list", s.requireAuth(http.HandlerFunc(s.handleFilesList)))
	mux.Handle("POST /api/files/copy", s.requireAuth(http.HandlerFunc(s.handleFilesCopy)))
	mux.Handle("POST /api/files/move", s.requireAuth(http.HandlerFunc(s.handleFilesMove)))
	mux.Handle("POST /api/files/rename", s.requireAuth(http.HandlerFunc(s.handleFilesRename)))
	mux.Handle("POST /api/files/renames", s.requireAuth(http.HandlerFunc(s.handleFilesRenames)))
	mux.Handle("POST /api/files/delete", s.requireAuth(http.HandlerFunc(s.handleFilesDelete)))
	mux.Handle("GET /api/files/pins", s.requireAuth(http.HandlerFunc(s.handleFilesPinsList)))
	mux.Handle("POST /api/files/pins", s.requireAuth(http.HandlerFunc(s.handleFilesPinsAdd)))
	mux.Handle("DELETE /api/files/pins", s.requireAuth(http.HandlerFunc(s.handleFilesPinsRemove)))
	mux.Handle("GET /api/files/sources", s.requireAuth(http.HandlerFunc(s.handleFilesSourcesList)))
	mux.Handle("POST /api/files/sources", s.requireAuth(http.HandlerFunc(s.handleFilesSourcesAdd)))
	mux.Handle("DELETE /api/files/sources", s.requireAuth(http.HandlerFunc(s.handleFilesSourcesRemove)))
	mux.Handle("POST /api/files/sources/scan", s.requireAuth(http.HandlerFunc(s.handleFilesSourcesScan)))
	mux.Handle("POST /api/files/resources", s.requireAuth(http.HandlerFunc(s.handleFilesResourcesMark)))
	mux.Handle("GET /api/jobs", s.requireAuth(http.HandlerFunc(s.handleJobsList)))
	mux.Handle("GET /api/videos", s.requireAuth(http.HandlerFunc(s.handleVideosList)))
	mux.Handle("GET /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoDetail)))
	mux.Handle("PATCH /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoPatch)))
	mux.Handle("DELETE /api/videos/{id}", s.requireAuth(http.HandlerFunc(s.handleVideoDelete)))
	mux.Handle("POST /api/videos/{id}/refresh", s.requireAuth(http.HandlerFunc(s.handleVideoRefresh)))
	mux.Handle("POST /api/videos/{id}/sync", s.requireAuth(http.HandlerFunc(s.handleVideoSync)))
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
	mux.Handle("POST /api/series/{id}/sync", s.requireAuth(http.HandlerFunc(s.handleSeriesSync)))
	mux.Handle("GET /api/series/{id}/members", s.requireAuth(http.HandlerFunc(s.handleSeriesMembers)))
	mux.Handle("GET /api/series/{id}/links", s.requireAuth(http.HandlerFunc(s.handleSeriesLinks)))
	mux.Handle("POST /api/series/{id}/links", s.requireAuth(http.HandlerFunc(s.handleSeriesAddLink)))
	mux.Handle("DELETE /api/series/{id}/links/{linkedId}", s.requireAuth(http.HandlerFunc(s.handleSeriesRemoveLink)))
	mux.Handle("GET /api/series/{id}/poster", s.requireAuth(http.HandlerFunc(s.handleSeriesPoster)))
	mux.Handle("GET /api/tags", s.requireAuth(http.HandlerFunc(s.handleTags)))
	mux.Handle("GET /api/home", s.requireAuth(http.HandlerFunc(s.handleHome)))
	mux.Handle("GET /api/search", s.requireAuth(http.HandlerFunc(s.handleSearch)))
	mux.Handle("GET /api/remux/status", s.requireAuth(http.HandlerFunc(s.handleRemuxStatus)))
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
