package streaming

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CacheClass aggregates one cache class (cover / thumb / subtitle / remux):
// totals and the subset that belongs to no indexed video (orphans). Cache
// classes are distinguished by their directory and file-name rule, so the
// owning video id can be recovered from each file name.
type CacheClass struct {
	Files       int64 `json:"files"`
	Bytes       int64 `json:"bytes"`
	Orphans     int64 `json:"orphans"`
	OrphanBytes int64 `json:"orphan_bytes"`
}

// subtitleIDRe splits an extracted-subtitle cache name "<id>-<track>".
var subtitleIDRe = regexp.MustCompile(`^(.*)-(\d+)$`)

// SubtitleCacheFile is one extracted-subtitle cache file with its owning video
// id and embedded stream index (Track is -1 for the legacy "<id>.vtt" name).
type SubtitleCacheFile struct {
	VideoID string
	Track   int
	Name    string
	Bytes   int64
}

// CacheOverview walks data_dir's cover/thumb/subtitle/remux caches and reports
// each class's totals and orphan counts (files whose owning video id is not
// indexed).
func (s *Service) CacheOverview(videoIDs map[string]struct{}) map[string]CacheClass {
	return map[string]CacheClass{
		"cover":    scanCache("cover", filepath.Join(s.dataDir, "covers"), videoIDs),
		"thumb":    scanCache("thumb", filepath.Join(s.dataDir, "thumbs"), videoIDs),
		"subtitle": scanCache("subtitle", s.subDir, videoIDs),
		"remux":    scanCache("remux", s.remuxDir, videoIDs),
	}
}

func scanCache(kind, dir string, videoIDs map[string]struct{}) CacheClass {
	var c CacheClass
	entries, err := os.ReadDir(dir)
	if err != nil {
		return c
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		c.Files++
		c.Bytes += info.Size()
		id, ok := cacheIDFromName(kind, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if !ok {
			c.Orphans++
			c.OrphanBytes += info.Size()
			continue
		}
		if _, indexed := videoIDs[id]; !indexed {
			c.Orphans++
			c.OrphanBytes += info.Size()
		}
	}
	return c
}

// ListSubtitleCache returns every extracted-subtitle cache file, parsed from the
// "<id>-<track>.vtt" / "<id>.vtt" names so the UI can group them per video.
func (s *Service) ListSubtitleCache() []SubtitleCacheFile {
	entries, err := os.ReadDir(s.subDir)
	if err != nil {
		return nil
	}
	var out []SubtitleCacheFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id, track := subtitleCacheID(e.Name())
		if id == "" {
			continue
		}
		out = append(out, SubtitleCacheFile{VideoID: id, Track: track, Name: e.Name(), Bytes: info.Size()})
	}
	return out
}

func subtitleCacheID(name string) (string, int) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if m := subtitleIDRe.FindStringSubmatch(base); m != nil {
		t, _ := strconv.Atoi(m[2])
		return m[1], t
	}
	if base != "" {
		return base, -1
	}
	return "", -1
}

// ClearAllSubtitles deletes every extracted-subtitle cache file (they are
// regenerated on next playback; the source file is untouched).
func (s *Service) ClearAllSubtitles() int {
	return removeDirFiles(s.subDir)
}

// RemuxCacheFile is one cached remux output (a per-audio-track MP4) with its
// owning video id. Fingerprint sidecars are excluded — they are metadata, not a
// remux the user manages.
type RemuxCacheFile struct {
	VideoID string
	Name    string
	Bytes   int64
}

// ListRemuxCache returns every cached remux MP4, parsed from the
// "<id>.mp4" / "<id>-a<N>.mp4" names so the UI can group them per video.
func (s *Service) ListRemuxCache() []RemuxCacheFile {
	entries, err := os.ReadDir(s.remuxDir)
	if err != nil {
		return nil
	}
	var out []RemuxCacheFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id, ok := remuxIDFromName(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if !ok {
			continue
		}
		out = append(out, RemuxCacheFile{VideoID: id, Name: e.Name(), Bytes: info.Size()})
	}
	return out
}

// ClearRemux deletes a video's cached remux MP4s (all audio tracks and their
// fingerprint sidecars) and returns how many files were removed.
func (s *Service) ClearRemux(videoID string) int {
	removed := 0
	for _, p := range []string{
		filepath.Join(s.remuxDir, videoID+".mp4"),
		filepath.Join(s.remuxDir, videoID+".mp4.meta"),
	} {
		if os.Remove(p) == nil {
			removed++
		}
	}
	matches, _ := filepath.Glob(filepath.Join(s.remuxDir, videoID+"-a*.mp4*"))
	for _, m := range matches {
		if os.Remove(m) == nil {
			removed++
		}
	}
	return removed
}

// ClearSubtitles deletes every extracted-subtitle cache file of one video.
func (s *Service) ClearSubtitles(videoID string) int {
	matches, _ := filepath.Glob(filepath.Join(s.subDir, videoID+"-*.vtt"))
	matches = append(matches, filepath.Join(s.subDir, videoID+".vtt"))
	removed := 0
	for _, m := range matches {
		if os.Remove(m) == nil {
			removed++
		}
	}
	return removed
}

// ClearSubtitleTrack deletes one extracted-subtitle cache file of a video
// (track -1 targets the legacy "<id>.vtt" file).
func (s *Service) ClearSubtitleTrack(videoID string, track int) int {
	if track < 0 {
		if os.Remove(filepath.Join(s.subDir, videoID+".vtt")) == nil {
			return 1
		}
		return 0
	}
	p := filepath.Join(s.subDir, fmt.Sprintf("%s-%d.vtt", videoID, track))
	if os.Remove(p) == nil {
		return 1
	}
	return 0
}

// ClearOrphans deletes every cached file whose owning video is not indexed
// (stale covers/thumbs/subtitles left over from removed or re-scanned videos).
func (s *Service) ClearOrphans(videoIDs map[string]struct{}) int {
	removed := 0
	for kind, dir := range s.cacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			id, ok := cacheIDFromName(kind, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
			if ok {
				if _, indexed := videoIDs[id]; indexed {
					continue
				}
			}
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				removed++
			}
		}
	}
	return removed
}

func (s *Service) cacheDirs() map[string]string {
	return map[string]string{
		"cover":    filepath.Join(s.dataDir, "covers"),
		"thumb":    filepath.Join(s.dataDir, "thumbs"),
		"subtitle": s.subDir,
		"remux":    s.remuxDir,
	}
}

func removeDirFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() && os.Remove(filepath.Join(dir, e.Name())) == nil {
			removed++
		}
	}
	return removed
}

// cacheIDFromName recovers the owning video id from a cached file base name:
// covers are "<id>", thumbs "<id>.thumb" (both without extension), extracted
// subtitles "<id>" or "<id>-<track>", remuxed MP4s "<id>" (the "<id>.mp4.meta"
// sidecar arrives as "<id>.mp4" once its extension is trimmed). Thumb files
// always carry the ".thumb" marker; anything else is not a valid cache file
// and treated as an orphan.
func cacheIDFromName(kind, base string) (string, bool) {
	switch kind {
	case "thumb":
		if strings.HasSuffix(base, ".thumb") {
			return strings.TrimSuffix(base, ".thumb"), true
		}
		return "", false
	case "subtitle":
		if m := subtitleIDRe.FindStringSubmatch(base); m != nil {
			return m[1], true
		}
		if base != "" {
			return base, true
		}
		return "", false
	case "remux":
		return remuxIDFromName(base)
	default:
		if base != "" {
			return base, true
		}
		return "", false
	}
}
