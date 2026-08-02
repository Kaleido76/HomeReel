package scanner

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// EpisodeHint is the show/season/episode grouping parsed from a relative path
// using JellyFin-compatible directory and file naming conventions.
type EpisodeHint struct {
	Show    string // show name ("" when not resolvable)
	Season  int
	Episode int
	HasSE   bool // matched an episode pattern (SxxEyy or 第x集)
}

var (
	reSE          = regexp.MustCompile(`(?i)\bs(\d{1,2})e(\d{1,3})\b`)
	reEpisodeCN   = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十]+)\s*集`)
	reSeasonDir   = regexp.MustCompile(`(?i)^\s*(?:season|s)\s*(\d{1,2})\s*$`)
	reSeasonDirCN = regexp.MustCompile(`^第\s*([0-9一二三四五六七八九十]+)\s*季\s*$`)
	rePart        = regexp.MustCompile(`(?i)\bpart\s*([0-9a-z]{1,3})\b`)
	rePartCN      = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十]+)\s*部`)
	reNumSuffix   = regexp.MustCompile(`[\s._-]?([0-9]{1,2})$`)
	reYearParen   = regexp.MustCompile(`\(\d{4}\)$`)
	reYearSuffix  = regexp.MustCompile(`\s\d{4}$`)
)

// ParseEpisode inspects a slash-separated relative path and decides whether it
// is a TV episode and, if so, to which show/season/episode it belongs. Files
// inside an explicit "Season N" folder are always treated as episodes, even
// when their name carries no SxxEyy marker (the folder is the series
// relationship); otherwise the file name must carry an episode marker.
func ParseEpisode(rel string) EpisodeHint {
	segments := strings.Split(rel, "/")
	filename := segments[len(segments)-1]
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	var hint EpisodeHint
	if m := reEpisodeCN.FindStringSubmatch(name); m != nil {
		hint.Episode = atoi(m[1])
		hint.HasSE = true
	} else if m := reSE.FindStringSubmatch(name); m != nil {
		hint.Season = atoi(m[1])
		hint.Episode = atoi(m[2])
		hint.HasSE = true
	}

	// Walk up the directory chain for the deepest "Season N" folder.
	parentSeason := -1
	showFromDir := ""
	for i := len(segments) - 2; i >= 0; i-- {
		if s, ok := parseSeasonDir(segments[i]); ok {
			parentSeason = s
			if i > 0 {
				showFromDir = segments[i-1]
			}
			break
		}
	}

	if parentSeason >= 0 {
		// <Show>/Season N/file: the folder fixes season & show.
		hint.Season = parentSeason
		hint.Show = cleanShowName(showFromDir)
		if !hint.HasSE {
			// No episode marker: derive the episode number from a trailing
			// numeric suffix (e.g. "e01").
			if m := reNumSuffix.FindStringSubmatch(name); m != nil {
				hint.Episode = atoi(m[1])
			}
			hint.HasSE = true
		}
		if hint.Show == "" {
			hint.Show = cleanShowName(showFromFilename(name))
		}
		return hint
	}

	if !hint.HasSE {
		return hint
	}
	if len(segments) >= 2 {
		// <Show>/file (episode naming carries season)
		hint.Show = cleanShowName(segments[len(segments)-2])
	}
	if hint.Show == "" {
		hint.Show = cleanShowName(showFromFilename(name))
	}
	return hint
}

// ParseMoviePart inspects a relative path and, when the file is one part of a
// movie franchise (Part N / 第N部 / numeric suffix), returns the franchise
// title and part number. ok=false when it is not a movie part.
func ParseMoviePart(rel string) (EpisodeHint, bool) {
	name := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(filepath.Base(rel)))
	var season int
	if m := rePartCN.FindStringSubmatch(name); m != nil {
		season = atoi(m[1])
	} else if m := rePart.FindStringSubmatch(name); m != nil {
		season = partToInt(m[1])
	} else if m := reNumSuffix.FindStringSubmatch(name); m != nil {
		season = atoi(m[1])
	}
	if season <= 0 {
		return EpisodeHint{}, false
	}
	show := cleanShowName(showFromFilename(name))
	if show == "" {
		return EpisodeHint{}, false
	}
	return EpisodeHint{Show: show, Season: season, Episode: 1}, true
}

// seriesKeyOf returns the normalised "title key" of a video's relative path
// (episode markers and part numbers removed), used for same-directory grouping.
func seriesKeyOf(rel string) string {
	name := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(filepath.Base(rel)))
	s := reSE.ReplaceAllString(name, "")
	s = reEpisodeCN.ReplaceAllString(s, "")
	s = rePart.ReplaceAllString(s, "")
	s = rePartCN.ReplaceAllString(s, "")
	s = reNumSuffix.ReplaceAllString(s, "")
	return cleanShowName(s)
}

// inSeasonDir reports whether the path sits directly inside a "Season N" or
// "第 N 季" folder, which is itself an explicit series relationship.
func inSeasonDir(rel string) bool {
	segments := strings.Split(rel, "/")
	if len(segments) < 2 {
		return false
	}
	_, ok := parseSeasonDir(segments[len(segments)-2])
	return ok
}

func parseSeasonDir(dir string) (int, bool) {
	if m := reSeasonDir.FindStringSubmatch(dir); m != nil {
		return atoi(m[1]), true
	}
	if m := reSeasonDirCN.FindStringSubmatch(dir); m != nil {
		return atoi(m[1]), true
	}
	return 0, false
}

// showFromFilename strips the episode/part markers and punctuation from a
// file name to recover the show title.
func showFromFilename(name string) string {
	s := reSE.ReplaceAllString(name, "")
	s = reEpisodeCN.ReplaceAllString(s, "")
	s = rePart.ReplaceAllString(s, "")
	s = rePartCN.ReplaceAllString(s, "")
	s = reNumSuffix.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "_", " ")
	return s
}

// cleanShowName normalises a show name: dots/underscores become spaces, extra
// whitespace is collapsed, and a trailing year like "(2011)" or "2011" drops.
func cleanShowName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '.' || r == '_' {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = reYearParen.ReplaceAllString(s, "")
	s = reYearSuffix.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// partToInt converts a Part marker (a/A=1, b=2, or a number) to an int.
func partToInt(s string) int {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	r := []rune(strings.ToLower(s))
	if len(r) == 1 && r[0] >= 'a' && r[0] <= 'z' {
		return int(r[0]-'a') + 1
	}
	return 0
}

func atoi(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return cnNum(s)
}

// cnNum converts a Chinese numeral (一..九十九) to an integer; 0 when invalid.
func cnNum(s string) int {
	digits := map[rune]int{'零': 0, '一': 1, '二': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	val := 0
	for _, r := range s {
		if r == '十' {
			if val == 0 {
				val = 1
			}
			val *= 10
			continue
		}
		d, ok := digits[r]
		if !ok {
			return 0
		}
		if val >= 10 {
			val += d
		} else {
			val = d
		}
	}
	return val
}
