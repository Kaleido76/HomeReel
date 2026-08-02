package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"videomesh/backend/internal/api"
	"videomesh/backend/internal/auth"
	"videomesh/backend/internal/config"
	"videomesh/backend/internal/db"
	"videomesh/backend/internal/events"
	"videomesh/backend/internal/files"
	"videomesh/backend/internal/jobs"
	"videomesh/backend/internal/scanner"
	"videomesh/backend/internal/storage"
	"videomesh/backend/internal/store"
	"videomesh/backend/internal/streaming"
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
	jobsSvc := jobs.NewService(store.NewJobRepo(database))
	bus := events.New()
	videosRepo := store.NewVideoRepo(database)
	historyRepo := store.NewHistoryRepo(database)
	streamingSvc := streaming.New(videosRepo, cfg.Server.DataDir,
		cfg.Media.FFmpegPath, cfg.Media.EnableHLS, cfg.Media.HLSPreset)
	scannerSvc := scanner.New(
		videosRepo,
		store.NewStorageRepo(database),
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

	// VideoDeleted → drop stale HLS cache (and cancel an in-flight transcode).
	go func() {
		for ev := range bus.Subscribe(events.VideoDeleted) {
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

	// Background job worker (ADR-008).
	worker := jobs.NewWorker(store.NewJobRepo(database), scannerSvc.HandleJob, cfg.Media.ProbeConcurrency)
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
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           api.New(authSvc, storageSvc, filesSvc, jobsSvc, scannerSvc, videosRepo, historyRepo, streamingSvc),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("VideoMesh server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
