package scrape

import (
	"context"
	"errors"
	"strconv"

	"videomesh/backend/internal/domain"
)

// ErrNoTMDB reports that online scraping is disabled (no API key configured).
var ErrNoTMDB = errors.New("tmdb not configured")

// IsNoTMDB reports whether err is ErrNoTMDB (so handlers can return a friendly
// message instead of a 500).
func IsNoTMDB(err error) bool {
	return errors.Is(err, ErrNoTMDB)
}

// SearchMovie returns TMDB candidate matches for a video title.
func (s *Service) SearchMovie(ctx context.Context, title string, year int) ([]Candidate, error) {
	if s.tmdb == nil {
		return nil, ErrNoTMDB
	}
	return s.tmdb.search(ctx, title, "movie", year)
}

// ApplyMovieDetail fetches a specific TMDB movie and applies its metadata and
// images to the video.
func (s *Service) ApplyMovieDetail(ctx context.Context, videoID string, tmdbID int) error {
	if s.tmdb == nil {
		return ErrNoTMDB
	}
	d, err := s.tmdb.detail(ctx, strconv.Itoa(tmdbID), "movie")
	if err != nil {
		return err
	}
	v, err := s.videos.Get(ctx, videoID)
	if err != nil {
		return err
	}
	title := detailTitle(d)
	patch := domain.VideoPatch{
		Title:          &title,
		Overview:       &d.Overview,
		Genre:          optStr(detailGenre(d)),
		CastText:       optStr(detailCast(d)),
		MetadataSource: src("tmdb"),
	}
	if y := detailYear(d); y > 0 {
		patch.Year = &y
	}
	if d.VoteAverage > 0 {
		r := d.VoteAverage
		patch.Rating = &r
	}
	poster, err := s.tmdb.saveImage(ctx, imageURL(s.tmdb.imgBase, d.PosterPath), "covers", v.ID+".jpg")
	if err != nil {
		return err
	}
	backdrop, err := s.tmdb.saveImage(ctx, imageURL(s.tmdb.imgBase, d.BackdropPath), "backdrops", v.ID+".jpg")
	if err != nil {
		return err
	}
	if backdrop != "" {
		patch.BackdropPath = &backdrop
	}
	if err := s.videos.UpdateMetadata(ctx, v.ID, patch); err != nil {
		return err
	}
	if poster != "" {
		return s.videos.UpdateCovers(ctx, v.ID, poster, "")
	}
	return nil
}

// SearchTV returns TMDB candidate matches for a show name.
func (s *Service) SearchTV(ctx context.Context, name string) ([]Candidate, error) {
	if s.tmdb == nil {
		return nil, ErrNoTMDB
	}
	return s.tmdb.search(ctx, name, "tv", 0)
}

// ApplyTVDetail fetches a specific TMDB TV show and applies its metadata and
// images to the show.
func (s *Service) ApplyTVDetail(ctx context.Context, showID string, tmdbID int) error {
	if s.tmdb == nil {
		return ErrNoTMDB
	}
	d, err := s.tmdb.detail(ctx, strconv.Itoa(tmdbID), "tv")
	if err != nil {
		return err
	}
	show, err := s.shows.Get(ctx, showID)
	if err != nil {
		return err
	}
	if name := detailTitle(d); name != "" {
		show.Name = name
	}
	show.Overview = d.Overview
	show.Genre = detailGenre(d)
	show.MetadataSource = "tmdb"
	if y := detailYear(d); y > 0 {
		show.Year = y
	}
	if d.VoteAverage > 0 {
		show.Rating = d.VoteAverage
	}
	poster, err := s.tmdb.saveImage(ctx, imageURL(s.tmdb.imgBase, d.PosterPath), "posters", show.ID+".jpg")
	if err != nil {
		return err
	}
	backdrop, err := s.tmdb.saveImage(ctx, imageURL(s.tmdb.imgBase, d.BackdropPath), "backdrops", show.ID+".jpg")
	if err != nil {
		return err
	}
	if poster != "" {
		show.PosterPath = poster
	}
	if backdrop != "" {
		show.BackdropPath = backdrop
	}
	return s.shows.UpdateMetadata(ctx, show)
}

func src(s string) *string { return &s }

// optStr returns a pointer to s, or nil when empty (so the column is not
// overwritten during an update).
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
