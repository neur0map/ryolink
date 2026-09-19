package ui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Ryoku palette — the Tokyo-Night-derived visual language shared with the
// Ryoku installers (ryoku-shell-installer/style.go): same background, text
// and brand hues, so ryolink reads as part of the same product.
var (
	ColorBackground = lipgloss.Color("#16161e") // sumi ink
	ColorDarkBg     = lipgloss.Color("#101017")
	ColorPanelBg    = lipgloss.Color("#1a1b26")
	ColorSand       = lipgloss.Color("#c0caf5") // sakura text
	ColorDim        = lipgloss.Color("#7079b3")
	ColorDimmer     = lipgloss.Color("#3b4261")
	ColorBorder     = lipgloss.Color("#2a2b3d")
	ColorHighlight  = lipgloss.Color("#FFD24A") // gold (gradient B stop)
	ColorAmber      = lipgloss.Color("#F25623") // vermilion (gradient A stop)
	ColorTitle      = lipgloss.Color("#c0caf5")
	ColorCommand    = lipgloss.Color("#7aa2f7") // ai-blue
	ColorDesc       = lipgloss.Color("#7079b3")
	ColorAccent     = lipgloss.Color("#F25623") // torii vermilion
	ColorGreen      = lipgloss.Color("#9ece6a") // matcha
	ColorTyping     = lipgloss.Color("#7dcfff")
	ColorMention    = lipgloss.Color("#e0af68") // kincha gold for @mentions
	ColorIndigo     = lipgloss.Color("#7aa2f7")

	// 12 tones for nicknames: the ryoku hue family — ink blues, sakura,
	// matcha, kincha, fuji violet — muted enough to sit on sumi ink.
	NickColors = []color.Color{
		lipgloss.Color("#7aa2f7"), // ai blue
		lipgloss.Color("#9ece6a"), // matcha
		lipgloss.Color("#e0af68"), // kincha gold
		lipgloss.Color("#f7768e"), // sakura pink
		lipgloss.Color("#7dcfff"), // sky ice
		lipgloss.Color("#bb9af7"), // fuji violet
		lipgloss.Color("#F25623"), // torii vermilion
		lipgloss.Color("#73acaa"), // seigaiha teal
		lipgloss.Color("#c0caf5"), // pale indigo
		lipgloss.Color("#a9b1d6"), // tsukumo grey-blue
		lipgloss.Color("#ff9e64"), // aki orange
		lipgloss.Color("#cfc9c2"), // shirakusa sand
	}
)

// Brand gradient stops, verbatim from the Ryoku installers (vermilion→gold).
var BrandGradA = [3]int{0xF2, 0x56, 0x23}
var BrandGradB = [3]int{0xFF, 0xD2, 0x4A}

// BrandColor lerps the ryoku brand gradient at t∈[0,1].
func BrandColor(t float64) color.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	r := BrandGradA[0] + int(float64(BrandGradB[0]-BrandGradA[0])*t)
	g := BrandGradA[1] + int(float64(BrandGradB[1]-BrandGradA[1])*t)
	b := BrandGradA[2] + int(float64(BrandGradB[2]-BrandGradA[2])*t)
	return lipgloss.Color(hexColor(r, g, b))
}

func hexColor(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

var (
	TopBarBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.Border{Bottom: "─"}, false, false, true, false).
				BorderForeground(ColorBorder)

	BottomBarStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			Border(lipgloss.Border{Top: "─"}, true, false, false, false).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	ChatBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorSand)

	// Left sidebar: border on right side
	LeftSidebarStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, true, false, false).
				BorderForeground(ColorBorder).
				Foreground(ColorSand).
				PaddingTop(1).
				PaddingLeft(1).
				PaddingRight(1)

	// Right sidebar: border on left side
	RightSidebarStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(ColorBorder).
				Foreground(ColorSand).
				PaddingTop(1).
				PaddingLeft(1).
				PaddingRight(1)

	SystemMsgStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			Italic(true)

	InputStyle = lipgloss.NewStyle().
			Foreground(ColorSand)

	// Splash screen styles
	SplashBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Foreground(ColorSand).
				Padding(1, 3)

	SplashTitleStyle = lipgloss.NewStyle().
				Foreground(ColorTitle).
				Bold(true)

	SplashSubtitleStyle = lipgloss.NewStyle().
				Foreground(ColorDim)

	SplashKeyStyle = lipgloss.NewStyle().
			Foreground(ColorHighlight).
			Bold(true)

	SplashDescStyle = lipgloss.NewStyle().
			Foreground(ColorDesc)

	SplashCategoryStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true).
				MarginTop(1)

	SplashCommandStyle = lipgloss.NewStyle().
				Foreground(ColorCommand).
				Bold(true)

	// Chat message styles
	MsgTimeStyle = lipgloss.NewStyle().
			Foreground(ColorDimmer)

	TypingStyle = lipgloss.NewStyle().
			Foreground(ColorTyping).
			Italic(true)

	// Mention autocomplete popup styles
	MentionPopupStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	MentionSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorAmber).
				Bold(true)

	MentionItemStyle = lipgloss.NewStyle().
				Foreground(ColorSand)

	MentionSelfStyle = lipgloss.NewStyle().
				Foreground(ColorAmber).
				Bold(true)

	MentionHighlightStyle = lipgloss.NewStyle().
				Foreground(ColorMention).
				Bold(true)
)

func NickStyle(colorIndex int) lipgloss.Style {
	idx := colorIndex % len(NickColors)
	return lipgloss.NewStyle().Foreground(NickColors[idx]).Bold(true)
}

func NickBarColor(colorIndex int) color.Color {
	return NickColors[colorIndex%len(NickColors)]
}
