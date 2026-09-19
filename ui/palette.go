package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PaletteRunMsg asks the App to run the command with this id. The palette
// itself knows nothing about what the commands do — it only filters and
// picks, so it can list everything without importing half the app.
type PaletteRunMsg struct{ ID string }

type paletteEntry struct {
	ID   string
	Name string
	Key  string // the direct keybind, shown as a hint
	Desc string
}

// PaletteModal is the ctrl+p command palette: type to filter, enter to run.
// Everything the F-keys do is reachable from here, which is what makes the
// keybinds discoverable without reading a help screen first.
type PaletteModal struct {
	input   textinput.Model
	entries []paletteEntry
	cursor  int
}

func NewPaletteModal() PaletteModal {
	ti := textinput.New()
	ti.Placeholder = "type a command..."
	ti.Focus()
	ti.Prompt = "› "
	return PaletteModal{input: ti, entries: paletteCommands()}
}

// paletteCommands is the full command list. Order is the order shown when
// the filter is empty: navigation first, then rooms, then the extras.
func paletteCommands() []paletteEntry {
	return []paletteEntry{
		{"rooms", "Rooms", "ctrl+r", "join or switch rooms"},
		{"store", "Store", "", "open the ryoku store"},
		{"nick", "Nickname", "f2", "change your nickname"},
		{"mentions", "Mentions", "f4", "jump to your recent mentions"},
		{"post", "Post a note", "f5", "leave a sticky in the gallery"},
		{"tankard", "Tankard", "f6", "focus the beer meter"},
		{"leaderboard", "Leaderboard", "f7", "wargame points"},
		{"dm", "Direct messages", "tab", "open your DM list"},
		{"feed", "Feed", "shift+tab", "toggle the reddit feed (lounge)"},
		{"gif", "Send a GIF", "", "search and send an animated gif"},
		{"cursor", "Cursor mode", "ctrl+m", "let the mouse drive the app"},
		{"help", "Help", "f1", "all the keybinds"},
		{"changelog", "Changelog", "", "what changed this week"},
		{"quit", "Leave", "q", "disconnect"},
	}
}

func (p *PaletteModal) filtered() []paletteEntry {
	q := strings.ToLower(strings.TrimSpace(p.input.Value()))
	if q == "" {
		return p.entries
	}
	var out []paletteEntry
	for _, e := range p.entries {
		hay := strings.ToLower(e.Name + " " + e.Desc + " " + e.ID)
		// subsequence match: "lb" finds Leaderboard, "st" finds Store
		i := 0
		for _, c := range hay {
			if i < len(q) && byte(c) == q[i] {
				i++
			}
		}
		if i == len(q) {
			out = append(out, e)
		}
	}
	return out
}

func (p PaletteModal) Update(msg tea.Msg) (PaletteModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		f := p.filtered()
		switch msg.String() {
		case "esc":
			return p, func() tea.Msg { return CloseModalMsg{} }
		case "enter":
			if len(f) > 0 && p.cursor < len(f) {
				id := f[p.cursor].ID
				return p, func() tea.Msg { return PaletteRunMsg{ID: id} }
			}
			return p, nil
		case "up", "ctrl+k":
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case "down", "ctrl+j", "tab":
			if p.cursor < len(f)-1 {
				p.cursor++
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if _, ok := msg.(tea.KeyPressMsg); ok {
		p.cursor = 0 // filter changed; keep the selection on the first hit
	}
	return p, cmd
}

func (p PaletteModal) View(width, height int) string {
	f := p.filtered()
	if p.cursor >= len(f) {
		p.cursor = max(0, len(f)-1)
	}

	// window the list around the cursor
	visible := 9
	lo := 0
	if p.cursor >= visible {
		lo = p.cursor - visible + 1
	}
	hi := min(lo+visible, len(f))

	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(ColorAmber).Bold(true).Render(" Commands ")
	b.WriteString(header + "\n")
	b.WriteString(p.input.View() + "\n\n")

	if len(f) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Italic(true).
			Render("  nothing matches that") + "\n")
	}
	for i := lo; i < hi; i++ {
		e := f[i]
		name := lipgloss.NewStyle().Foreground(ColorSand).Bold(i == p.cursor).Render(e.Name)
		desc := lipgloss.NewStyle().Foreground(ColorDim).Render(e.Desc)
		key := lipgloss.NewStyle().Foreground(ColorDimmer).Render(e.Key)
		line := "  " + name
		if pad := 16 - lipgloss.Width(e.Name); pad > 0 {
			line += strings.Repeat(" ", pad)
		} else {
			line += " "
		}
		line += desc
		if e.Key != "" {
			if gap := 40 - lipgloss.Width(line); gap > 0 {
				line += strings.Repeat(" ", gap)
			}
			line += " " + key
		}
		if i == p.cursor {
			line = lipgloss.NewStyle().Foreground(ColorAmber).Render("▸ ") + line[2:]
		}
		b.WriteString(line + "\n")
	}
	if len(f) > visible {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(
			"  "+"↑↓ more") + "\n")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAmber).
		Padding(1, 2).
		Width(56).
		Render(strings.TrimSuffix(b.String(), "\n"))
	return box
}
