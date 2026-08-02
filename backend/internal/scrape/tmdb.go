package scrape

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const tmdbAPIBase = "https://api.themoviedb.org/3"
const tmdbImageBase = "https://image.tmdb.org/t/p"

// Candidate is one TMDB search hit shown to the user for confirmation.
type Candidate struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Year      string `json:"year"`
	Overview  string `json:"overview"`
	Poster    string `json:"poster"`
	MediaType string `json:"media_type"` // movie | tv
}

// tmdbDetail is the subset of a TMDB detail response we apply.
type tmdbDetail struct {
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	VoteAverage  float64 `json:"vote_average"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Credits struct {
		Cast []struct {
			Name string `json:"name"`
		} `json:"cast"`
	} `json:"credits"`
}

type tmdbClient struct {
	cfg     TMDBConfig
	base    string
	imgBase string
	http    *http.Client
	dataDir string
}

func newTMDBClient(cfg TMDBConfig, dataDir string) *tmdbClient {
	return &tmdbClient{
		cfg:     cfg,
		base:    tmdbAPIBase,
		imgBase: tmdbImageBase,
		http:    &http.Client{Timeout: 15 * time.Second},
		dataDir: dataDir,
	}
}

func (c *tmdbClient) enabled() bool {
	return c.cfg.APIKey != ""
}

func (c *tmdbClient) search(ctx context.Context, query, mediaType string, year int) ([]Candidate, error) {
	if !c.enabled() {
		return nil, ErrNoTMDB
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/search/"+mediaType, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("api_key", c.cfg.APIKey)
	q.Set("language", c.language())
	q.Set("query", query)
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	req.URL.RawQuery = q.Encode()

	var raw struct {
		Results []struct {
			ID          int     `json:"id"`
			Title       string  `json:"title"`
			Name        string  `json:"name"`
			ReleaseDate string  `json:"release_date"`
			FirstAir    string  `json:"first_air_date"`
			Overview    string  `json:"overview"`
			PosterPath  string  `json:"poster_path"`
			VoteAverage float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := c.doJSON(ctx, req, &raw); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(raw.Results))
	for _, r := range raw.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		out = append(out, Candidate{
			ID:        r.ID,
			Title:     title,
			Year:      yearFromDate(r.ReleaseDate, r.FirstAir),
			Overview:  r.Overview,
			Poster:    imageURL(c.imgBase, r.PosterPath),
			MediaType: mediaType,
		})
	}
	return out, nil
}

// detail fetches a TMDB detail record (with credits).
func (c *tmdbClient) detail(ctx context.Context, id, mediaType string) (tmdbDetail, error) {
	var d tmdbDetail
	if !c.enabled() {
		return d, ErrNoTMDB
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/%s/%s", c.base, mediaType, id), nil)
	if err != nil {
		return d, err
	}
	q := req.URL.Query()
	q.Set("api_key", c.cfg.APIKey)
	q.Set("language", c.language())
	q.Set("append_to_response", "credits")
	req.URL.RawQuery = q.Encode()
	if err := c.doJSON(ctx, req, &d); err != nil {
		return d, err
	}
	return d, nil
}

// saveImage downloads a TMDB image file into dataDir/<kind>/<name>.
func (c *tmdbClient) saveImage(ctx context.Context, url, dir, name string) (string, error) {
	if url == "" {
		return "", nil
	}
	full := filepath.Join(c.dataDir, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(full, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tmdb image %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(dir, name)), nil
}

func (c *tmdbClient) doJSON(ctx context.Context, req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb api %s: status %d", req.URL.Path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *tmdbClient) language() string {
	if c.cfg.Language == "" {
		return "zh-CN"
	}
	return c.cfg.Language
}

func imageURL(base, path string) string {
	if path == "" {
		return ""
	}
	return base + "/w500" + path
}

func yearFromDate(dates ...string) string {
	for _, d := range dates {
		if len(d) >= 4 {
			if _, err := strconv.Atoi(d[:4]); err == nil {
				return d[:4]
			}
		}
	}
	return ""
}

func detailTitle(d tmdbDetail) string {
	if d.Title != "" {
		return d.Title
	}
	return d.Name
}

func detailGenre(d tmdbDetail) string {
	names := make([]string, 0, len(d.Genres))
	for _, g := range d.Genres {
		names = append(names, g.Name)
	}
	return strings.Join(names, ", ")
}

func detailCast(d tmdbDetail) string {
	names := make([]string, 0, 10)
	for _, c := range d.Credits.Cast {
		if c.Name == "" {
			continue
		}
		names = append(names, c.Name)
		if len(names) >= 10 {
			break
		}
	}
	return strings.Join(names, ", ")
}

func detailYear(d tmdbDetail) int {
	for _, s := range []string{d.ReleaseDate, d.FirstAirDate} {
		if len(s) >= 4 {
			if y, err := strconv.Atoi(s[:4]); err == nil {
				return y
			}
		}
	}
	return 0
}
