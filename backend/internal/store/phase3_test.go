package store

import (
	"context"
	"testing"

	"homereel/backend/internal/db"
	"homereel/backend/internal/domain"
	"homereel/backend/internal/search"
)

func newPhase3Store(t *testing.T) (domain.VideoRepo, domain.ShowRepo, domain.CollectionRepo, domain.HistoryRepo) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO storages (id, name, type, root_path, created_at)
		VALUES ('s1', 'test', 'internal', 'C:\\Videos', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	return NewVideoRepo(database), NewShowRepo(database), NewCollectionRepo(database), NewHistoryRepo(database)
}

func mkVideo(id, title string) domain.Video {
	return domain.Video{
		ID: id, StorageID: "s1", FileID: "f" + id, RelativePath: title + ".mp4",
		Path: "C:\\Videos\\" + title + ".mp4", Size: 10, MTime: 1, Title: title,
		CreatedAt: "2026-01-01T00:00:00.000000000Z", UpdatedAt: "2026-01-01T00:00:00.000000000Z",
		LastScannedAt: "2026-01-01T00:00:00.000000000Z",
	}
}

func TestEpisodeGroupingAndSearch(t *testing.T) {
	videos, shows, _, _ := newPhase3Store(t)
	ctx := context.Background()

	// Show must exist before episodes can reference it (FK).
	now := "2026-01-01T00:00:00.000000000Z"
	if err := shows.Create(ctx, domain.Show{ID: "show-1", Name: "Game of Thrones",
		MetadataSource: "manual", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := shows.EnsureSeason(ctx, "show-1", 1, "tv"); err != nil {
		t.Fatal(err)
	}

	if err := videos.Create(ctx, mkVideo("e1", "GoT.S01E01")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("e2", "GoT.S01E02")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("m1", "Interstellar")); err != nil {
		t.Fatal(err)
	}

	if err := videos.AssignEpisode(ctx, "e1", "show-1", 1, 1, "Winter Is Coming"); err != nil {
		t.Fatal(err)
	}
	if err := videos.AssignEpisode(ctx, "e2", "show-1", 1, 2, "The Kingsroad"); err != nil {
		t.Fatal(err)
	}
	if err := videos.AssignMovie(ctx, "m1"); err != nil {
		t.Fatal(err)
	}

	// Video kind + grouping columns persisted.
	e1, err := videos.Get(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if e1.Kind != "episode" || e1.SeasonNumber != 1 || e1.EpisodeNumber != 1 || e1.EpisodeTitle != "Winter Is Coming" {
		t.Errorf("episode grouping wrong: %+v", e1)
	}

	// Show derived counts: 2 episodes, 1 season, 0 unwatched.
	show, err := shows.Get(ctx, "show-1")
	if err != nil {
		t.Fatal(err)
	}
	if show.EpisodeCount != 2 || show.SeasonCount != 1 || show.UnwatchedCount != 2 {
		t.Errorf("show counts wrong: %+v", show)
	}

	// FTS search hits the show name via an episode's search_text (covered in
	// TestFTS5SearchIntegration).

	// Verify movie kind.
	m1, err := videos.Get(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if m1.Kind != "movie" || m1.ShowID != "" {
		t.Errorf("movie grouping wrong: %+v", m1)
	}
}

func TestTagsCollectionsContinueWatching(t *testing.T) {
	videos, _, collections, history := newPhase3Store(t)
	ctx := context.Background()

	if err := videos.Create(ctx, mkVideo("v1", "Movie A")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("v2", "Movie B")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("v3", "Movie C")); err != nil {
		t.Fatal(err)
	}

	// Tags.
	if err := videos.SetTags(ctx, "v1", []string{"科幻", "经典"}); err != nil {
		t.Fatal(err)
	}
	tags, err := videos.Tags(ctx, "v1")
	if err != nil || len(tags) != 2 {
		t.Fatalf("tags = %v, err = %v", tags, err)
	}
	all, err := videos.AllTags(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("all tags = %v, err = %v", all, err)
	}
	if all[0].Count != 1 {
		t.Errorf("tag count wrong: %+v", all[0])
	}

	// Collection CRUD.
	c1, err := collections.Create(ctx, "我的最爱")
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.AddVideo(ctx, c1.ID, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := collections.AddVideo(ctx, c1.ID, "v2"); err != nil {
		t.Fatal(err)
	}
	members, err := collections.Videos(ctx, c1.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("collection videos = %d, err = %v", len(members), err)
	}
	if err := collections.RemoveVideo(ctx, c1.ID, "v1"); err != nil {
		t.Fatal(err)
	}
	members, _ = collections.Videos(ctx, c1.ID)
	if len(members) != 1 {
		t.Errorf("after remove, members = %d", len(members))
	}

	// ContinueWatching: v1 started halfway, v3 completed → excluded.
	if err := history.Upsert(ctx, domain.History{VideoID: "v1", User: "local", Progress: 30, UpdatedAt: "2026-01-02T00:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	if err := history.Upsert(ctx, domain.History{VideoID: "v3", User: "local", Progress: 100, UpdatedAt: "2026-01-03T00:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	// Give v1 a duration so the < duration - 20 guard applies.
	if err := videos.UpdateProbe(ctx, domain.Video{ID: "v1", Duration: 100}); err != nil {
		t.Fatal(err)
	}
	cw, err := videos.ContinueWatching(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cw) != 1 || cw[0].ID != "v1" {
		t.Errorf("continue watching = %+v", cw)
	}
}

func TestFTS5SearchIntegration(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	videos := NewVideoRepo(database)
	shows := NewShowRepo(database)
	if _, err := database.Exec(`INSERT INTO storages (id, name, type, root_path, created_at)
		VALUES ('s1', 'test', 'internal', 'C:\\Videos', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("a", "Interstellar")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("b", "Dunkirk")); err != nil {
		t.Fatal(err)
	}
	if err := videos.Create(ctx, mkVideo("c", "Movie C")); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-01T00:00:00.000000000Z"
	if err := shows.Create(ctx, domain.Show{ID: "show-1", Name: "The Dark Knight",
		MetadataSource: "manual", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := videos.AssignEpisode(ctx, "c", "show-1", 1, 1, "Episode One"); err != nil {
		t.Fatal(err)
	}

	fts := search.NewFTS5(database, videos)
	results, err := fts.Search(ctx, "Interstellar", search.Options{Limit: 10})
	if err != nil || len(results) != 1 || results[0].ID != "a" {
		t.Fatalf("search title: %+v err=%v", results, err)
	}
	// Show name matches the episode through its search_text.
	results, err = fts.Search(ctx, "Dark Knight", search.Options{Limit: 10})
	if err != nil || len(results) != 1 || results[0].ID != "c" {
		t.Fatalf("search show name: %+v err=%v", results, err)
	}
	// Tags are indexed.
	if err := videos.SetTags(ctx, "b", []string{"战争"}); err != nil {
		t.Fatal(err)
	}
	results, err = fts.Search(ctx, "战争", search.Options{Limit: 10})
	if err != nil || len(results) != 1 || results[0].ID != "b" {
		t.Fatalf("search tag: %+v err=%v", results, err)
	}
}
