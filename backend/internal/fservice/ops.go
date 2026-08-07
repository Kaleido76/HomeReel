package fservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"homereel/backend/internal/domain"
	"homereel/backend/internal/jobs"
)

// fscopyMeta and fsmoveMeta are the job payloads of TypeFsCopy / TypeFsMove.
type fscopyMeta struct {
	Sources []string `json:"sources"`
	Dest    string   `json:"dest"`
}

// EnqueueCopy schedules a background copy of sources into the dest directory.
// Large transfers run as a long task so the HTTP request returns immediately
// and progress is visible in the task panel.
func (s *Service) EnqueueCopy(ctx context.Context, sources []string, dest string) (string, error) {
	return s.enqueueFsJob(ctx, jobs.TypeFsCopy, sources, dest, "复制到")
}

// EnqueueMove schedules a background move of sources into the dest directory.
func (s *Service) EnqueueMove(ctx context.Context, sources []string, dest string) (string, error) {
	return s.enqueueFsJob(ctx, jobs.TypeFsMove, sources, dest, "移动到")
}

func (s *Service) enqueueFsJob(ctx context.Context, typ string, sources []string, dest, verb string) (string, error) {
	if len(sources) == 0 || strings.TrimSpace(dest) == "" {
		return "", domain.ErrInvalid
	}
	extra, err := json.Marshal(fscopyMeta{Sources: sources, Dest: dest})
	if err != nil {
		return "", err
	}
	return s.jobs.Enqueue(ctx, typ, dest, verb+" "+filepath.Base(filepath.Clean(dest)), string(extra))
}

// HandleJob runs fscopy/fsmove jobs. Per-item failures are collected and
// reported as a job error at the end, so a partially-failed batch still
// surfaces in the task panel.
func (s *Service) HandleJob(ctx context.Context, j jobs.Job, report jobs.Reporter) error {
	var meta fscopyMeta
	if err := json.Unmarshal([]byte(j.Extra), &meta); err != nil || len(meta.Sources) == 0 || meta.Dest == "" {
		return errors.New("fs job missing sources/dest")
	}
	switch j.Type {
	case jobs.TypeFsCopy:
		return s.copyJob(ctx, meta.Sources, meta.Dest, report)
	case jobs.TypeFsMove:
		return s.moveJob(ctx, meta.Sources, meta.Dest, report)
	}
	return fmt.Errorf("unknown fs job type %q", j.Type)
}

// copyJob copies each source into dest, reporting byte-based progress. A source
// directory is copied recursively; the destination tree mirrors its relative
// layout under dest.
func (s *Service) copyJob(ctx context.Context, sources []string, dest string, report jobs.Reporter) error {
	total, err := totalBytes(sources)
	if err != nil {
		return err
	}
	var done int64
	step := func(n int64) {
		done += n
		if total > 0 {
			report.Progress(float64(done) / float64(total))
		}
	}
	var errs []string
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		report.Subtask("复制 " + filepath.Base(src))
		if err := copyTree(ctx, src, filepath.Join(dest, filepath.Base(src)), step); err != nil {
			errs = append(errs, src+": "+err.Error())
		}
	}
	return collectErrors(errs)
}

// moveJob moves each source into dest, preferring a same-volume rename and
// falling back to copy+delete across volumes (EXDEV). Progress is byte-based.
func (s *Service) moveJob(ctx context.Context, sources []string, dest string, report jobs.Reporter) error {
	total, err := totalBytes(sources)
	if err != nil {
		return err
	}
	var done int64
	step := func(n int64) {
		done += n
		if total > 0 {
			report.Progress(float64(done) / float64(total))
		}
	}
	var errs []string
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		report.Subtask("移动 " + filepath.Base(src))
		if err := moveTree(ctx, src, filepath.Join(dest, filepath.Base(src)), step); err != nil {
			errs = append(errs, src+": "+err.Error())
		}
	}
	return collectErrors(errs)
}

func collectErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errors.New(errs[0])
	}
	return fmt.Errorf("%d 项失败：%s", len(errs), strings.Join(errs, "；"))
}

// moveTree moves src to dst, copying and deleting when a cross-volume rename is
// not possible.
func moveTree(ctx context.Context, src, dst string, step func(int64)) error {
	if err := os.Rename(src, dst); err == nil {
		return stepSize(src, dst, step)
	}
	if err := copyTree(ctx, src, dst, step); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// stepSize counts a moved file's bytes towards overall progress. The size is
// read from the destination (or source) so cross-volume moves get accurate
// byte counts without a second walk.
func stepSize(src, dst string, step func(int64)) error {
	for _, p := range []string{dst, src} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			step(info.Size())
			return nil
		}
	}
	return nil
}

// copyTree recursively copies src (file or dir) to dst, counting copied bytes.
// File modification times are preserved. Symbolic links and directory junctions
// are skipped rather than followed, so a junction loop can never recurse
// forever (robocopy /XJ behaviour).
func copyTree(ctx context.Context, src, dst string, step func(int64)) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !info.IsDir() {
		if err := copyFile(src, dst, step); err != nil {
			return err
		}
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
		return nil
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, de := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyTree(ctx, filepath.Join(src, de.Name()), filepath.Join(dst, de.Name()), step); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, step func(int64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 256*1024)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			step(int64(n))
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// totalBytes sums the size of every file under the sources (directory sizes are
// walked recursively). A zero total leaves the job indeterminate.
func totalBytes(sources []string) (int64, error) {
	var total int64
	for _, src := range sources {
		err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				if info, ierr := d.Info(); ierr == nil {
					total += info.Size()
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
