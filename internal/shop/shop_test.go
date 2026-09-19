package shop

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // register the driver (store package does this in prod)
	"ryolink/internal/config"
)

func newTestShop(t *testing.T) (*Shop, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ryolink.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// stage one local artifact
	storeDir := filepath.Join(dir, "store")
	os.MkdirAll(storeDir, 0755)
	payload := strings.Repeat("recovery-script!", 400) // ~6.4 KB
	os.WriteFile(filepath.Join(storeDir, "ryoku-recovery.sh"), []byte(payload), 0644)

	cfg := config.StoreConfig{
		Enabled: true,
		Title:   "Ryoku Store",
		Items: []config.StoreItem{
			{ID: "recovery", Name: "ryoku-recovery", Kind: "script",
				Description: "panic button", Path: "store/ryoku-recovery.sh"},
			{ID: "iso", Name: "Ryoku Linux", Kind: "iso", Version: "0.59",
				URL: "https://mirror.example/ryoku-0.59.iso"},
			{ID: "gone", Name: "ghost", Kind: "file", Path: "store/missing.bin"},
		},
	}
	sh, err := New(cfg, dir, "https://ryoku.dev", db)
	if err != nil {
		t.Fatal(err)
	}
	return sh, payload
}

func newMux(t *testing.T, sh *Shop) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	sh.RegisterRoutes(mux, true)
	return mux
}

// downloadSignal lets tests wait until recordDownload has finished on the
// server goroutine — otherwise reading Item() after the response body races
// the increment.
func downloadSignal(t *testing.T) func() {
	t.Helper()
	ch := make(chan string, 16)
	AddDownloadHook(func(id string) {
		select {
		case ch <- id:
		default:
		}
	})
	return func() { t.Helper(); <-ch }
}

func TestCatalogResolution(t *testing.T) {
	sh, _ := newTestShop(t)
	items := sh.Items()
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	rec := byID["recovery"]
	if !rec.Local || rec.Missing || rec.Size <= 0 {
		t.Fatalf("local item wrong: %+v", rec)
	}
	if len(rec.SHA256) != 64 {
		t.Fatalf("sha256 missing: %q", rec.SHA256)
	}
	if rec.URL != "https://ryoku.dev/files/recovery" {
		t.Fatalf("local URL should use the public base: %q", rec.URL)
	}
	iso := byID["iso"]
	if iso.URL != "https://mirror.example/ryoku-0.59.iso" {
		t.Fatalf("external URL must pass through untouched: %q", iso.URL)
	}
	ghost := byID["gone"]
	if !ghost.Missing {
		t.Fatalf("missing file must be flagged: %+v", ghost)
	}
}

func TestDownloadServesAndCountsOnce(t *testing.T) {
	sh, payload := newTestShop(t)
	waitDl := downloadSignal(t)
	ts := httptest.NewServer(newMux(t, sh))
	defer ts.Close()

	// one whole GET = exactly one download
	resp, err := http.Get(ts.URL + "/files/recovery")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != payload {
		t.Fatal("served bytes differ from the staged file")
	}
	if got := resp.Header.Get("X-Sha256"); len(got) != 64 {
		t.Fatalf("missing X-Sha256 header: %q", got)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "ryoku-recovery.sh") {
		t.Fatalf("bad disposition: %q", cd)
	}
	waitDl() // the handler records right after serving the last byte
	it, _ := sh.Item("recovery")
	if it.Downloads != 1 {
		t.Fatalf("after one whole GET want 1 download, got %d", it.Downloads)
	}

	// a range request resumes but must NOT count again
	req, _ := http.NewRequest("GET", ts.URL+"/files/recovery", nil)
	req.Header.Set("Range", "bytes=10-99")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusPartialContent {
		t.Fatalf("want 206, got %d", resp2.StatusCode)
	}
	it, _ = sh.Item("recovery")
	if it.Downloads != 1 {
		t.Fatalf("range resume must not inflate the counter, got %d", it.Downloads)
	}

	// HEAD likewise
	reqH, _ := http.NewRequest("HEAD", ts.URL+"/files/recovery", nil)
	respH, err := http.DefaultClient.Do(reqH)
	if err != nil {
		t.Fatal(err)
	}
	respH.Body.Close()
	it, _ = sh.Item("recovery")
	if it.Downloads != 1 {
		t.Fatalf("HEAD must not count, got %d", it.Downloads)
	}
}

func TestMissingAndUnknown404(t *testing.T) {
	sh, _ := newTestShop(t)
	ts := httptest.NewServer(newMux(t, sh))
	defer ts.Close()
	for _, path := range []string{"/files/gone", "/files/nope", "/files/../../etc/passwd"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		// traversal must not be served; 404 (or 400) is fine
		if resp.StatusCode < 400 {
			t.Fatalf("%s should be refused, got %d", path, resp.StatusCode)
		}
	}
}

func TestMirrorRedirectCounts(t *testing.T) {
	sh, _ := newTestShop(t)
	waitDl := downloadSignal(t)
	ts := httptest.NewServer(newMux(t, sh))
	defer ts.Close()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't chase the external URL
		},
	}
	resp, err := client.Get(ts.URL + "/d/iso")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://mirror.example/ryoku-0.59.iso" {
		t.Fatalf("bad redirect target: %q", loc)
	}
	waitDl()
	it, _ := sh.Item("iso")
	if it.Downloads != 1 {
		t.Fatalf("mirror hop should count, got %d", it.Downloads)
	}
}

func TestDownloadNameCarriesVersion(t *testing.T) {
	it := Item{Name: "x", Kind: "iso", Version: "0.59"}
	if got := downloadName(it, "/data/store/ryoku.iso"); got != "ryoku-0.59.iso" {
		t.Fatalf("versioned name wrong: %q", got)
	}
	it2 := Item{Name: "x", Kind: "script", Version: "0.59"}
	if got := downloadName(it2, "/data/store/ryoku-0.59.iso"); got != "ryoku-0.59.iso" {
		t.Fatalf("already-versioned name must not double-append: %q", got)
	}
}

func TestLandingPageRenders(t *testing.T) {
	sh, _ := newTestShop(t)
	ts := httptest.NewServer(newMux(t, sh))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	html := string(body)
	for _, want := range []string{"Ryoku Store", "ryoku-recovery", "panic button", "https://mirror.example/ryoku-0.59.iso", "sha256"} {
		if !strings.Contains(html, want) {
			t.Fatalf("landing page missing %q", want)
		}
	}
	// the missing item is marked, not hidden
	if !strings.Contains(html, "staging soon") {
		t.Fatal("missing item must be honestly marked on the page")
	}
}

func TestCounterPersistenceAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ryolink.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(dir, "store")
	os.MkdirAll(storeDir, 0755)
	os.WriteFile(filepath.Join(storeDir, "a.sh"), []byte("x"), 0644)
	cfg := config.StoreConfig{
		Enabled: true, Title: "T",
		Items: []config.StoreItem{{ID: "a", Name: "a", Kind: "script", Path: "store/a.sh"}},
	}
	sh, err := New(cfg, dir, "", db)
	if err != nil {
		t.Fatal(err)
	}
	sh.recordDownload("a")
	db.Close()

	// reopen like a service restart
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	sh2, err := New(cfg, dir, "", db2)
	if err != nil {
		t.Fatal(err)
	}
	it, _ := sh2.Item("a")
	if it.Downloads != 1 {
		t.Fatalf("download counts must survive restart, got %d", it.Downloads)
	}
}

func TestRelativePublicURLWhenNoBase(t *testing.T) {
	sh, _ := newTestShop(t)
	sh.baseURL = ""
	if got := sh.PublicURL("recovery"); got != "/files/recovery" {
		t.Fatalf("without a base the URL must stay relative, got %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "—"},
		{512, "512 B"},
		{2048, "2.0 KiB"},
		{3 * 1024 * 1024, "3.0 MiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d)=%q want %q", c.n, got, c.want)
		}
	}
}

// the digest must be content-addressed: editing the file changes it, and the
// cache keys on (path,size,mtime) so a same-size edit within one second can
// legitimately return the stale digest — assert the normal case only.
func TestDigestRefreshOnNewSize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	os.WriteFile(p, []byte("aaa"), 0644)
	d1 := fileDigest(p)
	os.WriteFile(p, []byte("aaaaaa"), 0644)
	d2 := fileDigest(p)
	if d1 == "" || d2 == "" || d1 == d2 {
		t.Fatalf("digest must follow content: %q vs %q", d1[:8], d2[:8])
	}
	// sanity: d2 is sha256("aaaaaa")
	if want := "ed02457b5c41d964dbd2f2a609d63fe1bb7528dbe55e1abf5b52c249cd735797"; d2 != want {
		t.Fatalf("wrong digest %q", d2)
	}
}
