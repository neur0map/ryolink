package ui

import (
	"strings"
	"testing"
)

func TestWordmarkRendering(t *testing.T) {
	if !IsWordmark("ARCH") || !IsWordmark("cachy") {
		t.Fatal("plain words must be wordmarks")
	}
	if IsWordmark("力") || IsWordmark("a\nb") || IsWordmark("ssh://x/y") {
		t.Fatal("non-word logos must stay raw art")
	}

	rows := Wordmark("ARCH")
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	w := len([]rune(rows[0]))
	for i, r := range rows {
		if len([]rune(r)) != w {
			t.Fatalf("row %d width %d, want %d — ragged wordmark", i, len([]rune(r)), w)
		}
	}
	// lowercase input renders identically
	if strings.Join(Wordmark("arch"), "\n") != strings.Join(rows, "\n") {
		t.Fatal("lowercase input must render the same wordmark")
	}
}

// A wordmark logo must grow the card to fit all five rows — the clipping
// that made the old art look chopped.
func TestWordmarkCardFitsAllRows(t *testing.T) {
	it := StoreItemView{ID: "x", Name: "Ryoku Linux", Kind: "iso", Logo: "ARCH"}
	if h := cardHFor(it); h != 5+bentoNameRows+2 {
		t.Fatalf("card height = %d, want wordmark (5 rows) + text + borders", h)
	}
	s := NewStorefront("t")
	s.SetItems([]StoreItemView{it})
	s.SetSize(60, 30)
	view := s.View()
	if strings.Count(view, "█") < 20 {
		t.Fatal("wordmark blocks missing from rendered card")
	}
}
