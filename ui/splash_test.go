package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// The splash card must be a rigid block: every line is either blank (section
// gap) or exactly cardW wide. The old renderer left-padded per line, so lines
// of different widths drifted against the gradient rules.
func TestSplashCardLinesAlign(t *testing.T) {
	s := NewSplash("dusty_brewer#4722", "SHA256:abcd", true, "ryoku.dev", "")
	s.width, s.height = 100, 40
	for frame := 0; frame <= revealTicks+5; frame++ {
		s.frame = frame
		card := s.renderCard()
		for i, line := range strings.Split(card, "\n") {
			if line == "" {
				continue
			}
			if w := lipgloss.Width(line); w != cardW {
				t.Fatalf("frame %d line %d: width %d, want %d (%q)", frame, i, w, cardW, line)
			}
		}
	}
}

// The banner is the installer wordmark at 2x scale: five source rows become
// ten, and it must fit inside the card.
func TestSplashBannerScaled(t *testing.T) {
	rows := revealBanner(1, 0, 2)
	if len(rows) != 10 {
		t.Fatalf("scaled banner rows = %d, want 10", len(rows))
	}
	for _, r := range rows {
		if w := lipgloss.Width(r); w > cardW {
			t.Fatalf("banner row %d wide exceeds cardW %d", w, cardW)
		}
	}
}
