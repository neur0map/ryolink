package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func namedKey(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return keyMsg(s)
}

func mouseClickAt(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func detailFixture() []StoreItemView {
	return []StoreItemView{
		{ID: "iso", Name: "Ryoku Linux", Kind: "iso", Version: "0.59",
			Description: "The live image.", URL: "https://dl/iso"},
		{ID: "rec", Name: "Recovery", Kind: "script",
			Description: "alpha " + strings.Repeat("word ", 300) + "omega",
			SHA256:      "abc123"},
		{ID: "shell", Name: "Shell install", Kind: "script", URL: "https://dl/shell"},
	}
}

// ENTER in the modal copies the link; the copy cmd carries the URL.
func TestStoreDetailEnterCopies(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 0)
	m, cmd := m.Update(namedKey("enter"))
	if cmd == nil {
		t.Fatal("enter on an item with a URL must issue a clipboard cmd")
	}
	msg := cmd()
	copyMsg, ok := msg.(StoreCopyMsg)
	if !ok || copyMsg.URL != "https://dl/iso" {
		t.Fatalf("copy msg = %#v, want StoreCopyMsg{https://dl/iso}", msg)
	}
	if m.notice == "" {
		t.Fatal("copy must surface a confirmation notice")
	}

	// an item without a URL must not emit a copy cmd
	m2 := NewStoreDetailModal(detailFixture(), 1)
	m2, cmd2 := m2.Update(namedKey("enter"))
	if cmd2 != nil {
		t.Fatal("enter on a URL-less item must not emit a copy cmd")
	}
	if m2.notice == "" {
		t.Fatal("the no-link case must still explain itself")
	}
}

// All four arrows (and hjkl) walk the catalogue; the card pages with
// pgup/pgdown/space.
func TestStoreDetailFourWayBrowse(t *testing.T) {
	next := []tea.KeyPressMsg{keyMsg("j"), namedKey("right"), namedKey("down")}
	prev := []tea.KeyPressMsg{keyMsg("k"), namedKey("left"), namedKey("up")}
	for _, key := range next {
		m := NewStoreDetailModal(detailFixture(), 0)
		m, _ = m.Update(key)
		if it, _ := m.current(); it.ID != "rec" {
			t.Fatalf("%q must walk to the next product, got %q", key.String(), it.ID)
		}
	}
	for _, key := range prev {
		m := NewStoreDetailModal(detailFixture(), 0)
		m, _ = m.Update(key) // wraps to the end
		if it, _ := m.current(); it.ID != "shell" {
			t.Fatalf("%q must walk to the previous product, got %q", key.String(), it.ID)
		}
	}

	// pgdown pages the text and must NOT change the product
	m := NewStoreDetailModal(detailFixture(), 1)
	_ = m.View(80, 32) // establishes textH/nText
	if m.nText <= m.textH {
		t.Fatalf("fixture must scroll: nText=%d textH=%d", m.nText, m.textH)
	}
	m, _ = m.Update(namedKey("pgdown"))
	if it, _ := m.current(); it.ID != "rec" {
		t.Fatalf("pgdown must not change the product, got %q", it.ID)
	}
	if m.scroll == 0 {
		t.Fatal("pgdown on a scrollable card must advance the window")
	}
	// space pages too
	m, _ = m.Update(namedKey("home"))
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if m.scroll == 0 {
		t.Fatal("space must page the card down")
	}
}

// The status strip only appears when something is hidden below (or above).
func TestStoreDetailStatusLine(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 1)
	_ = m.View(80, 32)
	if m.statusLine() == "" {
		t.Fatal("a scrollable card must show a 'N below' affordance")
	}
	m.scroll = m.nText - m.textH // bottom
	v := stripANSIForTest(m.View(80, 32))
	if !strings.Contains(v, "above") {
		t.Fatal("scrolled to the bottom must report how much is above")
	}
}

// ←/→ walk the shelf and wrap; x copies the sha when present.
func TestStoreDetailNavWraps(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 0)
	m, _ = m.Update(namedKey("left")) // wrap to the end
	if it, _ := m.current(); it.ID != "shell" {
		t.Fatalf("left from first must wrap to last, got %q", it.ID)
	}
	m, _ = m.Update(namedKey("right")) // wrap back around
	if it, _ := m.current(); it.ID != "iso" {
		t.Fatalf("right from last must wrap to first, got %q", it.ID)
	}

	m, cmd := m.Update(keyMsg("x")) // no sha on item 0
	if cmd != nil || m.notice == "" {
		t.Fatalf("x without a checksum: cmd=%v notice=%q", cmd, m.notice)
	}
	m = NewStoreDetailModal(detailFixture(), 1)
	m, cmd = m.Update(keyMsg("x"))
	msg := cmd()
	if cm, ok := msg.(StoreCopyMsg); !ok || cm.URL != "abc123" {
		t.Fatalf("x must copy the sha, got %#v", msg)
	}
}

// A live catalog that reindexes (download counts move) must keep the modal
// on the same product, not snap to index 0.
func TestStoreDetailSetItemsStaysOnProduct(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 1)
	updated := detailFixture()
	updated[0].Downloads = 5
	updated[1].Downloads = 9
	m.SetItems(updated)
	if it, _ := m.current(); it.ID != "rec" || it.Downloads != 9 {
		t.Fatalf("SetItems lost the product: %#v", it)
	}
}

// Scrolling: a long description must be clamped to the window, and `end`
// must park at the last page rather than overshoot into blank lines.
func TestStoreDetailScrollClamps(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 1)
	m.w, m.h = 80, 32
	_ = m.View(80, 32) // establishes textH/nText
	if m.nText <= m.textH {
		t.Fatalf("fixture must produce more lines than fit: nText=%d textH=%d", m.nText, m.textH)
	}
	m, _ = m.Update(namedKey("end"))
	if max := m.nText - m.textH; m.scroll != max {
		t.Fatalf("end must clamp to %d, got %d", max, m.scroll)
	}
	m, _ = m.Update(namedKey("home"))
	if m.scroll != 0 {
		t.Fatalf("home must reset to 0, got %d", m.scroll)
	}
	m, _ = m.Update(namedKey("pgup")) // already at the top: must not go negative
	if m.scroll != 0 {
		t.Fatalf("pgup at top underflowed: %d", m.scroll)
	}
	// navigation resets the read position — a fresh card starts at its top
	m, _ = m.Update(namedKey("end"))
	m, _ = m.Update(namedKey("right"))
	if m.scroll != 0 {
		t.Fatalf("moving to the next product must reset scroll, got %d", m.scroll)
	}
}

// The '/' search inside the card commits a shelf filter to the App.
func TestStoreDetailSearchEmitsFilter(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 0)
	m.View(80, 32) // sets bodyW for the input
	m, _ = m.Update(keyMsg("/"))
	if !m.search {
		t.Fatal("'/' must open the search field")
	}
	for _, c := range "iso" {
		m, _ = m.Update(keyMsg(string(c)))
	}
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.search {
		t.Fatal("enter must commit and close the search field")
	}
	msg := cmd()
	fm, ok := msg.(StoreFilterMsg)
	if !ok || fm.Query != "iso" {
		t.Fatalf("want StoreFilterMsg{iso}, got %#v", msg)
	}

	// esc while typing abandons the search without touching the shelf
	m, _ = m.Update(keyMsg("/"))
	m, cmd = m.Update(namedKey("esc"))
	if m.search {
		t.Fatal("esc must leave search")
	}
	if cmd != nil {
		t.Fatal("abandoned search must not emit a filter msg")
	}
}

// The rendered card must never truncate the description — that was the
// whole complaint. Every word of the long description appears somewhere
// across the scroll positions.
func TestStoreDetailShowsWholeDescription(t *testing.T) {
	m := NewStoreDetailModal(detailFixture(), 1)
	_ = m.View(80, 32) // establishes textH/nText
	if m.nText <= m.textH {
		t.Fatalf("fixture must scroll: nText=%d textH=%d", m.nText, m.textH)
	}
	// walk every scroll window; the first and last words must both surface
	var first, last bool
	for pos := 0; pos <= m.nText; pos++ {
		m.scroll = pos
		v := stripANSIForTest(m.View(80, 32))
		if !first && strings.Contains(v, "alpha") {
			first = true
		}
		if first && strings.Contains(v, "omega") {
			last = true
			break
		}
	}
	if !first || !last {
		t.Fatalf("description truncated: first=%v last=%v", first, last)
	}
}

func stripANSIForTest(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ── grid-side filter/search ──────────────────────────────────────────────

// The grid's filter narrows `visible`, keeps shelf order, and the cursor
// clamps so a stale index can't select a hidden item.
func TestStorefrontFilterVisibleAndCursor(t *testing.T) {
	s := NewStorefront("t")
	s.SetItems(detailFixture())
	s.SetSize(60, 30)

	if len(s.visible()) != 3 {
		t.Fatalf("no filter must show everything, got %d", len(s.visible()))
	}
	s.SetFilter("script")
	if got := s.visible(); len(got) != 2 || got[0].ID != "rec" || got[1].ID != "shell" {
		t.Fatalf("kind filter mismatch: %#v", got)
	}
	s.cursor = 5 // stale index from before the filter
	s.SetItems(detailFixture())
	if s.cursor != 1 { // last visible index
		t.Fatalf("cursor must clamp into the filtered list, got %d", s.cursor)
	}
	if s.VisibleItems()[s.cursor].ID != "shell" {
		t.Fatal("clamped cursor must land on a visible item")
	}
	s.SetFilter("")
	if len(s.visible()) != 3 {
		t.Fatal("clearing the filter must restore the shelf")
	}
}

// Typing "/" opens the search bar; enter commits; esc clears the filter
// chip (the chip literally says "esc clears" — the promise must hold).
func TestStorefrontSearchKeys(t *testing.T) {
	s := NewStorefront("t")
	s.SetItems(detailFixture())
	s.SetSize(60, 30)

	s, _ = s.Update(keyMsg("/"))
	if !s.search {
		t.Fatal("slash must open the search bar")
	}
	s, _ = s.Update(namedKey("enter")) // empty query → no filter, back to browse
	if s.search || s.Filter() != "" {
		t.Fatalf("empty enter must leave browse mode: search=%v filter=%q", s.search, s.Filter())
	}

	s, _ = s.Update(keyMsg("/"))
	for _, c := range "iso" {
		s, _ = s.Update(keyMsg(string(c)))
	}
	s, _ = s.Update(namedKey("enter"))
	if s.Filter() != "iso" {
		t.Fatalf("enter must commit the query, filter=%q", s.Filter())
	}
	if len(s.visible()) != 1 || s.visible()[0].ID != "iso" {
		t.Fatalf("query 'iso' must leave 1 item, got %#v", s.visible())
	}
	s, _ = s.Update(namedKey("esc"))
	if s.Filter() != "" {
		t.Fatalf("esc must clear the filter chip, filter=%q", s.Filter())
	}
}

// ENTER on the grid opens the product card instead of copying (the copy
// keys keep working — c stays copy).
func TestStorefrontEnterOpensDetail(t *testing.T) {
	s := NewStorefront("t")
	s.SetItems(detailFixture())
	s.SetSize(60, 30)

	s, cmd := s.Update(namedKey("enter"))
	if cmd == nil {
		t.Fatal("enter must emit a command")
	}
	if _, ok := cmd().(StoreOpenDetailMsg); !ok {
		t.Fatalf("enter must open the detail modal, got %T", cmd())
	}
	_, cmd = s.Update(keyMsg("c"))
	if cmd == nil {
		t.Fatal("c must still copy")
	}
	if _, ok := cmd().(StoreCopyMsg); !ok {
		t.Fatalf("c must emit StoreCopyMsg, got %T", cmd())
	}
}

// Clicking the already-selected card opens the card too (click-select,
// click-again-open, matching the mouse model everywhere else).
func TestStorefrontDoubleClickOpensDetail(t *testing.T) {
	s := NewStorefront("t")
	s.SetItems(detailFixture())
	s.SetSize(60, 30) // narrow: one column
	s.SetOrigin(4)
	s.View() // establishes scroll window

	y := s.rowOrigin + 1
	if _, cmd := s.Update(mouseClickAt(4, y)); cmd == nil {
		t.Fatal("first click must select (no cmd is fine)")
	}
	// cursor now on item 0 (top row); clicking the same row again opens
	_, cmd := s.Update(mouseClickAt(4, y))
	if cmd == nil {
		t.Fatal("clicking the selected card must emit a cmd")
	}
	if _, ok := cmd().(StoreOpenDetailMsg); !ok {
		t.Fatalf("second click must open the detail, got %T", cmd())
	}
}
