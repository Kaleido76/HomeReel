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

	for _, show := range []domain.Show{
		{ID: "show1", Name: "星际穿越", Overview: "太空之旅", Genre: "科幻", Year: 2020, MetadataSource: "manual", CreatedAt: "t", UpdatedAt: "t"},
		{ID: "show2", Name: "爱情公寓", Overview: "都市日常", Genre: "喜剧", Year: 2015, MetadataSource: "manual", CreatedAt: "t", UpdatedAt: "t"},
	} {
		if err := shows.Create(ctx, show); err != nil {
			t.Fatalf("create show: %v", err)
		}
	}
	for _, season := range []struct {
		show string
		num  int
	}{
		{"show1", 1}, {"show1", 2}, {"show2", 1},
	} {
		if _, err := shows.EnsureSeason(ctx, season.show, season.num); err != nil {
			t.Fatalf("ensure season: %v", err)
		}
	}
	ep := func(id, show string, season int, tag string) {
		v := domain.Video{
			ID: id, SourceID: "s1", FileID: id, RelativePath: id + ".mp4", Path: "p",
			Size: 1, MTime: 1, Title: id,
			CreatedAt: "t", UpdatedAt: "t", LastScannedAt: "t",
		}
		if err := videos.Create(ctx, v); err != nil {
			t.Fatalf("create video: %v", err)
		}
		showName := map[string]string{"show1": "星际穿越", "show2": "爱情公寓"}[show]
		if _, err := shows.AssignSeason(ctx, showName, season,
			[]domain.EpisodeAssign{{VideoID: id, EpisodeNumber: 1, Title: id + ".mp4"}}); err != nil {
			t.Fatalf("assign: %v", err)
		}
		if err := videos.SetTags(ctx, id, []string{tag}); err != nil {
			t.Fatalf("set tags: %v", err)
		}
	}
	ep("e1", "show1", 1, "科幻")
	ep("e2", "show1", 2, "科幻")
	ep("e3", "show2", 1, "喜剧")

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
