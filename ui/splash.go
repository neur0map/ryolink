package ui

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ryolink/internal/version"
)

// The RYOKU wordmark, verbatim from the Ryoku ISO installer
// (installation/tui bigLetters): five block letters, five rows.
var bigLetters = [][]string{
	{"████ ", "█  █ ", "████ ", "█ █  ", "█  █ "},
	{"█   █", " █ █ ", "  █  ", "  █  ", "  █  "},
	{"█████", "█   █", "█   █", "█   █", "█████"},
	{"█  █ ", "█ █  ", "██   ", "█ █  ", "█  █ "},
	{"█   █", "█   █", "█   █", "█   █", "█████"},
}

func bigRows(scale int) []string {
	rows := make([]string, 5)
	for r := range 5 {
		parts := make([]string, len(bigLetters))
		for i := range bigLetters {
			parts[i] = bigLetters[i][r]
		}
		row := strings.Join(parts, " ")
		if scale > 1 {
			var d strings.Builder
			for _, ch := range row {
				d.WriteString(strings.Repeat(string(ch), scale))
			}
			row = d.String()
		}
		rows[r] = row
	}
	if scale <= 1 {
		return rows
	}
	// double the rows too: 2x2 scale per cell
	out := make([]string, 0, len(rows)*scale)
	for _, r := range rows {
		for range scale {
			out = append(out, r)
		}
	}
	return out
}

// revealBanner renders the wordmark mid-reveal: columns up to the reveal cut
// shimmer along the ryoku brand gradient (vermilion→gold, phase-shifted),
// the rest waits as dim ink. This is the installer's boot animation, ported.
func revealBanner(reveal float64, phase, scale int) []string {
	rows := bigRows(scale)
	total := lipgloss.Width(rows[0])
	cut := int(reveal * float64(total+3))
	dim := lipgloss.NewStyle().Foreground(ColorDimmer)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var b strings.Builder
		for i, ch := range []rune(row) {
			if ch == ' ' {
				b.WriteString(" ")
				continue
			}
			if i <= cut {
				t := float64((i+phase)%total) / float64(total-1)
				b.WriteString(lipgloss.NewStyle().Foreground(BrandColor(t * 2)).Render(string(ch)))
			} else {
				b.WriteString(dim.Render(string(ch)))
			}
		}
		out = append(out, b.String())
	}
	return out
}

var enterPulse = []string{
	"[ ENTER ]",
	"[ ENTER ]",
	"[  >>>  ]",
	"[  >>>  ]",
}

// revealTicks is how many splash frames the wordmark takes to light up
// fully (at splashTickInterval ≈ 150 ms, just under 4 s), then it holds and
// shimmers, like the installer intro.
const revealTicks = 24

// ryoku-hue sparks — the floating field behind the card.
var sparkChars = []string{"✦", "·", "✧", "°", "∘", "⋅", "*", "•"}
var sparkColors = []color.Color{
	lipgloss.Color("#F25623"), // torii vermilion
	lipgloss.Color("#FFD24A"), // gold
	lipgloss.Color("#7aa2f7"), // ai blue
	lipgloss.Color("#f7768e"), // sakura
	lipgloss.Color("#9ece6a"), // matcha
	lipgloss.Color("#3b4261"), // ink grey
	lipgloss.Color("#bb9af7"), // fuji violet
	lipgloss.Color("#7dcfff"), // ice
}

type spark struct {
	x, y    int
	charIdx int
	colIdx  int
	speed   int
	tick    int
}

type splashTickMsg time.Time

type Splash struct {
	ryolinkDomain string
	tagline       string
	nickname      string
	fingerprint   string
	flair         bool
	width         int
	height        int
	frame         int
	sparks        []spark
	rng           *rand.Rand
	inited        bool
}

func NewSplash(nickname, fingerprint string, flair bool, ryolinkDomain, tagline string) Splash {
	return Splash{
		ryolinkDomain: ryolinkDomain,
		tagline:       tagline,
		nickname:      nickname,
		fingerprint:   fingerprint,
		flair:         flair,
		rng:           rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()^0x5bd1e995))),
	}
}

func (s *Splash) initSparks() {
	if s.width == 0 || s.height == 0 {
		return
	}
	count := (s.width * s.height) / 25
	if count > 300 {
		count = 300
	}
	s.sparks = make([]spark, count)
	for i := range s.sparks {
		s.sparks[i] = spark{
			x:       s.rng.IntN(s.width),
			y:       s.rng.IntN(s.height),
			charIdx: s.rng.IntN(len(sparkChars)),
			colIdx:  s.rng.IntN(len(sparkColors)),
			speed:   1 + s.rng.IntN(3),
			tick:    s.rng.IntN(10),
		}
	}
	s.inited = true
}

func (s *Splash) tickSparks() {
	for i := range s.sparks {
		sp := &s.sparks[i]
		sp.tick++
		if sp.tick%sp.speed == 0 {
			sp.y--
			if s.rng.IntN(2) == 0 {
				sp.x += s.rng.IntN(3) - 1
			}
			if sp.y < 0 {
				sp.y = s.height - 1
				sp.x = s.rng.IntN(s.width)
			}
			if sp.x < 0 {
				sp.x = 0
			}
			if sp.x >= s.width {
				sp.x = s.width - 1
			}
		}
	}
}

func splashTick() tea.Cmd {
	return tea.Tick(splashTickInterval, func(t time.Time) tea.Msg {
		return splashTickMsg(t)
	})
}

func (s Splash) Init() tea.Cmd {
	return splashTick()
}

// Update keeps the original contract: keys are intercepted by App.Update;
// the splash model only advances its own frame clock.
func (s Splash) Update(msg tea.Msg) (Splash, tea.Cmd) {
	switch msg.(type) {
	case splashTickMsg:
		s.frame++
		s.tickSparks()
		return s, splashTick()
	}
	return s, nil
}

func (s Splash) View() tea.View {
	if s.width == 0 || s.height == 0 {
		return tea.NewView("loading...")
	}

	card := s.renderCard()
	boxLines := strings.Split(card, "\n")

	// center the card
	cardH := len(boxLines)
	startY := (s.height - cardH) / 2
	if startY < 0 {
		startY = 0
	}
	endY := startY + cardH
	maxW := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}
	startX := (s.width - maxW) / 2
	if startX < 0 {
		startX = 0
	}
	endX := startX + maxW

	// spark map: cell -> spark index
	type sparkRef struct {
		charIdx int
		colIdx  int
	}
	sparkMap := map[[2]int]sparkRef{}
	for _, sp := range s.sparks {
		key := [2]int{sp.x, sp.y}
		if _, exists := sparkMap[key]; !exists {
			sparkMap[key] = sparkRef{charIdx: sp.charIdx, colIdx: sp.colIdx}
		}
	}

	screenLines := make([]string, 0, s.height)
	boxIdx := 0
	for y := 0; y < s.height; y++ {
		var line strings.Builder
		if y >= startY && y < endY && boxIdx < len(boxLines) {
			for x := 0; x < startX; x++ {
				if sp, ok := sparkMap[[2]int{x, y}]; ok {
					c := sparkColors[sp.colIdx%len(sparkColors)]
					ch := sparkChars[sp.charIdx%len(sparkChars)]
					line.WriteString(lipgloss.NewStyle().Foreground(c).Render(ch))
				} else {
					line.WriteRune(' ')
				}
			}
			// Box content, padded to maxW
			boxLine := boxLines[boxIdx]
			boxIdx++
			pad := maxW - lipgloss.Width(boxLine)
			if pad < 0 {
				pad = 0
			}
			line.WriteString(boxLine + strings.Repeat(" ", pad))
			for x := endX; x < s.width; x++ {
				if sp, ok := sparkMap[[2]int{x, y}]; ok {
					c := sparkColors[sp.colIdx%len(sparkColors)]
					ch := sparkChars[sp.charIdx%len(sparkChars)]
					line.WriteString(lipgloss.NewStyle().Foreground(c).Render(ch))
				} else {
					line.WriteRune(' ')
				}
			}
		} else {
			for x := 0; x < s.width; x++ {
				if sp, ok := sparkMap[[2]int{x, y}]; ok {
					c := sparkColors[sp.colIdx%len(sparkColors)]
					ch := sparkChars[sp.charIdx%len(sparkChars)]
					line.WriteString(lipgloss.NewStyle().Foreground(c).Render(ch))
				} else {
					line.WriteRune(' ')
				}
			}
		}

		screenLines = append(screenLines, line.String())
	}

	v := tea.NewView(strings.Join(screenLines, "\n"))
	v.AltScreen = true
	v.WindowTitle = s.ryolinkDomain
	return v
}

// padLine centers raw content inside w, padding both sides so every card
// line is exactly w wide — per-line left-pad let lines of different widths
// drift against the rules.
func padLine(s string, w int) string {
	lw := lipgloss.Width(s)
	if lw >= w {
		return s
	}
	left := (w - lw) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-lw-left)
}

func (s Splash) renderCard() string {
	// The wordmark reveal eases out over revealTicks frames, then holds and
	// shimmers — the Ryoku installer's boot animation, doubled in size.
	reveal := float64(s.frame) / float64(revealTicks)
	if reveal > 1 {
		reveal = 1
	}
	reveal = 1 - (1-reveal)*(1-reveal) // ease-out
	phase := s.frame

	var b strings.Builder
	w := cardW

	b.WriteString(GradientBar(w, s.frame))
	b.WriteString("\n\n")

	title := GradientText(s.ryolinkDomain, ColorAmber, ColorHighlight, true)
	b.WriteString(padLine(title, w))
	b.WriteString("\n")
	tag := s.tagline
	if tag == "" {
		tag = "the ryoku store. and a room."
	}
	b.WriteString(padLine(SplashSubtitleStyle.Render(tag), w))
	b.WriteString("\n\n")

	for _, row := range revealBanner(reveal, phase, 2) {
		b.WriteString(padLine(row, w))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	nick := s.nickname
	if s.flair {
		nick = "★ " + nick
	}
	b.WriteString(padLine(SplashDescStyle.Render("you are ")+NickStyle(0).Render(nick), w))
	b.WriteString("\n\n")

	b.WriteString(padLine(SplashDescStyle.Render("get ryoku — iso, recovery, rescue files."), w))
	b.WriteString("\n")
	b.WriteString(padLine(SplashDescStyle.Render("then stay: rooms, games, people."), w))
	b.WriteString("\n\n")

	b.WriteString(padLine(SplashDescStyle.Italic(true).Render("no accounts. no logs."), w))
	b.WriteString("\n")
	b.WriteString(padLine(SplashDescStyle.Italic(true).Render("say what you mean."), w))
	b.WriteString("\n\n")

	b.WriteString(padLine(SplashDescStyle.Foreground(ColorDimmer).Render("chat resets every sunday. the store is permanent."), w))
	b.WriteString("\n\n")

	enterFrame := enterPulse[s.frame%len(enterPulse)]
	enterDesc := lipgloss.NewStyle().Foreground(ColorSand).Render(" enter — press return")
	enterKey := SplashKeyStyle.Render(enterFrame)
	quitKey := SplashKeyStyle.Render("[ Q ]")
	quitDesc := lipgloss.NewStyle().Foreground(ColorDim).Render(" exit")
	b.WriteString(padLine(enterKey+enterDesc+"    "+quitKey+quitDesc, w))
	b.WriteString("\n\n")

	b.WriteString(GradientBar(w, s.frame+20))
	b.WriteString("\n")
	verStr := fmt.Sprintf("[ v%s ]", version.Version)
	b.WriteString(padLine(SplashDescStyle.Render(verStr), w))
	b.WriteString("\n")
	cKey := SplashKeyStyle.Render("[ C ]")
	cDesc := lipgloss.NewStyle().Foreground(ColorDim).Render(" changelog")
	b.WriteString(padLine(cKey+cDesc, w))
	b.WriteString("\n")

	return b.String()
}

const cardW = 60

// GradientBar draws a horizontal rule sweeping the ryoku brand gradient —
// the card's top/bottom border, ping-ponged so the seam never snaps.
func GradientBar(w, frame int) string {
	var b strings.Builder
	for i := 0; i < w; i++ {
		t := float64((i+frame)%(2*w)) / float64(2*w-1)
		if t > 0.5 {
			t = 1 - t
		}
		b.WriteString(lipgloss.NewStyle().Foreground(BrandColor(t * 2)).Render("─"))
	}
	return b.String()
}

type EnterRyolinkMsg struct{}
type ShowHelpMsg struct{}
