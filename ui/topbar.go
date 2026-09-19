package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type TopBar struct {
	RyolinkName  string
	Room         string
	OnlineCount  int
	WeeklyCount  int
	AllTimeCount int
	Width        int
}

func (t TopBar) View() string {
	if t.Width < 20 {
		return ""
	}

	// Line 1: Diagonal fill with ryolink name embedded
	label := fmt.Sprintf(" %s ", t.RyolinkName)
	fillTotal := t.Width - len(label)
	leftN := fillTotal / 2
	rightN := fillTotal - leftN
	if leftN < 0 {
		leftN = 0
	}
	if rightN < 0 {
		rightN = 0
	}
	diagFill := lipgloss.NewStyle().Foreground(ColorBorder)
	diagTitle := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)
	diagLine := diagFill.Render(strings.Repeat("╱", leftN)) +
		diagTitle.Render(label) +
		diagFill.Render(strings.Repeat("╱", rightN))

	// Line 2: #room left | online right
	room := lipgloss.NewStyle().Foreground(ColorAmber).Bold(true).Render(
		fmt.Sprintf("  #%s", t.Room))

	onlineDot := lipgloss.NewStyle().Foreground(ColorGreen).Render("●")
	onlineNum := lipgloss.NewStyle().Foreground(ColorSand).Bold(true).Render(
		fmt.Sprintf("%d online", t.OnlineCount))
	weekly := lipgloss.NewStyle().Foreground(ColorDim).Render(
		fmt.Sprintf("%d this week", t.WeeklyCount))
	allTime := lipgloss.NewStyle().Foreground(ColorDimmer).Render(
		fmt.Sprintf("%d all time", t.AllTimeCount))
	dot := lipgloss.NewStyle().Foreground(ColorDimmer).Render(" · ")

	statsRight := fmt.Sprintf("%s %s%s%s%s%s  ", onlineDot, onlineNum, dot, weekly, dot, allTime)

	roomW := lipgloss.Width(room)
	statsW := lipgloss.Width(statsRight)
	gap := t.Width - roomW - statsW
	if gap < 1 {
		gap = 1
	}
	statsLine := room + strings.Repeat(" ", gap) + statsRight

	border := lipgloss.NewStyle().Foreground(ColorBorder).Render(
		"  " + strings.Repeat("─", t.Width-4) + "  ")

	return lipgloss.JoinVertical(lipgloss.Left, diagLine, statsLine, border)
}
