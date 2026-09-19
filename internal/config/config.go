package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RyolinkConfig is the identity block: what users see, not how it runs.
type RyolinkConfig struct {
	Name    string `yaml:"name"`
	Domain  string `yaml:"domain"`
	Tagline string `yaml:"tagline"`
}

type OwnerConfig struct {
	Name        string `yaml:"name"`
	Fingerprint string `yaml:"fingerprint"`
}

type RoomConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// ServerConfig covers listener + storage placement. Everything the process
// reads or writes lives under DataDir, so the whole instance can live in one
// directory (or volume) and be moved by changing one line.
type ServerConfig struct {
	Host        string `yaml:"host"`         // bind address (default 0.0.0.0)
	Port        int    `yaml:"port"`         // SSH port (default 2222)
	DataDir     string `yaml:"data_dir"`     // db, host key, logs, signals
	IdleTimeout string `yaml:"idle_timeout"` // drop dead sessions (default 2h0m)
	LogFile     string `yaml:"log_file"`     // "" = stderr (journald-friendly)
	WebAudio    *bool  `yaml:"web_audio"`    // nil = default off
	WebBind     string `yaml:"web_bind"`     // audio stream bind (default 127.0.0.1)
	WebPort     int    `yaml:"web_port"`     // audio stream port (default 8090)
}

// SecurityConfig is the app-layer abuse control. The server accepts any SSH
// key (the key IS the identity), so everything a real sshd would enforce is
// done here instead: connection budgets, auth-failure bans, scanner-probe
// bans, and a persistent IP deny list.
type SecurityConfig struct {
	MaxConns        int      `yaml:"max_conns"`         // total in-flight TCP conns (default 512)
	MaxConnsPerIP   int      `yaml:"max_conns_per_ip"`  // concurrent per IP (default 4)
	NewConnsPerMin  int      `yaml:"new_conns_per_min"` // per-IP connection rate (default 30)
	MaxAuthFails    int      `yaml:"max_auth_fails"`    // per IP before temp ban (default 10)
	BanMinutes      int      `yaml:"ban_minutes"`       // temp ban length (default 30)
	ProbeBanMinutes int      `yaml:"probe_ban_minutes"` // scanner-hammering ban (default 60)
	DenyCIDRs       []string `yaml:"deny_cidrs"`        // always-blocked ranges
	AllowCIDRs      []string `yaml:"allow_cidrs"`       // when set, ONLY these may connect
}

type Config struct {
	Ryolink  RyolinkConfig  `yaml:"ryolink"`
	Owner    OwnerConfig    `yaml:"owner"`
	Server   ServerConfig   `yaml:"server"`
	Security SecurityConfig `yaml:"security"`
	API      APIConfig      `yaml:"api"`
	Store    StoreConfig    `yaml:"store"`
	Bind     BindConfig     `yaml:"bind"`
	Rooms    []RoomConfig   `yaml:"rooms"`
}

// BindConfig carries interaction preferences (input-first vs keys).
type BindConfig struct {
	// Mouse: "auto" (default — pointer clicks activate, keys still work),
	// "on" (full motion tracking), "off" (keyboard-only fallback).
	Mouse string `yaml:"mouse"`
}

// StoreConfig turns ryolink's front page into the Ryoku software store:
// a browsable catalog in the TUI plus an HTTP download surface. The web
// endpoints share server.web_bind/web_port (the same mux as the radio
// stream); PublicURL is the base users see (e.g. https://dl.ryoku.dev) —
// put Caddy in front and keep the bind loopback.
// Items with a local `path` are streamed by ryolink itself (Range-safe);
// items with an external `url` are redirected — large ISOs belong behind a
// mirror, and the storefront shows their canonical address honestly.
type StoreConfig struct {
	Enabled   bool        `yaml:"enabled"`
	PublicURL string      `yaml:"public_url"` // base URL shown to users
	Title     string      `yaml:"title"`      // storefront header (default "Ryoku Store")
	Items     []StoreItem `yaml:"items"`
}

type StoreItem struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"` // iso | script | file
	Description string `yaml:"desc"`
	Version     string `yaml:"version"`
	Path        string `yaml:"path"` // local file (relative paths resolve against data_dir)
	URL         string `yaml:"url"`  // external canonical URL (wins over serving)
	Logo        string `yaml:"logo"` // ASCII art for the item's card (multi-line YAML)
}

// LogoLines returns the item's ASCII logo as lines, trimmed of surrounding
// blanks so card heights stay tight.
func (i StoreItem) LogoLines() []string {
	if i.Logo == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(i.Logo, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// APIConfig maps the optional third-party keys. Values may also come from the
// process environment (OPENAI_API_KEY, ...); the env wins so secrets can stay
// out of the file.
type APIConfig struct {
	OpenAIKey      string `yaml:"openai_key"`
	KlipyKey       string `yaml:"klipy_key"`
	ExaKey         string `yaml:"exa_key"`
	RedditClientID string `yaml:"reddit_client_id"`
	RedditSecret   string `yaml:"reddit_secret"`
}

var validRoomTypes = map[string]bool{
	"chat":    true,
	"gallery": true,
	"games":   true,
	"wargame": true,
	"store":   true,
}

// Default returns a config with every field safely populated; Load overlays
// the YAML on top of it, so an almost-empty file still runs.
func Default() *Config {
	c := &Config{}
	c.Server.Host = "0.0.0.0"
	c.Server.Port = 2222
	c.Server.IdleTimeout = "2h"
	c.Server.WebBind = "127.0.0.1"
	c.Server.WebPort = 8090
	f := false
	c.Server.WebAudio = &f
	c.Security.MaxConns = 512
	c.Security.MaxConnsPerIP = 4
	c.Security.NewConnsPerMin = 30
	c.Security.MaxAuthFails = 10
	c.Security.BanMinutes = 30
	c.Security.ProbeBanMinutes = 60
	c.Store.Title = "Ryoku Store"
	c.Bind.Mouse = "auto"
	return c
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.DataDir == "" {
		// Everything lives next to the config file unless told otherwise.
		abs, err := filepath.Abs(filepath.Dir(path))
		if err == nil {
			cfg.Server.DataDir = abs
		} else {
			cfg.Server.DataDir = "."
		}
	} else {
		abs, err := filepath.Abs(cfg.Server.DataDir)
		if err == nil {
			cfg.Server.DataDir = abs
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Ryolink.Name == "" {
		return fmt.Errorf("ryolink.yaml: ryolink.name is required")
	}
	if c.Ryolink.Domain == "" {
		return fmt.Errorf("ryolink.yaml: ryolink.domain is required")
	}
	if c.Owner.Name == "" {
		return fmt.Errorf("ryolink.yaml: owner.name is required")
	}
	if c.Owner.Fingerprint == "" {
		return fmt.Errorf("ryolink.yaml: owner.fingerprint is required")
	}
	if len(c.Rooms) == 0 {
		return fmt.Errorf("ryolink.yaml: at least one room is required")
	}
	for _, r := range c.Rooms {
		if r.Name == "" {
			return fmt.Errorf("ryolink.yaml: room name cannot be empty")
		}
		if !validRoomTypes[r.Type] {
			return fmt.Errorf("ryolink.yaml: invalid room type %q for room %q (valid: chat, gallery, games, wargame, store)", r.Type, r.Name)
		}
		if r.Type == "store" && !c.Store.Enabled {
			return fmt.Errorf("ryolink.yaml: room %q has type store but store.enabled is false", r.Name)
		}
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("ryolink.yaml: server.port must be 1-65535")
	}
	if c.Server.WebPort < 1 || c.Server.WebPort > 65535 {
		return fmt.Errorf("ryolink.yaml: server.web_port must be 1-65535")
	}
	for _, cidr := range append(append([]string{}, c.Security.DenyCIDRs...), c.Security.AllowCIDRs...) {
		if err := validateCIDR(cidr); err != nil {
			return err
		}
	}
	switch c.Bind.Mouse {
	case "auto", "on", "off":
	default:
		return fmt.Errorf("ryolink.yaml: bind.mouse must be auto, on, or off (got %q)", c.Bind.Mouse)
	}
	if c.Store.Enabled {
		seen := map[string]bool{}
		for _, it := range c.Store.Items {
			if it.ID == "" {
				return fmt.Errorf("ryolink.yaml: store.items: every item needs an id")
			}
			if seen[it.ID] {
				return fmt.Errorf("ryolink.yaml: store.items: duplicate id %q", it.ID)
			}
			seen[it.ID] = true
			if it.Path == "" && it.URL == "" {
				return fmt.Errorf("ryolink.yaml: store item %q needs a path or a url", it.ID)
			}
		}
	}
	return nil
}

// RoomNames returns room names in config order.
func (c *Config) RoomNames() []string {
	names := make([]string, len(c.Rooms))
	for i, r := range c.Rooms {
		names[i] = r.Name
	}
	return names
}

// FirstRoom returns the first room name (the landing room). With the store
// enabled this is typically the store room — the front page — while chat
// rooms are what you wander into afterwards.
func (c *Config) FirstRoom() string {
	if len(c.Rooms) == 0 {
		return ""
	}
	return c.Rooms[0].Name
}

// BarRoom returns the room the bartender works: the first `chat` room, not
// necessarily the landing room (which is the store when one is configured).
func (c *Config) BarRoom() string {
	for _, r := range c.Rooms {
		if r.Type == "chat" {
			return r.Name
		}
	}
	return c.FirstRoom()
}

// RoomIsType checks if a room has a specific type.
func (c *Config) RoomIsType(roomName, roomType string) bool {
	for _, r := range c.Rooms {
		if r.Name == roomName && r.Type == roomType {
			return true
		}
	}
	return false
}

// RoomTypeMap returns a map of room name to room type for fast lookups.
func (c *Config) RoomTypeMap() map[string]string {
	m := make(map[string]string, len(c.Rooms))
	for _, r := range c.Rooms {
		m[r.Name] = r.Type
	}
	return m
}

// WebAudioEnabled reports whether the audio stream endpoint should listen.
func (c *Config) WebAudioEnabled() bool {
	return c.Server.WebAudio != nil && *c.Server.WebAudio
}

// Secret resolves an API credential: environment first, then the config file.
func (c *Config) Secret(envVar, fileVal string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fileVal
}
