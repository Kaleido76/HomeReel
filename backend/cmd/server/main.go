package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"homereel/backend/internal/api"
	"homereel/backend/internal/auth"
	"homereel/backend/internal/config"
	"homereel/backend/internal/db"
	"homereel/backend/internal/events"
	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/netutil"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/search"
	"homereel/backend/internal/storage"
	"homereel/backend/internal/store"
	"homereel/backend/internal/streaming"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}

	database, err := db.Open(cfg.Server.DataDir)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	if err := db.Migrate(database); err != nil {
		return err
	}

	authSvc := auth.New(database, cfg.Auth.SessionDays)
	generated, err := authSvc.EnsurePassword(context.Background(), cfg.Auth.Password)
	if err != nil {
		return err
	}
	if generated != "" {
		slog.Warn("未配置 auth.password，已生成随机访问口令（仅本次显示一次）", "password", generated)
	}

	storageSvc := storage.New(store.NewStorageRepo(database))
	filesSvc := files.NewService(filepath.Join(cfg.Server.DataDir, "uploads"))
	live := jobs.NewLiveStatus()
	jobsSvc := jobs.NewService(store.NewJobRepo(database), live)
	bus := events.New()
	videosRepo := store.NewVideoRepo(database)
	showsRepo := store.NewShowRepo(database)
	seriesRepo := store.NewSeriesRepo(database)
	historyRepo := store.NewHistoryRepo(database)
	streamingSvc := streaming.New(videosRepo, cfg.Server.DataDir,
		cfg.Media.FFmpegPath, cfg.Media.EnableHLS, cfg.Media.HLSPreset)
	scannerSvc := scanner.New(
		videosRepo,
		store.NewStorageRepo(database),
		showsRepo,
		seriesRepo,
		jobsSvc,
		filesSvc,
		bus,
		cfg.Media.FFprobePath,
		cfg.Media.FFmpegPath,
		cfg.Server.DataDir,
	)

	// VideoImported → enqueue thumbnail generation (ADR-010, ADR-012).
	go func() {
		for ev := range bus.Subscribe(events.VideoImported) {
			if id := ev.Data["video_id"]; id != "" {
				if err := scannerSvc.EnqueueThumbnail(context.Background(), id); err != nil {
					slog.Warn("enqueue thumbnail", "video_id", id, "err", err)
				}
			}
		}
	}()

	// VideoDeleted/VideoUpdated → drop stale HLS/remux caches (and cancel an
	// in-flight transcode) so a deleted or replaced file is never served stale.
	go func() {
		for ev := range bus.Subscribe(events.VideoDeleted, events.VideoUpdated) {
			if id := ev.Data["video_id"]; id != "" {
				streamingSvc.RemoveCache(id)
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Periodically purge orphaned upload chunks (interrupted uploads).
	go func() {
		for {
			if n, err := filesSvc.CleanupStaleUploads(24 * time.Hour); err != nil {
				slog.Warn("cleanup stale uploads", "err", err)
			} else if n > 0 {
				slog.Info("cleaned stale uploads", "dirs", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()

	// Background job worker (ADR-008). Job results are published on the bus so
	// any component can react to a long task finishing (e.g. the explorer
	// unlocks a volume the moment its scan lands).
	worker := jobs.NewWorker(store.NewJobRepo(database), scannerSvc.HandleJob, cfg.Media.ProbeConcurrency, live)
	worker.SetNotify(func(ctx context.Context, j jobs.Job, err error) {
		typ := events.JobDone
		data := map[string]string{"job_id": j.ID, "type": j.Type, "target": j.Target}
		if err != nil {
			typ = events.JobFailed
			data["error"] = err.Error()
		}
		bus.Publish(events.Event{Type: typ, Data: data})
	})
	go worker.Run(ctx)

	// Watch enabled storage volumes for changes.
	if list, err := storageSvc.List(context.Background()); err == nil {
		for _, st := range list {
			if !st.Enabled {
				continue
			}
			if err := scannerSvc.Watch(ctx, st); err != nil {
				slog.Warn("watch storage", "storage_id", st.ID, "err", err)
			}
		}
	} else {
		slog.Warn("list storages for watching", "err", err)
	}

	server := &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: api.New(authSvc, storageSvc, filesSvc, jobsSvc, scannerSvc,
			videosRepo, showsRepo, seriesRepo, historyRepo, streamingSvc,
			search.NewFTS5(database, videosRepo), bus, cfg.Server.DataDir,
			config.ResolveStaticDir(cfg.Server.StaticDir)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if staticDir := config.ResolveStaticDir(cfg.Server.StaticDir); staticDir != "" {
		slog.Info("serving frontend", "static_dir", staticDir)
	}

	// Bind explicitly so a port conflict fails startup immediately, and only
	// then announce the reachable URLs (config host is usually 0.0.0.0, so the
	// LAN IPs are resolved here instead of forcing the operator to look them up).
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", server.Addr, err)
	}
	slog.Info("HomeReel server listening", "addr", server.Addr, "urls", netutil.URLs(cfg.Server.Host, cfg.Server.Port))

	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
