package ui

import (
	"strings"
	"testing"
)

// The bento grid has variable row heights (logo bands differ), and clicks
// map through the same geometry View drew with. If the two drift, clicks
// land on the wrong card — the bug the old fixed-3-line assumption hid.
func TestStorefrontClicksLandOnOwnCard(t *testing.T) {
	items := []StoreItemView{
		{ID: "a", Name: "Ryoku Linux", Kind: "iso", Logo: strings.Repeat("-`\n", 6), URL: "https://x/a"},
		{ID: "b", Name: "Recovery", Kind: "script", URL: "https://x/b"}, // no logo: short card
		{ID: "c", Name: "CachyOS", Kind: "iso", Logo: strings.Repeat("=++=\n", 6), URL: "https://x/c"},
		{ID: "d", Name: "Shell install", Kind: "script", URL: "https://x/d"},
	}
	s := NewStorefront("Ryoku Store")
	s.SetItems(items)
	s.SetSize(130, 40) // wide: two columns
	s.SetOrigin(5)

	s.View() // establishes s.top via the scroll window

	perRow := s.bentoCols()
	if perRow != 2 {
		t.Fatalf("expected 2 columns at width 130, got %d", perRow)
	}
	cardW := s.bentoCardW()

	// Every item must be reachable: some cell inside its card maps back to it.
	for idx := range items {
		row := idx / perRow
		col := idx % perRow
		relY := 1 // inside the border
		baseY := s.rowOrigin
		for r := s.top / perRow; r < row; r++ {
			baseY += s.rowHeight(r, perRow)
		}
		x := col*(cardW+2) + cardW/2
		if got := s.itemAtClick(x, baseY+relY); got != idx {
			t.Fatalf("click at card %d (x=%d y=%d) mapped to item %d", idx, x, baseY+relY, got)
		}
	}

	// Clicks below the grid map to nothing, not to a stale item.
	if got := s.itemAtClick(2, s.rowOrigin+200); got != -1 {
		t.Fatalf("click far below grid = %d, want -1", got)
	}
}

// Scrolling: moving the cursor to the last item must keep its row visible.
func TestStorefrontCursorRowStaysVisible(t *testing.T) {
	items := make([]StoreItemView, 12)
	for i := range items {
		items[i] = StoreItemView{ID: string(rune('a' + i)), Name: "item", Kind: "file",
			Logo: strings.Repeat("x\n", 3)}
	}
	s := NewStorefront("t")
	s.SetItems(items)
	s.SetSize(60, 20) // narrow: one column, short viewport
	s.SetOrigin(4)

	s.cursor = len(items) - 1
	s.View()
	if s.top > len(items)-1 {
		t.Fatalf("scrolled past the end: top=%d", s.top)
	}
	// the cursor's row must be on screen: row height 3+4+2=9, viewport ~14 rows
	// → at most 1 row visible; top must equal the cursor's index.
	if s.top != len(items)-1 {
		t.Fatalf("cursor row not visible: top=%d, want %d", s.top, len(items)-1)
	}
}
