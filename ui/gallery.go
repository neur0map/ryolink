package ui

import (
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ryolink/internal/store"
)

type GalleryNote struct {
	ID          int
	X, Y        int
	Text        string
	Nickname    string
	Fingerprint string
	ColorIndex  int
}

type GalleryView struct {
	notes       []GalleryNote
	selected    int
	dragging    bool
	dragOffsetX int
	dragOffsetY int
	width       int
	height      int
	// Screen offset: where the gallery area starts on the full terminal
	screenOffX  int
	screenOffY  int
	fingerprint string
	rng         *rand.Rand
}

func NewGalleryView(fingerprint string) GalleryView {
	return GalleryView{
		notes:       make([]GalleryNote, 0),
		selected:    -1,
		fingerprint: fingerprint,
		rng:         rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()^0x5bd1e995))),
	}
}

func (g *GalleryView) SetSize(w, h int)         { g.width = w; g.height = h }
func (g *GalleryView) SetScreenOffset(x, y int) { g.screenOffX = x; g.screenOffY = y }
func (g *GalleryView) AddNote(n GalleryNote)    { g.notes = append(g.notes, n) }
func (g *GalleryView) ClearAll()                { g.notes = nil; g.selected = -1 }

func (g *GalleryView) LoadNotes(rows []store.NoteRow) {
	g.notes = make([]GalleryNote, len(rows))
	for i, r := range rows {
		g.notes[i] = GalleryNote{
			ID: r.ID, X: r.X, Y: r.Y, Text: r.Text,
			Nickname: r.Nickname, Fingerprint: r.Fingerprint, ColorIndex: r.ColorIndex,
		}
	}
}

func (g *GalleryView) MoveNote(id, x, y int) {
	for i := range g.notes {
		if g.notes[i].ID == id {
			g.notes[i].X = x
			g.notes[i].Y = y
			return
		}
	}
}

func (g *GalleryView) RemoveNote(id int) {
	for i := range g.notes {
		if g.notes[i].ID == id {
			g.notes = append(g.notes[:i], g.notes[i+1:]...)
			if g.selected >= len(g.notes) {
				g.selected = -1
			}
			return
		}
	}
}

func (g *GalleryView) RandomPosition() (int, int) {
	maxX := g.width - 30
	maxY := g.height - 10
	if maxX < 2 {
		maxX = 2
	}
	if maxY < 2 {
		maxY = 2
	}
	return 2 + g.rng.IntN(maxX), 1 + g.rng.IntN(maxY)
}

// toLocal converts absolute screen mouse coordinates to gallery-local coordinates.
func (g GalleryView) toLocal(screenX, screenY int) (int, int) {
	return screenX - g.screenOffX, screenY - g.screenOffY
}

func (g GalleryView) Update(msg tea.Msg) (GalleryView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "d", "delete", "backspace":
			if g.selected >= 0 && g.selected < len(g.notes) {
				note := g.notes[g.selected]
				if note.Fingerprint == g.fingerprint {
					return g, func() tea.Msg {
						return GalleryDeleteMsg{NoteID: note.ID}
					}
				}
			}
		case "e":
			if g.selected >= 0 && g.selected < len(g.notes) {
				note := g.notes[g.selected]
				return g, func() tea.Msg {
					return GalleryExpandMsg{Note: note}
				}
			}
		case "tab":
			if len(g.notes) > 0 {
				g.selected = (g.selected + 1) % len(g.notes)
			}
		}

	case tea.MouseClickMsg:
		lx, ly := g.toLocal(msg.Mouse().X, msg.Mouse().Y)
		hit := g.hitTest(lx, ly)
		if hit >= 0 {
			g.selected = hit
			g.dragging = true
			g.dragOffsetX = lx - g.notes[hit].X
			g.dragOffsetY = ly - g.notes[hit].Y
			// Bring to front by moving to end of slice
			note := g.notes[hit]
			g.notes = append(g.notes[:hit], g.notes[hit+1:]...)
			g.notes = append(g.notes, note)
			g.selected = len(g.notes) - 1
		} else {
			g.selected = -1
		}

	case tea.MouseReleaseMsg:
		if g.dragging && g.selected >= 0 && g.selected < len(g.notes) {
			note := g.notes[g.selected]
			g.dragging = false
			if note.Fingerprint == g.fingerprint {
				return g, func() tea.Msg {
					return GalleryMoveMsg{NoteID: note.ID, X: note.X, Y: note.Y}
				}
			}
		}
		g.dragging = false

	case tea.MouseMotionMsg:
		if g.dragging && g.selected >= 0 && g.selected < len(g.notes) {
			lx, ly := g.toLocal(msg.Mouse().X, msg.Mouse().Y)
			newX := lx - g.dragOffsetX
			newY := ly - g.dragOffsetY
			// Clamp to bounds
			if newX < 0 {
				newX = 0
			}
			if newY < 0 {
				newY = 0
			}
			maxX := g.width - 10
			maxY := g.height - 4
			if newX > maxX {
				newX = maxX
			}
			if newY > maxY {
				newY = maxY
			}
			g.notes[g.selected].X = newX
			g.notes[g.selected].Y = newY
		}
	}

	return g, nil
}

func (g GalleryView) hitTest(lx, ly int) int {
	for i := len(g.notes) - 1; i >= 0; i-- {
		n := g.notes[i]
		w, h := g.noteSize(n.Text)
		if lx >= n.X && lx < n.X+w && ly >= n.Y && ly < n.Y+h {
			return i
		}
	}
	return -1
}

func (g GalleryView) View() string {
	if g.width <= 0 || g.height <= 0 {
		return ""
	}

	// Use Lipgloss Canvas + Compositor
	// 1. Background layer with hatch pattern
	bgContent := g.renderBackground()
	bgLayer := lipgloss.NewLayer(bgContent)

	// 2. Note layers
	var noteLayers []*lipgloss.Layer
	for idx, note := range g.notes {
		isSelected := idx == g.selected
		isOwn := note.Fingerprint == g.fingerprint
		cardContent := g.renderCard(note, isSelected, isOwn)
		layer := lipgloss.NewLayer(cardContent).
			X(note.X).
			Y(note.Y).
			Z(idx + 1) // z > 0 so they're above background
		noteLayers = append(noteLayers, layer)
	}

	// 3. Compose
	allLayers := append([]*lipgloss.Layer{bgLayer}, noteLayers...)
	comp := lipgloss.NewCompositor(allLayers...)
	return comp.Render()
}

func (g GalleryView) renderBackground() string {
	var lines []string
	for y := 0; y < g.height; y++ {
		var b strings.Builder
		for x := 0; x < g.width; x++ {
			switch (x + y) % 6 {
			case 0:
				b.WriteRune('╲')
			case 3:
				b.WriteRune('╱')
			default:
				b.WriteRune(' ')
			}
		}
		lines = append(lines, b.String())
	}
	bg := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Foreground(lipgloss.Color("236")).Render(bg)
}

func (g GalleryView) renderCard(note GalleryNote, selected, isOwn bool) string {
	cardWidth := 28
	nickDisplay := note.Nickname
	if isOwn {
		nickDisplay = "~" + nickDisplay
	}

	borderColor := NickColors[note.ColorIndex%len(NickColors)]

	style := lipgloss.NewStyle().
		Width(cardWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Foreground(ColorSand)

	if selected {
		style = style.
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorHighlight)
	}

	// Content: text + separator + nick + expand button
	text := note.Text
	// Truncate display text for card
	displayText := text
	truncated := false
	if len(displayText) > 40 {
		displayText = displayText[:37] + "..."
		truncated = true
	}

	nickLine := lipgloss.NewStyle().
		Foreground(NickColors[note.ColorIndex%len(NickColors)]).
		Bold(true).
		Render(nickDisplay)

	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color("238")).
		Render(strings.Repeat("─", cardWidth-6))

	expandBtn := ""
	if selected || truncated {
		expandBtn = "\n" + lipgloss.NewStyle().
			Foreground(ColorHighlight).
			Render("[ E expand ]")
	}

	content := displayText + "\n" + sep + "\n" + nickLine + expandBtn

	return style.Render(content)
}

func (g GalleryView) noteSize(text string) (int, int) {
	// Card: 28 width + 2 border = 30, content lines + 2 border + 2 padding = variable
	w := 30
	lines := strings.Count(text, "\n") + 1
	h := lines + 5 // border*2 + padding*2 + separator + nick
	return w, h
}

type GalleryDeleteMsg struct{ NoteID int }
type GalleryMoveMsg struct{ NoteID, X, Y int }
type GalleryExpandMsg struct{ Note GalleryNote }
