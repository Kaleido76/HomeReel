// Package logging centralizes backend log setup and HTTP access logging.
//
// Levels (config log.level): debug < info < warn < error, default info.
// Format (config log.format): text | json, default text.
// Output: stderr, plus an optional file (config log.file) that is date-rotated
// once at startup — a home service runs for days, so rotating on start is
// enough and no background rolling machinery is needed.
//
// Every package logs through the slog package-level functions; Setup installs
// the configured handler as slog.Default so a single call covers the whole
// binary, keeping call sites untouched.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the log section of config.yaml.
type Config struct {
	Level  string `yaml:"level"`  // debug | info | warn | error; empty = info
	Format string `yaml:"format"` // text | json; empty = text
	File   string `yaml:"file"`   // optional output file; empty = stderr only
}

// Default returns the built-in log configuration.
func Default() Config {
	return Config{Level: "info", Format: "text"}
}

// LevelName returns the effective level name (unknown/empty → "info"), useful
// for surfacing the active level at startup.
func (c Config) LevelName() string {
	return parseLevel(c.Level).String()
}

// Setup builds the slog handler from cfg, installs it as slog.Default and
// returns it together with a close func that releases the optional file
// handle (a no-op when cfg.File is empty).
func Setup(cfg Config) (*slog.Logger, func(), error) {
	writer := io.Writer(os.Stderr)
	closeLog := func() {}
	if cfg.File != "" {
		f, err := openLogFile(cfg.File)
		if err != nil {
			return nil, nil, err
		}
		closeLog = func() { _ = f.Close() }
		writer = io.MultiWriter(os.Stderr, f)
	}
	opts := &slog.HandlerOptions{
		Level:       parseLevel(cfg.Level),
		ReplaceAttr: replaceTime,
	}
	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "json") {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, closeLog, nil
}

// parseLevel maps a config level string to a slog level, defaulting to info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// replaceTime renders the record timestamp in a compact local format instead of
// slog's long RFC3339 with zone, which reads poorly in a console.
func replaceTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.String(slog.TimeKey, a.Value.Time().Format("2006-01-02 15:04:05.000"))
	}
	return a
}

// openLogFile opens path for append, first rotating an existing non-empty file
// to "<base>-<YYYYMMDD><ext>" so each service start begins a fresh log while
// the previous run stays greppable.
func openLogFile(path string) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		if err := os.Rename(path, rotatedName(path, time.Now())); err != nil {
			return nil, fmt.Errorf("rotate log %s: %w", path, err)
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// rotatedName derives "<base>-<date><ext>" for the one-time daily rotation.
func rotatedName(path string, t time.Time) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-" + t.Format("20060102") + ext
}