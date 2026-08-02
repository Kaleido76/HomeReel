package scrape

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNFOTraditional(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Movie.nfo")
	content := `<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Interstellar</title>
  <year>2014</year>
  <rating>8.6</rating>
  <plot>A team explores a wormhole.</plot>
  <genre>Sci-Fi</genre>
  <genre>Adventure</genre>
  <studio>Paramount</studio>
  <cast><name>Matthew McConaughey</name></cast>
  <cast><name>Anne Hathaway</name></cast>
</movie>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseNFO(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Interstellar" || m.Year != 2014 || m.Rating != 8.6 {
		t.Errorf("basic fields wrong: %+v", m)
	}
	if m.Genre != "Sci-Fi, Adventure" {
		t.Errorf("genre = %q", m.Genre)
	}
	if m.CastText != "Matthew McConaughey, Anne Hathaway" {
		t.Errorf("cast = %q", m.CastText)
	}
	if m.Studio != "Paramount" || m.Overview == "" {
		t.Errorf("studio/overview wrong: %+v", m)
	}
}

func TestParseNFONewRatings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Show.nfo")
	content := `<tvshow>
  <title>Game of Thrones</title>
  <ratings>
    <rating name="default" max="10"><value>9.3</value></rating>
  </ratings>
</tvshow>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseNFO(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Game of Thrones" || m.Rating != 9.3 {
		t.Errorf("parsed = %+v", m)
	}
}

func TestParseNFOMissing(t *testing.T) {
	m, err := ParseNFO(filepath.Join(t.TempDir(), "none.nfo"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if m.Title != "" {
		t.Errorf("expected empty metadata, got %+v", m)
	}
}

func TestParseNFOEpisode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "S01E01.nfo")
	content := `<episodedetails>
  <title>Winter Is Coming</title>
  <season>1</season>
  <episode>1</episode>
</episodedetails>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseNFO(path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasSeason || m.Season != 1 || m.Episode != 1 || m.Title != "Winter Is Coming" {
		t.Errorf("episode nfo parsed = %+v", m)
	}
}
