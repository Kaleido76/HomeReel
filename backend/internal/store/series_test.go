package store

import (
	"context"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/domain"
)

func newSeriesTestRepo(t *testing.T) (domain.SeriesRepo, domain.VideoRepo, domain.ShowRepo, domain.SourceRepo) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewSeriesRepo(database), NewVideoRepo(database), NewShowRepo(database), NewSourceRepo(database)
}

func TestSeriesListFilter(t *testing.T) {
	series, videos, shows, sources := newSeriesTestRepo(t)
	ctx := context.Background()
	seedSource(t, sources, "s1", `C:\x`)

	// Each series is a show+season bound to its own root path; two roots with
	// the same name stay two independent series.
	mk := func(id, name, root, tag, overview string, genre string, year int) domain.Series {
		s, err := series.CreateAtRoot(ctx, name, root)
		if err != nil {
			t.Fatalf("create series: %v", err)
		}
		if err := shows.UpdateMetadata(ctx, domain.Show{
			ID: s.ShowID, Name: name, Overview: overview, Genre: genre, Year: year,
			MetadataSource: "manual", CreatedAt: "t", UpdatedAt: "t",
		}); err != nil {
			t.Fatalf("update show metadata: %v", err)
		}
		v := domain.Video{
			ID: id, SourceID: "s1", FileID: id, RelativePath: id + ".mp4", Path: "p",
			Size: 1, MTime: 1, Title: id,
			CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t",
		}
		if err := videos.Create(ctx, v); err != nil {
			t.Fatalf("create video: %v", err)
		}
		if err := series.BindMembers(ctx, s.ID,
			[]domain.EpisodeAssign{{VideoID: id, EpisodeNumber: 1, Title: id + ".mp4"}}); err != nil {
			t.Fatalf("bind member: %v", err)
		}
		if err := videos.SetTags(ctx, id, []string{tag}); err != nil {
			t.Fatalf("set tags: %v", err)
		}
		return s
	}
	s1 := mk("e1", "星际穿越", `C:\x\inter1`, "科幻", "太空之旅", "科幻", 2020)
	s2 := mk("e2", "星际穿越", `C:\x\inter2`, "科幻", "太空之旅", "科幻", 2020)
	mk("e3", "爱情公寓", `C:\x\apt`, "喜剧", "都市日常", "喜剧", 2015)
	if s1.ID == s2.ID {
		t.Fatalf("two same-named series must be independent, got %s", s1.ID)
	}

	all, err := series.List(ctx, domain.SeriesQuery{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %d, want 3", len(all))
	}

	t.Run("q matches name or overview", func(t *testing.T) {
		got, err := series.List(ctx, domain.SeriesQuery{Q: "星际"})
		if err != nil || len(got) != 2 {
			t.Fatalf("name q = %d err=%v", len(got), err)
		}
		got, err = series.List(ctx, domain.SeriesQuery{Q: "都市"})
		if err != nil || len(got) != 1 || got[0].Title != "爱情公寓" {
			t.Fatalf("overview q = %+v err=%v", got, err)
		}
	})

	t.Run("genre", func(t *testing.T) {
		got, err := series.List(ctx, domain.SeriesQuery{Genre: "喜剧"})
		if err != nil || len(got) != 1 || got[0].Title != "爱情公寓" {
			t.Fatalf("genre = %+v err=%v", got, err)
		}
	})

	t.Run("year", func(t *testing.T) {
		got, err := series.List(ctx, domain.SeriesQuery{Year: 2020})
		if err != nil || len(got) != 2 {
			t.Fatalf("year = %d err=%v", len(got), err)
		}
	})

	t.Run("tag via member video", func(t *testing.T) {
		got, err := series.List(ctx, domain.SeriesQuery{Tags: []string{"科幻"}})
		if err != nil || len(got) != 2 {
			t.Fatalf("tag = %d err=%v", len(got), err)
		}
		got, err = series.List(ctx, domain.SeriesQuery{Tags: []string{"科幻", "喜剧"}})
		if err != nil || len(got) != 0 {
			t.Fatalf("conflicting tags = %d err=%v", len(got), err)
		}
	})
}
