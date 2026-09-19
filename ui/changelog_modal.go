package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"ryolink/internal/version"
)

type ChangelogModal struct{}

func NewChangelogModal() ChangelogModal {
	return ChangelogModal{}
}

func (c ChangelogModal) View(width, height int) string {
	modalW := 46

	headerText := " Changelog "
	fillLen := modalW - len(headerText)
	if fillLen < 4 {
		fillLen = 4
	}
	leftFill := strings.Repeat("╱", fillLen/2)
	rightFill := strings.Repeat("╱", fillLen-fillLen/2)

	headerFill := lipgloss.NewStyle().Foreground(ColorBorder).Render(leftFill)
	headerTitle := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(headerText)
	headerFillR := lipgloss.NewStyle().Foreground(ColorBorder).Render(rightFill)
	header := headerFill + headerTitle + headerFillR

	verStyle := lipgloss.NewStyle().Foreground(ColorAmber).Bold(true)
	bullet := lipgloss.NewStyle().Foreground(ColorDim)
	change := lipgloss.NewStyle().Foreground(ColorSand)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")

	// Show latest 2 versions
	shown := version.Changelog
	if len(shown) > 2 {
		shown = shown[:2]
	}

	for i, entry := range shown {
		b.WriteString("\n")
		tag := "v" + entry.Version
		if i == 0 {
			tag += " (latest)"
		}
		b.WriteString("  " + verStyle.Render(tag))
		b.WriteString("\n")

		for _, ch := range entry.Changes {
			b.WriteString(bullet.Render("  · ") + change.Render(ch))
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	footerFill := lipgloss.NewStyle().Foreground(ColorBorder).Render(
		strings.Repeat("╱", modalW))
	b.WriteString(footerFill)
	b.WriteString("\n")

	esc := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("ESC")
	b.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render(
		fmt.Sprintf("  %s close", esc)))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Render(b.String())
}
