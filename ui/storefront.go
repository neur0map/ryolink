package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ryolink/internal/fuzzy"
)

// Storefront is the TUI browser for the Ryoku store: one block per artifact
// (ISO, recovery script, rescue file), cursor- or click-selectable, with the
// canonical download URL always visible. It renders inside the chat column
// when the current room has type "store" — the store is ryolink's front
// page; the chat rooms are what you do after you've picked up your copy.
//
// Interaction model: the grid is the index (browse, filter), ENTER opens
// the product card (StoreDetailModal) where the full description and the
// copy actions live. `/` filters the shelf in place.
type Storefront struct {
	items     []StoreItemView
	cursor    int
	width     int
	height    int
	notices   []string // transient lines (URL copied, download hints)
	title     string   // store title
	rowOrigin int      // screen y of the first item row (set by parent)
	top       int      // index of first visible item (scroll window)

	filter string          // active shelf filter (fuzzy over name/desc/kind)
	input  textinput.Model // '/' search bar; shown when focused or non-empty
	search bool
}

// StoreItemView is the presentation shape the ui layer receives from
// internal/shop (decoupled so ui never imports database code).
type StoreItemView struct {
	ID          string
	Name        string
	Kind        string // iso | script | file
	Description string
	Details     string // long form for the detail modal (may be empty)
	Version     string
	URL         string // canonical public URL
	Size        int64
	SHA256      string
	Downloads   int
	Missing     bool
	Logo        string // ASCII art for the card (multi-line, may be empty)
}

func NewStorefront(title string) *Storefront {
	ti := textinput.New()
	ti.Placeholder = "filter the shelves..."
	ti.Prompt = "/ "
	return &Storefront{title: title, input: ti}
}

func (s *Storefront) SetItems(items []StoreItemView) {
	// items is the FULL catalogue; the cursor indexes the filtered view, so
	// clamp against what's actually selectable right now.
	s.items = items
	if s.cursor >= len(s.visible()) {
		s.cursor = max(0, len(s.visible())-1)
	}
}

func (s *Storefront) SetSize(w, h int) { s.width, s.height = w, h }

// SetOrigin records the screen row where the item list starts, so mouse
// clicks (absolute coordinates) map to rows.
func (s *Storefront) SetOrigin(top int) { s.rowOrigin = top }

// visible is the catalogue under the active filter, in shelf order.
func (s *Storefront) visible() []StoreItemView {
	q := strings.ToLower(strings.TrimSpace(s.filter))
	if q == "" {
		return s.items
	}
	var out []StoreItemView
	for _, it := range s.items {
		hay := strings.ToLower(it.Name + " " + it.Description + " " + it.Kind + " " + it.Version)
		if fuzzy.Score(q, hay) >= 0 {
			out = append(out, it)
		}
	}
	return out
}

// SetFilter applies a shelf filter from outside the grid (the detail
// modal's '/' search, or the command palette).
func (s *Storefront) SetFilter(q string) {
	s.filter = strings.TrimSpace(q)
	s.input.SetValue(s.filter)
	s.cursor = 0
	s.search = false
	s.input.Blur()
}

// Filter returns the active shelf filter ("" = all items).
func (s *Storefront) Filter() string { return s.filter }

// VisibleItems returns the shelf under the active filter — the same list
// the detail modal navigates, so the cursor transfers 1:1 on ENTER.
func (s *Storefront) VisibleItems() []StoreItemView { return s.visible() }

// Cursor is the selected index in filtered space.
func (s *Storefront) Cursor() int { return s.cursor }

// SetCursor moves the shelf selection (used when the detail modal closes
// and its browsing position should carry back to the grid).
func (s *Storefront) SetCursor(i int) {
	if n := len(s.visible()); n > 0 {
		s.cursor = min(max(i, 0), n-1)
	} else {
		s.cursor = 0
	}
}

// Searching reports whether the '/' filter bar currently has focus.
func (s *Storefront) Searching() bool { return s.search }

// Selected returns the item under the cursor, in filtered space.
func (s *Storefront) Selected() (StoreItemView, bool) {
	v := s.visible()
	if s.cursor < 0 || s.cursor >= len(v) {
		return StoreItemView{}, false
	}
	return v[s.cursor], true
}

// StoreCopyMsg asks the parent program to place a URL on the user's
// clipboard (tea.SetClipboard works over SSH via OSC 52).
type StoreCopyMsg struct{ URL string }

func (s *Storefront) Update(msg tea.Msg) (*Storefront, tea.Cmd) {
	// While the search bar has focus it owns all text input.
	if s.search {
		switch m := msg.(type) {
		case tea.KeyPressMsg:
			switch m.String() {
			case "esc":
				s.search = false
				s.input.Blur()
				s.filter = ""
				s.input.SetValue("")
				s.cursor = 0
				return s, nil
			case "enter":
				s.search = false
				s.input.Blur()
				s.filter = strings.TrimSpace(s.input.Value())
				s.cursor = 0
				return s, nil
			case "up", "down":
				// leave search to browse results
				s.search = false
				s.input.Blur()
				s.filter = strings.TrimSpace(s.input.Value())
				s.cursor = 0
				// fall through to the grid handler below
			default:
				var cmd tea.Cmd
				s.input, cmd = s.input.Update(msg)
				return s, cmd
			}
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
	}

	switch m := msg.(type) {
	case tea.KeyPressMsg:
		v := s.visible()
		switch m.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(v)-1 {
				s.cursor++
			}
		case "left", "h":
			perRow := s.bentoCols()
			if s.cursor-perRow >= 0 {
				s.cursor -= perRow
			}
		case "right", "l":
			perRow := s.bentoCols()
			if s.cursor+perRow < len(v) {
				s.cursor += perRow
			}
		case "home":
			s.cursor = 0
		case "end":
			s.cursor = max(0, len(v)-1)
		case "enter", " ":
			if _, ok := s.Selected(); ok {
				return s, func() tea.Msg { return StoreOpenDetailMsg{} }
			}
		case "c", "y":
			if it, ok := s.Selected(); ok && it.URL != "" {
				s.notice("link copied — paste in a browser, or: curl -LO " + it.URL)
				return s, func() tea.Msg { return StoreCopyMsg{URL: it.URL} }
			}
		case "/":
			s.search = true
			s.input.SetValue(s.filter)
			s.input.Focus()
			// deliberately not forwarded to the input: that would insert a
			// literal “/” as the first character of the query
			return s, nil
		case "esc":
			// the “/ query (esc clears)” chip promises this
			if s.filter != "" {
				s.filter = ""
				s.input.SetValue("")
				s.cursor = 0
			}
		}
	case tea.MouseWheelMsg:
		v := s.visible()
		if m.Button == tea.MouseWheelUp && s.cursor > 0 {
			s.cursor--
		} else if m.Button == tea.MouseWheelDown && s.cursor < len(v)-1 {
			s.cursor++
		}
	case tea.MouseClickMsg:
		// Bento: clicks map through the same row geometry View drew with.
		// Click selects; click again opens the product card.
		if idx := s.itemAtClick(m.X, m.Y); idx >= 0 {
			if idx == s.cursor {
				return s, func() tea.Msg { return StoreOpenDetailMsg{} }
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

	// headerH counts title+sub+gradient+blank = 4, plus the search bar row
	// when the bar is showing (focused, or holding a filter).
	headerH := 4
	if s.search || s.filter != "" {
		searchStyle := lipgloss.NewStyle().Foreground(ColorDim).Italic(true)
		bar := "  " + s.input.View()
		if !s.search {
			bar = searchStyle.Render("  / " + s.filter + "  (esc clears)")
		}
		b.WriteString(bar + "\n")
		headerH++
	}

	items := s.visible()
	if len(items) == 0 {
		if len(s.items) > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render(
				"  nothing on the shelf matches “" + s.filter + "” — esc to clear"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render(
				"  the shelves are empty — the owner has not stocked ryolink.yaml yet."))
		}
		return b.String()
	}

	perRow := s.bentoCols()
	cardW := s.bentoCardW()
	avail := max(1, s.height-headerH-len(s.notices))

	// Scroll by rows: the cursor's row must stay visible. Row heights vary
	// (logo bands differ), so budget screen rows, not item counts.
	rowOf := s.cursor / perRow
	firstRow := 0
	used := 0
	for r := 0; r*perRow < len(items) && r <= rowOf; r++ {
		used += s.rowHeight(r, perRow, items)
		for firstRow < r && used > avail {
			used -= s.rowHeight(firstRow, perRow, items)
			firstRow++
		}
	}
	s.top = firstRow * perRow
	lo := s.top

	// render rows until the height budget runs out
	used = 0
	r := firstRow
	for ; r*perRow < len(items); r++ {
		rh := s.rowHeight(r, perRow, items)
		if used+rh > avail {
			break
		}
		used += rh
		var cells []string
		for c := 0; c < perRow && r*perRow+c < len(items); c++ {
			cells = append(cells, s.renderCard(items[r*perRow+c], cardW, rh))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, joinGap(cells)...))
		b.WriteString("\n")
	}
	hi := r * perRow
	if hi > len(items) {
		hi = len(items)
	}
	if lo > 0 || hi < len(items) {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorDimmer).Render(
			fmt.Sprintf("  items %d–%d of %d", lo+1, hi, len(items))))
		b.WriteString("\n")
	}

	for _, n := range s.notices {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorGreen).Render("  ✓ "+n) + "\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render(
		"  ↑↓ / click  ·  enter opens  ·  / search  ·  c copy url"))
	return b.String()
}

// rowHeight is the screen height of grid row r (tallest card in it).
func (s *Storefront) rowHeight(r, perRow int, items []StoreItemView) int {
	h := 0
	for c := 0; c < perRow && r*perRow+c < len(items); c++ {
		if ch := cardHFor(items[r*perRow+c]); ch > h {
			h = ch
		}
	}
	return h
}

// itemAtClick maps a click to the item whose card contains (x, y). The
// index is in *filtered* space; the grid renders that list, not s.items.
func (s *Storefront) itemAtClick(x, y int) int {
	// the '/' search bar row shifts the grid down one from SetOrigin's 4
	origin := s.rowOrigin
	if s.search || s.filter != "" {
		origin++
	}
	rel := y - origin
	if rel < 0 {
		return -1
	}
	items := s.visible()
	perRow := s.bentoCols()
	cardW := s.bentoCardW()
	r := s.top / perRow
	for ; r*perRow < len(items); r++ {
		rh := s.rowHeight(r, perRow, items)
		if rel < rh {
			col := 0
			for c := 1; c < perRow && r*perRow+c < len(items); c++ {
				if x >= c*(cardW+2) {
					col = c
				}
			}
			idx := r*perRow + col
			if idx < len(items) {
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
	v := s.visible()
	if i < 0 || i >= len(v) {
		return ""
	}
	return v[i].ID
}
