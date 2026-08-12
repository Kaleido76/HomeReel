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
	"syscall"
	"time"

	"homereel/backend/internal/api"
	"homereel/backend/internal/auth"
	"homereel/backend/internal/config"
	"homereel/backend/internal/db"
	"homereel/backend/internal/events"
	"homereel/backend/internal/fservice"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/netutil"
	"homereel/backend/internal/scanner"
	"homereel/backend/internal/search"
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

	live := jobs.NewLiveStatus()
	jobsSvc := jobs.NewService(store.NewJobRepo(database), live)
	bus := events.New()
	videosRepo := store.NewVideoRepo(database)
	sourcesRepo := store.NewSourceRepo(database)
	showsRepo := store.NewShowRepo(database)
	seriesRepo := store.NewSeriesRepo(database)
	historyRepo := store.NewHistoryRepo(database)
	streamingSvc := streaming.New(videosRepo, cfg.Server.DataDir, cfg.Media.FFmpegPath, cfg.Media.FFprobePath)
	scannerSvc := scanner.New(
		videosRepo,
		sourcesRepo,
		showsRepo,
		seriesRepo,
		jobsSvc,
		bus,
		cfg.Media.FFprobePath,
		cfg.Media.FFmpegPath,
		cfg.Server.DataDir,
	)
	// Generic machine-wide file browser (文件 tab): absolute-path listing,
	// clipboard-style copy/move behind its own background jobs, format-factory
	// conversions, and the multimedia-source + manual-resource markers that
	// feed the video library.
	fsvc := fservice.New(jobsSvc, store.NewSettingsRepo(database), sourcesRepo,
		cfg.Media.FFmpegPath, cfg.Media.FFprobePath)
	// Every file operation in the browser feeds the unified library pipeline
	// (ADR-017): after a copy/move/rename/delete/convert, the scanner ingests
	// or evicts the affected paths so the library is never left stale.
	fsvc.SetLibraryNotifier(scannerSvc.IngestPaths, scannerSvc.EvictPaths)

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

	// VideoDeleted → drop the generated cover/thumb/subtitle files so a deleted
	// video is never served stale. VideoUpdated → only the extracted-subtitle
	// cache: the scanner regenerates covers whenever it re-probes (processInline)
	// and a pure rename/move leaves them valid (ADR-017).
	go func() {
		for ev := range bus.Subscribe(events.VideoDeleted) {
			if id := ev.Data["video_id"]; id != "" {
				streamingSvc.RemoveCache(id)
			}
		}
	}()
	go func() {
		for ev := range bus.Subscribe(events.VideoUpdated) {
			if id := ev.Data["video_id"]; id != "" {
				streamingSvc.RemoveSubtitles(id)
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background job worker (ADR-008). Job results are published on the bus so
	// any component can react to a long task finishing. Generic file-browser
	// jobs (fscopy/fsmove/convert) are dispatched to the fservice handler,
	// everything else (probe/thumbnail/scan_source) to the scanner.
	worker := jobs.NewWorker(store.NewJobRepo(database), func(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
		if j.Type == jobs.TypeFsCopy || j.Type == jobs.TypeFsMove || j.Type == jobs.TypeConvert {
			return fsvc.HandleJob(ctx, j, report)
		}
		return scannerSvc.HandleJob(ctx, j, report)
	}, cfg.Media.ProbeConcurrency, live)
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

	server := &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: api.New(authSvc, jobsSvc, scannerSvc, fsvc,
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
