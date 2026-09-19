package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Storefront is the TUI browser for the Ryoku store: one block per artifact
// (ISO, recovery script, rescue file), cursor- or click-selectable, with the
// canonical download URL always visible. It renders inside the chat column
// when the current room has type "store" — the store is ryolink's front
// page; the chat rooms are what you do after you've picked up your copy.
type Storefront struct {
	items     []StoreItemView
	cursor    int
	width     int
	height    int
	notices   []string // transient lines (URL copied, download hints)
	title     string   // store title
	rowOrigin int      // screen y of the first item row (set by parent)
	top       int      // index of first visible item (scroll window)
}

// StoreItemView is the presentation shape the ui layer receives from
// internal/shop (decoupled so ui never imports database code).
type StoreItemView struct {
	ID          string
	Name        string
	Kind        string // iso | script | file
	Description string
	Version     string
	URL         string // canonical public URL
	Size        int64
	SHA256      string
	Downloads   int
	Missing     bool
	Logo        string // ASCII art for the card (multi-line, may be empty)
}

func NewStorefront(title string) *Storefront {
	return &Storefront{title: title}
}

func (s *Storefront) SetItems(items []StoreItemView) {
	if s.cursor >= len(items) {
		s.cursor = max(0, len(items)-1)
	}
	s.items = items
}

func (s *Storefront) SetSize(w, h int) { s.width, s.height = w, h }

// SetOrigin records the screen row where the item list starts, so mouse
// clicks (absolute coordinates) map to rows.
func (s *Storefront) SetOrigin(top int) { s.rowOrigin = top }

// Selected returns the item under the cursor.
func (s *Storefront) Selected() (StoreItemView, bool) {
	if s.cursor < 0 || s.cursor >= len(s.items) {
		return StoreItemView{}, false
	}
	return s.items[s.cursor], true
}

// StoreCopyMsg asks the parent program to place a URL on the user's
// clipboard (tea.SetClipboard works over SSH via OSC 52).
type StoreCopyMsg struct{ URL string }

func (s *Storefront) Update(msg tea.Msg) (*Storefront, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		switch m.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.items)-1 {
				s.cursor++
			}
		case "home":
			s.cursor = 0
		case "end":
			s.cursor = max(0, len(s.items)-1)
		case "c", "y", "enter", " ":
			if it, ok := s.Selected(); ok && it.URL != "" {
				s.notice("link copied — paste in a browser, or: curl -LO " + it.URL)
				return s, func() tea.Msg { return StoreCopyMsg{URL: it.URL} }
			}
		}
	case tea.MouseWheelMsg:
		if m.Button == tea.MouseWheelUp && s.cursor > 0 {
			s.cursor--
		} else if m.Button == tea.MouseWheelDown && s.cursor < len(s.items)-1 {
			s.cursor++
		}
	case tea.MouseClickMsg:
		// Bento: clicks map through the same row geometry View drew with.
		// Click selects; click-again copies.
		if idx := s.itemAtClick(m.X, m.Y); idx >= 0 {
			if idx == s.cursor {
				if it, ok := s.Selected(); ok && it.URL != "" {
					s.notice("link copied — paste in a browser, or: curl -LO " + it.URL)
					return s, func() tea.Msg { return StoreCopyMsg{URL: it.URL} }
				}
			}
			s.cursor = idx
		}
	}
	return s, nil
}

func (s *Storefront) notice(msg string) {
	s.notices = append(s.notices, msg)
	if len(s.notices) > 2 {
		s.notices = s.notices[len(s.notices)-2:]
	}
}

// AddNotice surfaces a transient line under the list.
func (s *Storefront) AddNotice(msg string) { s.notice(msg) }

var kindGlyph = map[string]string{
	"iso":    "◆", // disc
	"script": "⚡", // rescue script
	"file":   "□",
}

func kindLabel(k string) string {
	switch k {
	case "iso":
		return "IMAGE"
	case "script":
		return "SCRIPT"
	default:
		return "FILE"
	}
}

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

// Bento geometry: two columns on wide terminals, one on narrow. Card height
// is fixed per row (the tallest card in the row sets it) so JoinHorizontal
// stays rectangular; View records the row tops so clicks map back to items.
const (
	bentoNameRows = 4 // name, desc, meta, fetch lines under the logo band
	bentoMaxLogo  = 3 // logo art is clipped to this many rows
)

func (s *Storefront) bentoCols() int {
	if s.width >= 112 {
		return 2
	}
	return 1
}

func (s *Storefront) bentoCardW() int {
	cols := s.bentoCols()
	w := (s.width - 4 - (cols-1)*2) / cols
	if w > 56 {
		w = 56 // keep cards from stretching into slabs on huge terminals
	}
	return w
}

// logoBand resolves a logo value into the card's art rows: a single-word
// logo becomes a 5-row block-letter wordmark, anything else is raw ASCII
// art clipped to bentoMaxLogo. Shared by cardHFor and renderCard so the
// geometry and the pixels can never disagree.
func logoBand(logo string) []string {
	lines := logoLines(logo)
	if len(lines) == 1 && IsWordmark(logo) {
		return Wordmark(logo)
	}
	if len(lines) > bentoMaxLogo {
		lines = lines[:bentoMaxLogo]
	}
	return lines
}

// cardHFor returns the rendered height of one item's card (border included).
func cardHFor(it StoreItemView) int {
	logo := 1
	if n := len(logoBand(it.Logo)); n > 0 {
		logo = n
	}
	return logo + bentoNameRows + 2 // +2: top/bottom border
}

func (s *Storefront) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(ColorAmber).Bold(true).Render("  " + s.title)
	sub := lipgloss.NewStyle().Foreground(ColorDim).Italic(true).Render("  ryoku: get it, rescue it, run it.")
	b.WriteString(title + "\n")
	b.WriteString(sub + "\n")
	b.WriteString(GradientBar(max(8, s.width-2), 7) + "\n\n")

	if len(s.items) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render("  the shelves are empty — the owner has not stocked ryolink.yaml yet."))
		return b.String()
	}

	perRow := s.bentoCols()
	cardW := s.bentoCardW()
	avail := max(1, s.height-4-len(s.notices))

	// Scroll by rows: the cursor's row must stay visible. Row heights vary
	// (logo bands differ), so budget screen rows, not item counts.
	rowOf := s.cursor / perRow
	firstRow := 0
	used := 0
	for r := 0; r*perRow < len(s.items) && r <= rowOf; r++ {
		used += s.rowHeight(r, perRow)
		for firstRow < r && used > avail {
			used -= s.rowHeight(firstRow, perRow)
			firstRow++
		}
	}
	s.top = firstRow * perRow
	lo := s.top

	// render rows until the height budget runs out
	used = 0
	r := firstRow
	for ; r*perRow < len(s.items); r++ {
		rh := s.rowHeight(r, perRow)
		if used+rh > avail {
			break
		}
		used += rh
		var cells []string
		for c := 0; c < perRow && r*perRow+c < len(s.items); c++ {
			cells = append(cells, s.renderCard(s.items[r*perRow+c], cardW, rh))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, joinGap(cells)...))
		b.WriteString("\n")
	}
	hi := r * perRow
	if hi > len(s.items) {
		hi = len(s.items)
	}
	if lo > 0 || hi < len(s.items) {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(
			fmt.Sprintf("  items %d–%d of %d", lo+1, hi, len(s.items))))
		b.WriteString("\n")
	}

	for _, n := range s.notices {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorGreen).Render("  ✓ "+n) + "\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render("  ↑↓ / click  ·  enter/c copy url"))
	return b.String()
}

// rowHeight is the screen height of grid row r (tallest card in it).
func (s *Storefront) rowHeight(r, perRow int) int {
	h := 0
	for c := 0; c < perRow && r*perRow+c < len(s.items); c++ {
		if ch := cardHFor(s.items[r*perRow+c]); ch > h {
			h = ch
		}
	}
	return h
}

// itemAtClick maps a click to the item whose card contains (x, y).
func (s *Storefront) itemAtClick(x, y int) int {
	rel := y - s.rowOrigin
	if rel < 0 {
		return -1
	}
	perRow := s.bentoCols()
	cardW := s.bentoCardW()
	r := s.top / perRow
	for ; r*perRow < len(s.items); r++ {
		rh := s.rowHeight(r, perRow)
		if rel < rh {
			col := 0
			for c := 1; c < perRow && r*perRow+c < len(s.items); c++ {
				if x >= c*(cardW+2) {
					col = c
				}
			}
			idx := r*perRow + col
			if idx < len(s.items) {
				return idx
			}
			return -1
		}
		rel -= rh
	}
	return -1
}

func joinGap(cells []string) []string {
	out := make([]string, 0, len(cells)*2-1)
	for i, c := range cells {
		if i > 0 {
			out = append(out, "  ")
		}
		out = append(out, c)
	}
	return out
}

// renderCard draws one bento cell at fixed w×h: logo art (or the kind
// glyph), name, description, meta, and both ways to fetch it — the curl line
// for the terminal, the bare URL for the browser.
func (s *Storefront) renderCard(it StoreItemView, w, h int) string {
	selected := it.ID == s.idAt(s.cursor)

	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDimmer).Padding(0, 1).Width(w).Height(h - 2)
	if selected {
		border = border.BorderForeground(ColorAmber)
	}

	inner := w - 4 // border (2) + padding (2)
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	var cb strings.Builder

	// --- logo band: wordmark or raw ASCII art (see logoBand) ---
	if lines := logoBand(it.Logo); len(lines) > 0 {
		art := lipgloss.NewStyle().Foreground(ColorCommand)
		if it.Missing {
			art = art.Foreground(ColorDimmer)
		}
		for _, ln := range lines {
			cb.WriteString(art.Render(trunc(ln, inner)) + "\n")
		}
	} else {
		glyph := kindGlyph[it.Kind]
		if glyph == "" {
			glyph = kindGlyph["file"]
		}
		cb.WriteString(lipgloss.NewStyle().Foreground(ColorAmber).Bold(true).Render(glyph) +
			" " + lipgloss.NewStyle().Foreground(ColorIndigo).Bold(true).Render(kindLabel(it.Kind)) + "\n")
	}

	// --- name + version ---
	name := it.Name
	if it.Version != "" {
		name += " " + it.Version
	}
	nameStyle := lipgloss.NewStyle().Foreground(ColorSand).Bold(true)
	if it.Missing {
		nameStyle = nameStyle.Foreground(ColorDimmer).Strikethrough(true)
	}
	cb.WriteString(nameStyle.Render(trunc(name, inner)) + "\n")

	// --- description ---
	cb.WriteString(dim.Render(trunc(it.Description, inner)) + "\n")

	// --- meta ---
	meta := fmt.Sprintf("%s · %d downloads", humanBytes(it.Size), it.Downloads)
	if it.Missing {
		meta = "not on disk — the operator must stage it"
	}
	cb.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(trunc(meta, inner)) + "\n")

	// --- both fetch paths ---
	if it.URL != "" {
		cb.WriteString(lipgloss.NewStyle().Foreground(ColorGreen).
			Render(trunc("curl -LO "+it.URL, inner)) + "\n")
	} else {
		cb.WriteString(dim.Render("ask the bartender") + "\n")
	}

	return border.Render(strings.TrimSuffix(cb.String(), "\n"))
}

func logoLines(logo string) []string {
	if logo == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(logo, "\n"), "\n")
}

func trunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func (s *Storefront) idAt(i int) string {
	if i < 0 || i >= len(s.items) {
		return ""
	}
	return s.items[i].ID
}
