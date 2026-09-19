package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ryolink/internal/poll"
)

// PollVoteMsg signals a vote was cast.
type PollVoteMsg struct {
	PollID      int
	OptionIndex int
}

// ─────────────────────────────────────
// Poll Vote Overlay
// ─────────────────────────────────────

type PollVoteOverlay struct {
	polls       []poll.Poll
	current     int    // which poll we're viewing
	cursor      int    // which option is highlighted
	fingerprint string // current user, to show their vote
}

func NewPollVoteOverlay(polls []poll.Poll, fingerprint string) PollVoteOverlay {
	return PollVoteOverlay{
		polls:       polls,
		fingerprint: fingerprint,
	}
}

func (v PollVoteOverlay) Update(msg tea.Msg) (PollVoteOverlay, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return v, func() tea.Msg { return CloseModalMsg{} }
		case "tab":
			if len(v.polls) > 1 {
				v.current = (v.current + 1) % len(v.polls)
				v.cursor = 0
			}
		case "up", "k":
			v.cursor--
			if v.cursor < 0 {
				p := v.currentPoll()
				if p != nil {
					v.cursor = len(p.Options) - 1
				} else {
					v.cursor = 0
				}
			}
		case "down", "j":
			p := v.currentPoll()
			if p != nil {
				v.cursor++
				if v.cursor >= len(p.Options) {
					v.cursor = 0
				}
			}
		case "enter":
			p := v.currentPoll()
			if p != nil && !p.Closed {
				pollID := p.ID
				optIdx := v.cursor
				return v, func() tea.Msg {
					return PollVoteMsg{PollID: pollID, OptionIndex: optIdx}
				}
			}
		}
	}
	return v, nil
}

func (v PollVoteOverlay) currentPoll() *poll.Poll {
	if len(v.polls) == 0 {
		return nil
	}
	return &v.polls[v.current]
}

// SetPolls updates the polls list (called when a vote broadcast arrives).
func (v *PollVoteOverlay) SetPolls(polls []poll.Poll) {
	v.polls = polls
	if v.current >= len(v.polls) {
		v.current = 0
	}
}

func (v PollVoteOverlay) View(width, height int) string {
	if len(v.polls) == 0 {
		return ""
	}

	p := v.polls[v.current]
	modalW := 42

	// Header
	headerText := fmt.Sprintf(" POLLS [%d/%d] ", v.current+1, len(v.polls))
	if p.Closed {
		headerText = fmt.Sprintf(" POLLS [%d/%d] (closed) ", v.current+1, len(v.polls))
	}
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

	dim := lipgloss.NewStyle().Foreground(ColorDim)
	accent := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")

	// Title + author
	b.WriteString("  " + accent.Render(p.Title))
	b.WriteString("\n")
	b.WriteString("  " + dim.Render(fmt.Sprintf("by %s · %d votes", p.CreatorNick, p.TotalVotes())))
	b.WriteString("\n\n")

	// Options with bars
	counts := p.VoteCount()
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	myVote := -1
	if idx, ok := p.Votes[v.fingerprint]; ok {
		myVote = idx
	}

	barW := modalW - 20
	if barW < 8 {
		barW = 8
	}

	for i, opt := range p.Options {
		isSelected := i == v.cursor
		isMyVote := i == myVote

		// Cursor marker
		marker := "  "
		if isSelected {
			marker = lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("› ")
		}

		// Check mark for your vote
		check := " "
		if isMyVote {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("✓")
		}

		// Bar
		filled := 0
		if maxCount > 0 {
			filled = counts[i] * barW / maxCount
		}
		bar := strings.Repeat("▓", filled) + strings.Repeat("░", barW-filled)

		var barStyle lipgloss.Style
		if isSelected {
			barStyle = lipgloss.NewStyle().Foreground(ColorAmber)
		} else {
			barStyle = lipgloss.NewStyle().Foreground(ColorDim)
		}

		// Option name
		name := opt
		if len(name) > 12 {
			name = name[:11] + "…"
		}

		var nameStyle lipgloss.Style
		if isSelected {
			nameStyle = lipgloss.NewStyle().Foreground(ColorAmber).Bold(true)
		} else {
			nameStyle = lipgloss.NewStyle().Foreground(ColorSand)
		}

		countStr := dim.Render(fmt.Sprintf("%d", counts[i]))

		fmt.Fprintf(&b, "%s%s %s %s %s\n",
			marker,
			check,
			nameStyle.Width(12).Render(name),
			barStyle.Render(bar),
			countStr)
	}

	// Footer
	b.WriteString("\n")
	footerFill := lipgloss.NewStyle().Foreground(ColorBorder).Render(
		strings.Repeat("╱", modalW))
	b.WriteString(footerFill)
	b.WriteString("\n")

	tab := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("TAB")
	arrows := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("↑↓")
	enter := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("ENTER")
	esc := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("ESC")

	var footerParts []string
	if len(v.polls) > 1 {
		footerParts = append(footerParts, fmt.Sprintf("%s next", tab))
	}
	footerParts = append(footerParts, fmt.Sprintf("%s select", arrows))
	if !p.Closed {
		footerParts = append(footerParts, fmt.Sprintf("%s vote", enter))
	}
	footerParts = append(footerParts, fmt.Sprintf("%s close", esc))

	b.WriteString(dim.Render("  " + strings.Join(footerParts, "  ·  ")))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Render(b.String())
}

// RenderPollCard renders a compact poll card for inline chat display.
func RenderPollCard(p *poll.Poll) string {
	accent := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	highlight := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)

	var b strings.Builder

	if p.Closed {
		b.WriteString(dim.Render("POLL (closed)"))
	} else {
		b.WriteString(highlight.Render("POLL"))
	}
	b.WriteString("\n")
	b.WriteString(accent.Render(p.Title))
	b.WriteString("\n")

	if p.Closed {
		counts := p.VoteCount()
		maxCount := 0
		for _, c := range counts {
			if c > maxCount {
				maxCount = c
			}
		}
		for i, opt := range p.Options {
			filled := 0
			if maxCount > 0 {
				filled = counts[i] * 8 / maxCount
			}
			bar := strings.Repeat("▓", filled) + strings.Repeat("░", 8-filled)
			b.WriteString(dim.Render(fmt.Sprintf("%s %s %d", opt, bar, counts[i])))
			b.WriteString("\n")
		}
		b.WriteString(dim.Render(fmt.Sprintf("%d votes", p.TotalVotes())))
	} else {
		nums := []string{"❶", "❷", "❸", "❹"}
		for i, opt := range p.Options {
			b.WriteString(dim.Render(nums[i]+" "+opt) + "\n")
		}
		b.WriteString(dim.Render(fmt.Sprintf("/vote to cast · %d votes", p.TotalVotes())))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Render(b.String())
}
