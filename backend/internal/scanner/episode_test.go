package scanner

import "testing"

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		rel    string
		want   EpisodeHint
		wantSE bool
	}{
		// Directory structure: <Show>/Season N/<file>
		{rel: "Game of Thrones/Season 1/GoT S01E01.mkv",
			want:   EpisodeHint{Show: "Game of Thrones", Season: 1, Episode: 1},
			wantSE: true},
		{rel: "权力的游戏/第 1 季/第一集.mkv",
			want:   EpisodeHint{Show: "权力的游戏", Season: 1, Episode: 1},
			wantSE: true},
		// <Show> dir directly containing episode files
		{rel: "Breaking Bad/Breaking.Bad.S01E01.720p.mkv",
			want:   EpisodeHint{Show: "Breaking Bad", Season: 1, Episode: 1},
			wantSE: true},
		// Root-level file with filename rule
		{rel: "The.Office.S01E01.mkv",
			want:   EpisodeHint{Show: "The Office", Season: 1, Episode: 1},
			wantSE: true},
		{rel: "Stranger Things (2016) S1E2.mkv",
			want:   EpisodeHint{Show: "Stranger Things", Season: 1, Episode: 2},
			wantSE: true},
		// 中文集规则
		{rel: "凡人修仙传/第01集.mkv",
			want:   EpisodeHint{Show: "凡人修仙传", Episode: 1},
			wantSE: true},
		// Single movie → not an episode
		{rel: "Interstellar.mkv", wantSE: false},
		{rel: "Movies/Inception (2010)/Inception.mkv", wantSE: false},
		{rel: "D:/video.mp4", wantSE: false},
	}

	for _, c := range cases {
		got := ParseEpisode(c.rel)
		if got.HasSE != c.wantSE {
			t.Errorf("ParseEpisode(%q).HasSE = %v, want %v", c.rel, got.HasSE, c.wantSE)
			continue
		}
		if !c.wantSE {
			continue
		}
		if got.Show != c.want.Show || got.Season != c.want.Season || got.Episode != c.want.Episode {
			t.Errorf("ParseEpisode(%q) = %+v, want %+v", c.rel, got, c.want)
		}
	}
}
