package scrape

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Metadata is the subset of a Kodi/JellyFin .nfo file the library applies to
// videos and shows (ADR-016).
type Metadata struct {
	Title     string
	Year      int
	Rating    float64
	Overview  string
	Genre     string
	Studio    string
	CastText  string
	Season    int
	Episode   int
	HasSeason bool
}

// nfo mirrors the common Kodi fields across <movie>/<tvshow>/<episodedetails>
// roots. Rating lives either in the legacy <rating> element or in the newer
// <ratings><rating><value> block.
type nfo struct {
	Title   string `xml:"title"`
	Year    int    `xml:"year"`
	Rating  string `xml:"rating"`
	Ratings struct {
		Rating []struct {
			Value string `xml:"value"`
		} `xml:"rating"`
	} `xml:"ratings"`
	Plot   string   `xml:"plot"`
	Genre  []string `xml:"genre"`
	Studio string   `xml:"studio"`
	Cast   []struct {
		Name string `xml:"name"`
	} `xml:"cast"`
	Season  string `xml:"season"`
	Episode string `xml:"episode"`
}

// ParseNFO parses a Kodi/JellyFin .nfo file. An empty Metadata is returned
// when the file is absent; malformed files return an error so the caller can
// log and continue.
func ParseNFO(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, err
	}
	var n nfo
	if err := xml.Unmarshal(data, &n); err != nil {
		return Metadata{}, fmt.Errorf("parse nfo %s: %w", path, err)
	}
	m := Metadata{
		Title:    strings.TrimSpace(n.Title),
		Year:     n.Year,
		Overview: strings.TrimSpace(n.Plot),
		Genre:    strings.Join(trimAll(n.Genre), ", "),
		Studio:   strings.TrimSpace(n.Studio),
	}
	for _, c := range n.Cast {
		if name := strings.TrimSpace(c.Name); name != "" {
			if m.CastText != "" {
				m.CastText += ", "
			}
			m.CastText += name
		}
	}
	if r := strings.TrimSpace(n.Rating); r != "" {
		m.Rating = parseRating(r)
	} else {
		for _, r := range n.Ratings.Rating {
			if v := strings.TrimSpace(r.Value); v != "" {
				m.Rating = parseRating(v)
				break
			}
		}
	}
	if n.Season != "" {
		if s, err := strconv.Atoi(strings.TrimSpace(n.Season)); err == nil {
			m.Season = s
			m.HasSeason = true
		}
	}
	if n.Episode != "" {
		m.Episode, _ = strconv.Atoi(strings.TrimSpace(n.Episode))
	}
	return m, nil
}

func parseRating(s string) float64 {
	// Kodi may store "7.5" or "7.5 / 10"; take the first numeric token.
	for _, part := range strings.Fields(s) {
		if f, err := strconv.ParseFloat(part, 64); err == nil {
			return f
		}
	}
	return 0
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
