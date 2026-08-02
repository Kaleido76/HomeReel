package scrape

import (
	"context"
	"os"
	"path/filepath"

	"videomesh/backend/internal/domain"
)

// TMDBConfig enables optional online scraping (ADR-016). An empty APIKey
// leaves online scraping disabled (NFO + manual editing still work).
type TMDBConfig struct {
	APIKey   string
	Language string
}

// Service orchestrates metadata scraping (ADR-016): local NFO files are
// applied automatically on import; TMDB scraping is triggered by the user.
type Service struct {
	videos domain.VideoRepo
	shows  domain.ShowRepo
	tmdb   *tmdbClient
}

// New builds a scrape service. dataDir hosts downloaded poster/backdrop files.
func New(videos domain.VideoRepo, shows domain.ShowRepo, dataDir string, tmdb TMDBConfig) *Service {
	return &Service{
		videos: videos,
		shows:  shows,
		tmdb:   newTMDBClient(tmdb, dataDir),
	}
}

// ApplyNFOForVideo reads the .nfo file beside a video and applies its
// metadata; for episodes it also applies the show-level tvshow.nfo.
func (s *Service) ApplyNFOForVideo(ctx context.Context, v domain.Video) error {
	meta, err := ParseNFO(videoNFOFile(v.Path))
	if err != nil || (meta.Title == "" && meta.Year == 0 && meta.Overview == "" &&
		meta.Genre == "" && meta.Studio == "" && meta.CastText == "") {
		return err
	}
	patch := nfoToVideoPatch(meta)
	source := "nfo"
	patch.MetadataSource = &source
	if err := s.videos.UpdateMetadata(ctx, v.ID, patch); err != nil {
		return err
	}
	if v.Kind == "episode" {
		return s.applyShowNFO(ctx, v)
	}
	return nil
}

// applyShowNFO applies the <show dir>/tvshow.nfo to the show a video belongs
// to, so a whole series gets its metadata from one file.
func (s *Service) applyShowNFO(ctx context.Context, v domain.Video) error {
	if v.ShowID == "" {
		return nil
	}
	nfoPath := findTVShowNFO(filepath.Dir(v.Path))
	if nfoPath == "" {
		return nil
	}
	meta, err := ParseNFO(nfoPath)
	if err != nil {
		return err
	}
	show, err := s.shows.Get(ctx, v.ShowID)
	if err != nil {
		return err
	}
	updated := false
	if meta.Overview != "" && show.Overview != meta.Overview {
		show.Overview = meta.Overview
		updated = true
	}
	if meta.Year > 0 && show.Year != meta.Year {
		show.Year = meta.Year
		updated = true
	}
	if meta.Rating > 0 && show.Rating != meta.Rating {
		show.Rating = meta.Rating
		updated = true
	}
	if meta.Genre != "" && show.Genre != meta.Genre {
		show.Genre = meta.Genre
		updated = true
	}
	if updated {
		show.MetadataSource = "nfo"
		return s.shows.UpdateMetadata(ctx, show)
	}
	return nil
}

// videoNFOFile is the sidecar NFO for a video: <dir>/<name>.nfo. The Kodi
// convention names it after the video file (Movie.mkv → Movie.nfo).
func videoNFOFile(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return filepath.Join(dir, base[:len(base)-len(ext)]+".nfo")
}

// findTVShowNFO walks up from dir looking for a tvshow.nfo file.
func findTVShowNFO(dir string) string {
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, "tvshow.nfo")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func nfoToVideoPatch(m Metadata) domain.VideoPatch {
	p := domain.VideoPatch{}
	if m.Title != "" {
		p.Title = &m.Title
	}
	if m.Year > 0 {
		p.Year = &m.Year
	}
	if m.Rating > 0 {
		p.Rating = &m.Rating
	}
	if m.Overview != "" {
		p.Overview = &m.Overview
	}
	if m.Genre != "" {
		p.Genre = &m.Genre
	}
	if m.Studio != "" {
		p.Studio = &m.Studio
	}
	if m.CastText != "" {
		p.CastText = &m.CastText
	}
	return p
}
