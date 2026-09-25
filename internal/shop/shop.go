// Package shop is the Ryoku store front: a configured catalog of artifacts
// (ISO images, recovery scripts, rescue files) that the TUI browses, the
// web landing page lists, and the HTTP surface hands out. Items with a
// local `path` are streamed by ryolink itself (Range-safe, resumable);
// items with an external `url` are redirected — large ISOs belong behind a
// mirror, and the storefront shows their canonical address honestly instead
// of proxying gigabytes.
package shop

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ryolink/internal/config"
)

// Item is one storefront entry with its resolved on-disk state.
type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // iso | script | file
	Description string `json:"desc"`
	Details     string `json:"details,omitempty"`
	Version     string `json:"version"`
	Logo        string `json:"logo,omitempty"`
	URL         string `json:"url,omitempty"` // external canonical URL, if linked
	Size        int64  `json:"size"`          // bytes, -1 when unavailable
	SHA256      string `json:"sha256,omitempty"`
	Downloads   int    `json:"downloads"`
	Local       bool   `json:"local"`   // served by this instance
	Missing     bool   `json:"missing"` // configured but not on disk right now
}

// Shop owns the catalog, the download counters, and the HTTP surface.
// RegisterRoutes mounts onto the shared web mux (same listener as radio).
type Shop struct {
	cfg     config.StoreConfig
	dataDir string
	baseURL string // public origin users download from (e.g. https://ryoku.dev)
	db      *sql.DB

	mu    sync.RWMutex
	items []Item
}

// New builds the storefront from config. Relative item paths resolve against
// dataDir. baseURL is the canonical public origin; when empty a relative
// path is advertised instead.
func New(cfg config.StoreConfig, dataDir, baseURL string, db *sql.DB) (*Shop, error) {
	s := &Shop{cfg: cfg, dataDir: dataDir, baseURL: strings.TrimSuffix(baseURL, "/"), db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	s.reindex()
	return s, nil
}

func (s *Shop) migrate() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS shop_downloads (
		item_id  TEXT PRIMARY KEY,
		count    INTEGER NOT NULL DEFAULT 0,
		last_at  DATETIME
	);`)
	return err
}

// reindex refreshes the resolved item list from config + disk state.
func (s *Shop) reindex() {
	items := make([]Item, 0, len(s.cfg.Items))
	for _, it := range s.cfg.Items {
		e := Item{
			ID: it.ID, Name: it.Name, Kind: normalizeKind(it.Kind),
			Description: it.Description, Details: it.Details, Version: it.Version, URL: it.URL,
			Logo: strings.Join(it.LogoLines(), "\n"),
			Size: -1,
		}
		if it.Path != "" {
			e.Local = true
			p := resolvePath(s.dataDir, it.Path)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				e.Size = st.Size()
				e.SHA256 = fileDigest(p)
			} else {
				e.Missing = true
			}
		}
		items = append(items, e)
	}
	counts := map[string]int{}
	if s.db != nil {
		rows, err := s.db.Query(`SELECT item_id, count FROM shop_downloads`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id string
				var n int
				if err := rows.Scan(&id, &n); err == nil {
					counts[id] = n
				}
			}
		}
	}
	s.mu.Lock()
	for i := range items {
		items[i].Downloads = counts[items[i].ID]
	}
	s.items = items
	s.mu.Unlock()
}

func resolvePath(dataDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dataDir, p)
}

// Reindex re-reads disk state (files appearing/disappearing without a
// restart). Safe to call concurrently.
func (s *Shop) Reindex() { s.reindex() }

func normalizeKind(k string) string {
	switch strings.ToLower(k) {
	case "iso", "script", "file":
		return strings.ToLower(k)
	default:
		return "file"
	}
}

var digestCache sync.Map

// dlHooks lets tests observe download recording (it runs on the handler
// goroutine after ServeContent returns, racing the client's read loop).
var (
	dlHooksMu sync.Mutex
	dlHooks   []func(string)
)

// AddDownloadHook registers an observer fired on every recorded download.
// Test-only; production callers pass none.
func AddDownloadHook(fn func(string)) {
	dlHooksMu.Lock()
	dlHooks = append(dlHooks, fn)
	dlHooksMu.Unlock()
} // "path|size|mtime" -> digest

// fileDigest caches sha256 per (path,size,mtime); re-hashing a 3 GB ISO on
// every render would be ruinous.
func fileDigest(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	key := fmt.Sprintf("%s|%d|%d", path, st.Size(), st.ModTime().UnixNano())
	if v, ok := digestCache.Load(key); ok {
		return v.(string)
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	sum := hex.EncodeToString(h.Sum(nil))
	digestCache.Store(key, sum)
	return sum
}

// Items returns a snapshot of the catalog with current counts and public
// URLs filled in.
func (s *Shop) Items() []Item {
	s.mu.RLock()
	out := make([]Item, len(s.items))
	copy(out, s.items)
	s.mu.RUnlock()
	for i := range out {
		if out[i].URL == "" {
			out[i].URL = s.PublicURL(out[i].ID)
		}
	}
	return out
}

// Item looks one entry up by id.
func (s *Shop) Item(id string) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// configuredPath returns the raw configured path for an item id.
func (s *Shop) configuredPath(id string) string {
	for _, c := range s.cfg.Items {
		if c.ID == id {
			return c.Path
		}
	}
	return ""
}

// Title returns the storefront header text.
func (s *Shop) Title() string { return s.cfg.Title }

// Enabled reports whether the store front is switched on.
func (s *Shop) Enabled() bool { return s.cfg.Enabled }

func (s *Shop) recordDownload(id string) {
	defer func() {
		dlHooksMu.Lock()
		for _, fn := range dlHooks {
			fn(id)
		}
		dlHooksMu.Unlock()
	}()
	if s.db != nil {
		_, _ = s.db.Exec(`
			INSERT INTO shop_downloads (item_id, count, last_at)
			VALUES (?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(item_id) DO UPDATE SET
				count = count + 1, last_at = CURRENT_TIMESTAMP`, id)
	}
	s.mu.Lock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Downloads++
		}
	}
	s.mu.Unlock()
}

// PublicURL renders the user-facing download URL for an item.
func (s *Shop) PublicURL(id string) string {
	if it, ok := s.Item(id); ok && it.URL != "" && !strings.HasPrefix(it.URL, "/") {
		return it.URL
	}
	rel := "/files/" + urlEscape(id)
	if s.baseURL != "" {
		return s.baseURL + rel
	}
	return rel
}

func urlEscape(id string) string {
	return strings.ReplaceAll(id, " ", "%20")
}

// RegisterRoutes mounts the shop's HTTP surface onto mux:
//
//	/            the store landing page (when the shop owns the mux)
//	/api/items   JSON catalog
//	/files/<id>  streamed local items (Range-safe)
//	/d/<id>      redirect to the external mirror URL
func (s *Shop) RegisterRoutes(mux *http.ServeMux, ownLanding bool) {
	mux.HandleFunc("/api/items", s.handleAPI)
	mux.HandleFunc("/files/", s.handleFile)
	mux.HandleFunc("/d/", s.handleRedirect)
	if ownLanding {
		mux.HandleFunc("/", s.handleLanding)
	}
}

type apiResponse struct {
	Title string `json:"title"`
	Items []Item `json:"items"`
}

func (s *Shop) handleAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(apiResponse{Title: s.cfg.Title, Items: s.Items()})
}

// handleFile streams a local item. Range requests are honored
// (ServeContent), so an interrupted download resumes instead of restarting.
func (s *Shop) handleFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/files/")
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		http.NotFound(w, r)
		return
	}
	it, ok := s.Item(id)
	if !ok || !it.Local || it.Missing {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	path := resolvePath(s.dataDir, s.configuredPath(id))
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName(it, path)+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch it.Kind {
	case "script":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case "iso":
		w.Header().Set("Content-Type", "application/x-iso9660-image")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if sum := fileDigest(path); sum != "" {
		w.Header().Set("X-Sha256", sum)
	}
	// Count one whole GET only: HEAD and range resumes must not inflate it.
	cw := &countWriter{ResponseWriter: w}
	http.ServeContent(cw, r, filepath.Base(path), st.ModTime(), f)
	if r.Method == http.MethodGet && r.Header.Get("Range") == "" && cw.n >= st.Size() {
		s.recordDownload(id)
	}
}

// handleRedirect sends clients to the external canonical URL (mirror/CDN).
func (s *Shop) handleRedirect(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/d/")
	it, ok := s.Item(id)
	if !ok || it.URL == "" {
		http.NotFound(w, r)
		return
	}
	s.recordDownload(id)
	http.Redirect(w, r, it.URL, http.StatusFound)
}

func downloadName(it Item, path string) string {
	base := filepath.Base(path)
	// ISOs and scripts read better with the version baked into the name
	if it.Version != "" && !strings.Contains(base, it.Version) {
		ext := filepath.Ext(base)
		return strings.TrimSuffix(base, ext) + "-" + it.Version + ext
	}
	return base
}

// countWriter tallies bytes written so a completed whole-file GET can be
// recorded as exactly one download.
type countWriter struct {
	http.ResponseWriter
	n int64
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

func (c *countWriter) ReadFrom(r io.Reader) (int64, error) {
	// ServeContent uses io.Copy → ReadFrom for full sends; count the bytes.
	n, err := io.Copy(c.ResponseWriter, r)
	c.n += n
	return n, err
}

// ---- landing page -----------------------------------------------------------

type pageData struct {
	Title   string
	Items   []Item
	Updated string
}

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"bytes": humanBytes,
}).Parse(landingHTML))

func humanBytes(n int64) string {
	switch {
	case n < 0:
		return "—"
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1024*1024*1024))
	}
}

func (s *Shop) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, pageData{
		Title:   s.cfg.Title,
		Items:   s.Items(),
		Updated: time.Now().UTC().Format("2006-01-02"),
	})
}
