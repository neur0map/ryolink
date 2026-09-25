package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// StoreOpenDetailMsg asks the App to open the detail modal for the item
// under the storefront cursor. The storefront never renders the modal
// itself — modals belong to the App overlay, so the dim-and-centre path is
// shared with every other dialog.
type StoreOpenDetailMsg struct{}

// StoreFilterMsg asks the App to apply a shelf filter to the storefront
// grid (emitted from the modal's '/' search).
type StoreFilterMsg struct{ Query string }

// StoreDetailModal is the full product card: it takes most of the screen
// and lays the product out with room to breathe — title, art, fetch
// block, then the whole description (wrapped, paged, never truncated).
// All four arrows (and hjkl) walk the catalogue; pgup/pgdn/space/wheel
// page through a long card. The key legend is fixed chrome; copy results
// land on their own notice line, so the legend never disappears.
type StoreDetailModal struct {
	items  []StoreItemView
	idx    int
	scroll int // scrolled body lines
	notice string

	input   textinput.Model // '/' search overlay
	search  bool
	pending string // query committed on enter (read by the App)

	w, h   int // terminal size (scroll budget + placement)
	bodyW  int // inner text width
	textH  int // wrapped body lines that fit
	nText  int // total body lines
	navRow int // absolute screen row of the ◂ prev / next ▸ strip
}

func NewStoreDetailModal(items []StoreItemView, idx int) StoreDetailModal {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = max(0, len(items)-1)
	}
	ti := textinput.New()
	ti.Placeholder = "filter the shelves..."
	ti.Prompt = "/ "
	return StoreDetailModal{items: items, idx: idx, input: ti}
}

// SetItems refreshes the catalogue (download counts and staged-file state
// move under a live modal) and stays on the same product.
func (m *StoreDetailModal) SetItems(items []StoreItemView) {
	if m.idx < len(m.items) {
		id := m.items[m.idx].ID
		for i, it := range items {
			if it.ID == id {
				m.idx = i
				break
			}
		}
	}
	m.items = items
	if m.idx >= len(items) {
		m.idx = max(0, len(items)-1)
	}
}

func (m StoreDetailModal) current() (StoreItemView, bool) {
	if m.idx < 0 || m.idx >= len(m.items) {
		return StoreItemView{}, false
	}
	return m.items[m.idx], true
}

// move steps to a neighbouring product and resets the read position.
func (m *StoreDetailModal) move(d int) {
	if n := len(m.items); n > 0 {
		m.idx = (m.idx + d + n) % n
		m.scroll = 0
		m.notice = ""
	}
}

func (m *StoreDetailModal) scrollBy(d int) {
	m.scroll = clampInt(m.scroll+d, 0, max(0, m.nText-m.textH))
}

func (m StoreDetailModal) Update(msg tea.Msg) (StoreDetailModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tea.MouseWheelMsg:
		if p := lastStoreModal; p.inside(msg.X, msg.Y) {
			if msg.Button == tea.MouseWheelUp {
				m.scrollBy(-3)
			} else {
				m.scrollBy(3)
			}
		} else {
			// wheel outside the card = walk the shelf
			if msg.Button == tea.MouseWheelUp {
				m.move(-1)
			} else {
				m.move(1)
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if lastStoreModal.inside(msg.X, msg.Y) && msg.Y == m.navRow {
			if msg.X < lastStoreModal.x+lastStoreModal.w/2 {
				m.move(-1)
			} else {
				m.move(1)
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.search {
			switch msg.String() {
			case "esc":
				m.search = false
				m.input.Blur()
				return m, nil
			case "enter":
				m.pending = strings.TrimSpace(m.input.Value())
				m.search = false
				m.input.Blur()
				q := m.pending
				return m, func() tea.Msg { return StoreFilterMsg{Query: q} }
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return CloseModalMsg{} }
		// four-way browse: every arrow (and hjkl) walks the catalogue
		case "left", "up", "h", "k":
			m.move(-1)
		case "right", "down", "l", "j":
			m.move(1)
		// paging the card itself
		case "pgup":
			m.scrollBy(-m.pageStep())
		case "pgdown", "space":
			m.scrollBy(m.pageStep())
		case "home":
			m.scroll = 0
		case "end":
			m.scrollBy(1 << 30)
		case "enter", "c", "y":
			if it, ok := m.current(); ok && it.URL != "" {
				m.notice = "link copied — paste in a browser, or: curl -LO " + it.URL
				return m, func() tea.Msg { return StoreCopyMsg{URL: it.URL} }
			}
			m.notice = "this item has no public link yet"
		case "u":
			if it, ok := m.current(); ok && it.URL != "" {
				m.notice = "curl command copied"
				return m, func() tea.Msg { return StoreCopyMsg{URL: "curl -LO " + it.URL} }
			}
			m.notice = "this item has no public link yet"
		case "x":
			if it, ok := m.current(); ok && it.SHA256 != "" {
				m.notice = "checksum copied"
				return m, func() tea.Msg { return StoreCopyMsg{URL: it.SHA256} }
			}
			m.notice = "no checksum on disk yet"
		case "/":
			m.search = true
			m.input.Focus()
			m.input.SetValue("")
			m.input.SetWidth(max(10, m.bodyW))
			return m, nil
		}
	}
	return m, nil
}

func (m StoreDetailModal) pageStep() int {
	if m.textH > 2 {
		return m.textH - 2
	}
	return 1
}

// modalPlacement is where Overlay will land the box (it centres the same
// way), so Update can hit-test mouse rows against the drawn pixels.
type modalPlacement struct{ x, y, w, h int }

var lastStoreModal modalPlacement

func (p modalPlacement) inside(x, y int) bool {
	if p.w == 0 {
		return true // never drawn yet (tests): the modal owns the event
	}
	return x >= p.x && x < p.x+p.w && y >= p.y && y < p.y+p.h
}

// ── rendering ─────────────────────────────────────────────────────────────

func (m *StoreDetailModal) View(width, height int) string {
	m.w, m.h = width, height
	// A big card: three quarters of the screen, floor of 72 columns when
	// the terminal can afford it, never wider than the terminal minus a
	// margin so the dimmed shelf still frames it.
	inner := min(width-4, max(60, width*3/4))
	m.bodyW = inner - 6 // border (2) + horizontal padding (4)
	if m.search {
		m.input.SetWidth(m.bodyW)
	}

	it, ok := m.current()
	if !ok {
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).Padding(1, 2).Width(inner).
			Render(dimStyle().Render("the shelf is empty"))
	}

	nameStyle := lipgloss.NewStyle().Foreground(ColorSand).Bold(true)
	art := lipgloss.NewStyle().Foreground(ColorCommand)
	sep := lipgloss.NewStyle().Foreground(ColorDimmer).Render(strings.Repeat("─", m.bodyW))

	var b strings.Builder

	// --- title row: name · version on the left, kind badge on the right ---
	name := it.Name
	if it.Version != "" {
		name += " · v" + it.Version
	}
	name = trunc(name, m.bodyW-8)
	if it.Missing {
		nameStyle = nameStyle.Foreground(ColorDimmer).Strikethrough(true)
	}
	b.WriteString(nameStyle.Render(name) +
		lipgloss.NewStyle().Foreground(ColorDimmer).Render(
			leftPad("["+kindLabel(it.Kind)+"]", m.bodyW-lipgloss.Width(name))) + "\n")
	b.WriteString(sep + "\n")

	// --- logo band: full art, generous (the grid card clips it; here it
	// has the room the art deserves) ---
	if lines := logoLines(it.Logo); len(lines) > 0 {
		if len(lines) == 1 && IsWordmark(it.Logo) {
			lines = Wordmark(it.Logo)
		}
		if len(lines) > 7 {
			lines = lines[:7]
		}
		b.WriteString("\n")
		for _, ln := range lines {
			b.WriteString("   " + art.Render(trunc(ln, m.bodyW-3)) + "\n")
		}
	}
	b.WriteString("\n")

	// --- fetch block: the way(s) to get it, wrapped, never truncated ---
	green := lipgloss.NewStyle().Foreground(ColorGreen)
	if it.URL != "" {
		b.WriteString(wrapped("  $ ", "curl -LO "+it.URL, m.bodyW, green))
		b.WriteString(wrapped("    ", it.URL, m.bodyW, dimStyle()))
	} else {
		b.WriteString(dimStyle().Render("  no public link — ask the bartender") + "\n")
	}
	if it.SHA256 != "" {
		b.WriteString(wrapped("  ", "sha256 "+it.SHA256, m.bodyW, dimStyle()))
	}
	meta := fmt.Sprintf("%s · %d downloads", humanBytes(it.Size), it.Downloads)
	if it.Missing {
		meta += " · not staged on this box"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(meta) + "\n")
	b.WriteString("\n" + sep + "\n")

	// --- body: the one-line description as the intro (bright), then the
	// long details (soft) — a hierarchy, not a wall ---
	type tline struct {
		text string
		st   lipgloss.Style
	}
	var text []tline
	if d := strings.TrimSpace(it.Description); d != "" {
		intro := lipgloss.NewStyle().Foreground(ColorSand)
		for _, l := range wrapProse(d, m.bodyW) {
			text = append(text, tline{l, intro})
		}
	}
	if det := strings.TrimSpace(it.Details); det != "" {
		if len(text) > 0 {
			text = append(text, tline{"", lipgloss.NewStyle()})
		}
		body := lipgloss.NewStyle().Foreground(ColorDesc)
		for _, l := range wrapProse(det, m.bodyW) {
			text = append(text, tline{l, body})
		}
	}
	if len(text) == 0 {
		text = []tline{{"the operator left no description on this one.", dimStyle()}}
	}
	m.nText = len(text)

	// The body fills what's left; footer is status + nav + keys + notice.
	headerH := lipgloss.Height(b.String())
	const footerH = 4
	const frameH = 4 // border (2) + vertical padding (2)
	m.textH = max(3, height-2-headerH-footerH-frameH)
	m.scroll = clampInt(m.scroll, 0, max(0, m.nText-m.textH))
	shown := text[m.scroll:min(m.scroll+m.textH, m.nText)]
	for _, l := range shown {
		b.WriteString(l.st.Render(l.text) + "\n")
	}
	// pad the window so the footer sits at the same rows every frame
	for i := len(shown); i < m.textH; i++ {
		b.WriteString("\n")
	}

	// --- footer: scroll status, shelf nav, keys, notice ---
	b.WriteString(m.statusLine() + "\n")
	b.WriteString(m.navLine() + "\n")
	b.WriteString(m.footerLine() + "\n")
	notice := " "
	if m.notice != "" {
		notice = lipgloss.NewStyle().Foreground(ColorGreen).Render("✓ " + m.notice)
	}
	if m.search {
		notice = m.input.View()
	}
	b.WriteString(notice)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAmber).
		Padding(1, 2).
		Width(inner).
		Render(strings.TrimSuffix(b.String(), "\n"))

	// cache placement for hit-testing: Overlay centres identically
	bh := lipgloss.Height(box)
	bw := lipgloss.Width(box)
	lastStoreModal = modalPlacement{
		x: max(0, (width-bw)/2),
		y: max(0, (height-bh)/2),
		w: bw, h: bh,
	}
	// from the bottom: border, pad, notice, keys, nav → nav is bh-5 rows down
	m.navRow = lastStoreModal.y + bh - 5
	return box
}

// navLine shows the position on the shelf plus clickable prev/next names —
// the affordance that makes "browse the catalogue" discoverable.
func (m StoreDetailModal) navLine() string {
	if len(m.items) <= 1 {
		return ""
	}
	// the line carries a 2-space indent on each side: budget for it or
	// the closing ▸ wraps onto a row of its own
	bw := m.bodyW - 4
	dim := dimStyle()
	prev := "◂ " + trunc(m.items[(m.idx-1+len(m.items))%len(m.items)].Name, bw/2-4)
	next := trunc(m.items[(m.idx+1)%len(m.items)].Name, bw/2-4) + " ▸"
	pos := fmt.Sprintf("%d / %d", m.idx+1, len(m.items))
	pad := bw - lipgloss.Width(prev) - lipgloss.Width(next) - lipgloss.Width(pos)
	if pad < 2 {
		prev, next = "◂", "▸"
		pad = bw - 4 - lipgloss.Width(pos)
	}
	left := pad / 2
	return "  " + dim.Render(prev+strings.Repeat(" ", max(1, left))) +
		lipgloss.NewStyle().Foreground(ColorDimmer).Render(pos) +
		dim.Render(strings.Repeat(" ", max(1, pad-left))+next+"  ")
}

// footerLine is the fixed keys strip. It never changes content, so copy
// results can't make it disappear (that line belongs to the notice strip).
func (m StoreDetailModal) footerLine() string {
	dim := dimStyle()
	dimmer := lipgloss.NewStyle().Foreground(ColorDimmer)
	k := func(key, what string) string {
		return dimmer.Render(key) + dim.Render(" "+what)
	}
	parts := []string{k("⏎", "copy"), k("u", "curl"), k("x", "sha")}
	if len(m.items) > 1 {
		parts = append(parts, k("←↑↓→", "browse"))
	}
	if m.nText > m.textH {
		parts = append(parts, k("pgup/pgdn", "page"))
	}
	parts = append(parts, k("esc", "back"))
	return "  " + strings.Join(parts, dim.Render("  ·  "))
}

// statusLine tells the reader how much of the card they can't see — a
// silent cut at the fold was the original complaint.
func (m StoreDetailModal) statusLine() string {
	rest := m.nText - m.textH - m.scroll
	if rest <= 0 && m.scroll <= 0 {
		return "" // the whole card fits; nothing to explain
	}
	dimmer := lipgloss.NewStyle().Foreground(ColorDimmer)
	var parts []string
	if m.scroll > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d above", m.scroll))
	}
	if rest > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d below", rest))
	}
	return dimmer.Render("  " + strings.Join(parts, " · "))
}

// wrapped renders label+value as hanging-indented lines that always fit
// bodyW — the full URL matters more than a tidy single row.
func wrapped(label, value string, w int, style lipgloss.Style) string {
	var b strings.Builder
	rest := value
	first := true
	for {
		room := w - lipgloss.Width(label)
		if room < 8 {
			room = 8
		}
		chunk := rest
		if lipgloss.Width(chunk) > room {
			r := []rune(chunk)
			cut := room
			for cut > 0 && lipgloss.Width(string(r[:cut])) > room {
				cut--
			}
			chunk, rest = string(r[:cut]), string(r[cut:])
		} else {
			rest = ""
		}
		l := label
		if !first {
			l = strings.Repeat(" ", lipgloss.Width(label))
		}
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(l) + style.Render(chunk) + "\n")
		if rest == "" {
			return b.String()
		}
		first = false
	}
}

// leftPad right-aligns s inside w columns.
func leftPad(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// wrapProse word-wraps plain text to w columns, preserving explicit
// newlines and blank-line paragraph breaks.
func wrapProse(s string, w int) []string {
	var out []string
	if w < 8 {
		w = 8
	}
	for _, para := range strings.Split(s, "\n") {
		para = strings.TrimRight(para, " \t")
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
			} else if lipgloss.Width(line)+1+lipgloss.Width(word) <= w {
				line += " " + word
			} else {
				out = append(out, line)
				line = word
			}
			// a single word wider than the column: hard-split it
			for lipgloss.Width(line) > w {
				r := []rune(line)
				cut := w
				for cut > 0 && lipgloss.Width(string(r[:cut])) > w {
					cut--
				}
				out = append(out, string(r[:cut]))
				line = string(r[cut:])
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ColorDim)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
