package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ryolink/internal/dm"
)

type DMOpenConvoMsg struct {
	PeerFP   string
	PeerNick string
}

type DMInbox struct {
	conversations []dm.Conversation
	cursor        int
	width, height int
}

func NewDMInbox(convos []dm.Conversation) DMInbox {
	return DMInbox{
		conversations: convos,
	}
}

func (d *DMInbox) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *DMInbox) SetConversations(convos []dm.Conversation) {
	d.conversations = convos
	if d.cursor >= len(convos) {
		d.cursor = len(convos) - 1
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
}

func (d DMInbox) Update(msg tea.Msg) (DMInbox, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if d.cursor > 0 {
				d.cursor--
			}
		case "down", "j":
			if d.cursor < len(d.conversations)-1 {
				d.cursor++
			}
		case "enter":
			if len(d.conversations) > 0 {
				c := d.conversations[d.cursor]
				return d, func() tea.Msg {
					return DMOpenConvoMsg{PeerFP: c.PeerFP, PeerNick: c.PeerNick}
				}
			}
		}
	}
	return d, nil
}

func (d DMInbox) View() string {
	accent := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	dimmer := lipgloss.NewStyle().Foreground(ColorDimmer)
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	sand := lipgloss.NewStyle().Foreground(ColorSand)
	highlight := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)
	amber := lipgloss.NewStyle().Foreground(ColorAmber).Bold(true)

	var b strings.Builder

	contentW := d.width - 4
	if contentW < 20 {
		contentW = 20
	}

	b.WriteString("\n")
	b.WriteString("  " + accent.Render("DIRECT MESSAGES"))
	b.WriteString("\n")
	b.WriteString("  " + dimmer.Render(strings.Repeat("─", contentW)))
	b.WriteString("\n")

	if len(d.conversations) == 0 {
		b.WriteString("\n")
		b.WriteString("  " + dim.Render("No conversations yet."))
		b.WriteString("\n\n")
		b.WriteString("  " + dim.Render("Use ") + highlight.Render("/dm @name") + dim.Render(" in ryolink to start one."))
		b.WriteString("\n")
	}

	now := time.Now()
	maxNameW := 14

	for i, c := range d.conversations {
		isCurrent := i == d.cursor

		name := c.PeerNick
		if len(name) > maxNameW {
			name = name[:maxNameW-1] + "."
		}

		// Preview text — single line, cleaned up
		preview := strings.ReplaceAll(c.LastMessage, "\n", " ")
		previewMaxW := contentW - maxNameW - 14
		if previewMaxW < 10 {
			previewMaxW = 10
		}
		if len(preview) > previewMaxW {
			preview = preview[:previewMaxW-1] + "…"
		}

		// Relative time
		timeStr := inboxRelativeTime(c.LastTime, now)
		timeRendered := dimmer.Render(timeStr)

		b.WriteString("\n")

		if isCurrent {
			indicator := highlight.Render(" ▸ ")
			nameStr := amber.Render(fmt.Sprintf("%-*s", maxNameW, name))
			previewStr := sand.Render(preview)
			b.WriteString(indicator + nameStr + " " + previewStr)
		} else {
			nameStr := sand.Render(fmt.Sprintf("%-*s", maxNameW, name))
			previewStr := dim.Render(preview)
			b.WriteString("   " + nameStr + " " + previewStr)
		}

		// Unread badge + time on same line
		suffix := "  " + timeRendered
		if c.UnreadCount > 0 {
			badge := amber.Render(fmt.Sprintf(" %d new", c.UnreadCount))
			suffix = badge + suffix
		}
		b.WriteString(suffix)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString("  " + dimmer.Render("↑↓ navigate · ENTER open · TAB back to ryolink"))

	return b.String()
}

func inboxRelativeTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return t.Format("Jan 2")
	}
}
