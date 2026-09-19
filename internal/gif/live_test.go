package gif

import (
	"os"
	"testing"
)

func TestKlipyLiveSearch(t *testing.T) {
	key := os.Getenv("KLIPY_API_KEY")
	if key == "" {
		t.Skip("no KLIPY_API_KEY")
	}
	c := NewKlipyClient(key)
	results, err := c.Search("cat")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("0 results for 'cat'")
	}
	r := results[0]
	t.Logf("%d results; first: %q %s %dx%d", len(results), r.Title, r.URL, r.Width, r.Height)
	if r.URL == "" {
		t.Fatal("first result has no url")
	}
}
