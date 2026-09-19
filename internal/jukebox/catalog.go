package jukebox

import (
	"encoding/json"
	"net/url"
	"strings"

	"ryolink/catalogs"
)

type catalogConfig struct {
	DefaultBaseURL string         `json:"default_base_url"`
	Sources        []sourceConfig `json:"sources"`
}

type sourceConfig struct {
	File    string `json:"file"`
	BaseURL string `json:"base_url"`
	Format  string `json:"format"`
	Artist  string `json:"artist"`
}

type catalogTrack struct {
	id     string
	title  string
	artist string
	url    string
}

type Catalog struct {
	tracks []Track
}

func NewCatalog() *Catalog {
	data, err := catalogs.FS.ReadFile("catalog.json")
	if err != nil {
		panic("jukebox: read catalog.json: " + err.Error())
	}
	var cfg catalogConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic("jukebox: parse catalog.json: " + err.Error())
	}

	c := &Catalog{}

	for _, src := range cfg.Sources {
		baseURL := src.BaseURL
		if baseURL == "" {
			baseURL = cfg.DefaultBaseURL
		}
		format := src.Format
		if format == "" {
			format = "path"
		}

		raw, err := catalogs.FS.ReadFile(src.File)
		if err != nil {
			panic("jukebox: read catalog " + src.File + ": " + err.Error())
		}

		parsed := parseCatalogFile(string(raw), format, baseURL, src.Artist)
		for _, ct := range parsed {
			c.tracks = append(c.tracks, Track{
				ID:     ct.id,
				Title:  ct.title,
				Artist: ct.artist,
				URL:    ct.url,
			})
		}
	}

	return c
}

func (c *Catalog) TrackCount() int {
	return len(c.tracks)
}

func (c *Catalog) AllTracks() []Track {
	out := make([]Track, len(c.tracks))
	copy(out, c.tracks)
	return out
}

func parseCatalogFile(raw, format, baseURL, artist string) []catalogTrack {
	switch format {
	case "id_title":
		return parseIDTitle(raw, baseURL, artist)
	default:
		return parsePath(raw, baseURL, artist)
	}
}

func parseIDTitle(raw, baseURL, artist string) []catalogTrack {
	var tracks []catalogTrack
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "!", 2)
		if len(parts) != 2 {
			continue
		}
		tracks = append(tracks, catalogTrack{
			id:     parts[0],
			title:  parts[1],
			artist: artist,
			url:    baseURL + parts[0],
		})
	}
	return tracks
}

func parsePath(raw, baseURL, artist string) []catalogTrack {
	var tracks []catalogTrack
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(strings.ToLower(line), ".mp3") {
			continue
		}
		trackURL := baseURL + encodePath(line)
		decoded, err := url.PathUnescape(line)
		if err != nil {
			decoded = line
		}
		parts := strings.Split(decoded, "/")
		name := parts[len(parts)-1]
		name = strings.TrimSuffix(name, ".mp3")
		name = stripTrackNumber(name)
		tracks = append(tracks, catalogTrack{
			id:     line,
			title:  name,
			artist: artist,
			url:    trackURL,
		})
	}
	return tracks
}

func encodePath(raw string) string {
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 {
		return url.PathEscape(raw)
	}
	return parts[0] + "/" + url.PathEscape(parts[1])
}

func stripTrackNumber(name string) string {
	if len(name) > 3 && name[0] >= '0' && name[0] <= '9' {
		for i, c := range name {
			if c == ' ' || c == '-' {
				return strings.TrimSpace(name[i+1:])
			}
			if i > 3 {
				break
			}
		}
	}
	return name
}
