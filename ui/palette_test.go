package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyMsg(s string) tea.KeyPressMsg {
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func TestPaletteFilterAndRun(t *testing.T) {
	p := NewPaletteModal()

	// "lb" is a subsequence of "leaderboard" — the point of fuzzy filtering.
	p, _ = p.Update(keyMsg("l"))
	p, _ = p.Update(keyMsg("b"))
	f := p.filtered()
	if len(f) == 0 || f[0].ID != "leaderboard" {
		ids := make([]string, len(f))
		for i, e := range f {
			ids[i] = e.ID
		}
		t.Fatalf("filter 'lb' = %v, want leaderboard first", ids)
	}

	// Enter runs the highlighted entry via PaletteRunMsg.
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	msg := cmd()
	run, ok := msg.(PaletteRunMsg)
	if !ok || run.ID != "leaderboard" {
		t.Fatalf("enter emitted %#v, want PaletteRunMsg{leaderboard}", msg)
	}

	// Typing narrows the list and must reset the cursor so it can't point
	// past the end of a shorter result.
	p.cursor = 10
	p, _ = p.Update(keyMsg("q"))
	if p.cursor != 0 {
		t.Fatalf("cursor after filter change = %d, want 0", p.cursor)
	}
}
