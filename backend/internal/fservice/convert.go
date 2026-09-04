package fservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"homereel/backend/internal/files"
	"homereel/backend/internal/jobs"
	"homereel/backend/internal/media"
)

// ConvertParams drives a format-factory conversion: which video codec to use
// (stream copy, h264, h265), the re-encode quality (CRF), how audio is handled,
// and whether the first subtitle is burned into the picture. Empty fields fall
// back to the fast-MP4 defaults (stream copy + smart audio).
type ConvertParams struct {
	Video string `json:"video"` // copy | h264 | h265; empty = copy
	VCRF  int    `json:"vcrf"`  // re-encode quality (0 = preset default)
	Audio string `json:"audio"` // smart | copy | aac; empty = smart
	AKbps int    `json:"akbps"` // AAC bitrate (0 = 192)
	Burn  bool   `json:"burn"`  // burn the first subtitle into the picture
}

// norm fills defaults and clamps out-of-range values so an arbitrary client
// payload can never produce a nonsensical ffmpeg invocation.
func (p ConvertParams) norm() ConvertParams {
	if p.Video != "h264" && p.Video != "h265" {
		p.Video = "copy"
	}
	if p.Audio != "copy" && p.Audio != "aac" {
		p.Audio = "smart"
	}
	if p.AKbps < 64 || p.AKbps > 512 {
		p.AKbps = 192
	}
	if p.VCRF < 0 || p.VCRF > 51 {
		p.VCRF = 0
	}
	if p.Video == "h264" && p.VCRF == 0 {
		p.VCRF = 19
	}
	if p.Video == "h265" && p.VCRF == 0 {
		p.VCRF = 23
	}
	return p
}

// convertMeta is the job payload of TypeConvert: the absolute source path to
// turn into a faststart MP4 copy (a file or a directory of videos) plus the
// conversion parameters chosen in the operations panel.
type convertMeta struct {
	Path   string        `json:"path"`
	Params ConvertParams `json:"params"`
}

// ConvertJob reports one enqueued conversion unit and its intended output.
// The actual output may still bump to a " (N)" suffix at run time when the
// name collides with an existing file.
type ConvertJob struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // file | dir
	JobID  string `json:"job_id"`
	Output string `json:"output"`
}

// EnqueueConvert validates each selected path and enqueues one convert job per
// unit (a video file → an mp4 copy next to it; a directory → a sibling " (MP4)"
// folder holding the converted copies of its direct-level video files).
// Per-item validation failures are returned separately so the caller can show
// exactly which selection could not be queued.
func (s *Service) EnqueueConvert(ctx context.Context, paths []string, params ConvertParams) ([]ConvertJob, []OpError) {
	var out []ConvertJob
	var errs []OpError
	for _, p := range paths {
		clean := filepath.Clean(p)
		info, err := os.Stat(clean)
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		} else if !files.IsConvertible(clean) {
			errs = append(errs, OpError{Path: p, Message: "不是可转换的媒体文件"})
			continue
		}
		extra, err := json.Marshal(convertMeta{Path: clean, Params: params})
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		id, err := s.jobs.Enqueue(ctx, jobs.TypeConvert, clean, "转换 · "+filepath.Base(clean), string(extra))
		if err != nil {
			errs = append(errs, OpError{Path: p, Message: err.Error()})
			continue
		}
		out = append(out, ConvertJob{
			Path:   clean,
			Kind:   kind,
			JobID:  id,
			Output: intendedOutput(clean, info.IsDir()),
		})
	}
	return out, errs
}

// HandleConvert runs one TypeConvert job. On success the produced mp4 paths are
// ingested so the library indexes them immediately (ADR-017) — a conversion is
// an import route, not something to wait for the next scan.
func (s *Service) HandleConvert(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	var meta convertMeta
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || meta.Path == "" {
		return errors.New("convert job missing path")
	}
	params := meta.Params.norm()
	info, err := os.Stat(meta.Path)
	if err != nil {
		return err
	}
	var outputs []string
	if info.IsDir() {
		outputs, err = s.convertDir(ctx, meta.Path, params, report)
	} else {
		outputs, err = s.convertFile(ctx, meta.Path, params, report)
	}
	if err != nil {
		return err
	}
	s.notifyIngest(ctx, outputs)
	slog.Info("convert done", "path", meta.Path, "outputs", len(outputs))
	return nil
}

// convertFile creates a faststart MP4 copy of one video next to the source,
// with the same base name and a " (N)" suffix if the target name is taken. It
// returns the produced output path.
func (s *Service) convertFile(ctx context.Context, src string, params ConvertParams, report jobs.Reporter) ([]string, error) {
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	dst := filepath.Join(filepath.Dir(src), freeName(filepath.Dir(src), stem, ".mp4"))
	report.Subtask("转换 " + filepath.Base(src))
	if err := s.runConvert(ctx, src, dst, params, func(f float64) { report.Progress(f) }); err != nil {
		return nil, err
	}
	return []string{dst}, nil
}

// convertDir creates a sibling " (MP4)" folder of the source directory and
// converts every direct-level video file inside it (non-recursive). Overall
// progress advances by file count; per-file failures are collected into one
// job error so a partially successful batch still shows in the task panel. It
// returns the produced output paths.
func (s *Service) convertDir(ctx context.Context, dir string, params ConvertParams, report jobs.Reporter) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var videos []string
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		full := filepath.Join(dir, de.Name())
		if files.IsConvertible(full) {
			videos = append(videos, full)
		}
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("目录下没有可转换的文件：%s", filepath.Base(dir))
	}
	outDir := filepath.Join(filepath.Dir(dir), freeName(filepath.Dir(dir), filepath.Base(dir), " (MP4)"))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var outputs []string
	var errs []string
	for i, v := range videos {
		if err := ctx.Err(); err != nil {
			return outputs, err
		}
		report.Subtask(fmt.Sprintf("转换 %s（%d/%d）", filepath.Base(v), i+1, len(videos)))
		base := float64(i) / float64(len(videos))
		dst := filepath.Join(outDir, freeName(outDir, strings.TrimSuffix(filepath.Base(v), filepath.Ext(v)), ".mp4"))
		if err := s.runConvert(ctx, v, dst, params, func(f float64) {
			report.Progress(base + f/float64(len(videos)))
		}); err != nil {
			errs = append(errs, filepath.Base(v)+": "+err.Error())
			continue
		}
		outputs = append(outputs, dst)
	}
	report.Progress(1)
	return outputs, collectErrors(errs)
}

// runConvert turns src into a browser-friendly faststart MP4 copy at dst,
// reporting the per-file completion fraction [0,1] through step. The whole
// conversion — stream probe, attempt ordering, progress parsing — lives in
// media.ConvertToMp4; params are normalized here before mapping into the media
// options.
func (s *Service) runConvert(ctx context.Context, src, dst string, params ConvertParams, step func(float64)) error {
	p := params.norm()
	return media.ConvertToMp4(ctx, s.media, media.ConvertOpts{
		Src:   src,
		Dst:   dst,
		Video: p.Video,
		VCRF:  p.VCRF,
		Audio: p.Audio,
		AKbps: p.AKbps,
		Burn:  p.Burn,
	}, step)
}

// freeName returns base+ext inside dir, bumping to "base (N)+ext" until a free
// name is found so a conversion never overwrites an existing file.
func freeName(dir, base, ext string) string {
	cand := base + ext
	for n := 1; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, cand)); os.IsNotExist(err) {
			return cand
		}
		cand = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
}

// intendedOutput mirrors freeName's first choice (before collision bumps) so
// the enqueue response can show where each copy is heading. When the source is
// already an .mp4 the copy is always bumped to " (1)" — never the source file.
func intendedOutput(path string, isDir bool) string {
	if isDir {
		return filepath.Join(filepath.Dir(path), filepath.Base(path)+" (MP4)")
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := filepath.Join(filepath.Dir(path), stem+".mp4")
	if out == path {
		out = filepath.Join(filepath.Dir(path), stem+" (1).mp4")
	}
	return out
}
