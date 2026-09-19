package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ryolink/internal/chat"
	"ryolink/internal/sudoku"
)

const (
	// Board with 4-char wide cells: │ XX │ XX │ XX ║ XX │ ...
	// 9 cells × 4 chars + 4 separators (│ at edges and ║ at box boundaries) = 40 chars
	renderedBoardW = 40
	checkFlashMs   = 5000
)

type SudokuView struct {
	game        *sudoku.Game
	fingerprint string
	nickname    string
	colorIndex  int
	cursorRow   int
	cursorCol   int
	focusChat   bool // true = typing in chat, false = board
	input       textinput.Model
	messages    []chat.Message
	wrongCells  map[[2]int]time.Time // flashing wrong cells from check
	width       int
	height      int
}

func NewSudokuView(game *sudoku.Game, fingerprint, nickname string, colorIndex int) SudokuView {
	ti := textinput.New()
	ti.Placeholder = "Chat..."
	ti.CharLimit = 200
	ti.Prompt = "  > "

	return SudokuView{
		game:        game,
		fingerprint: fingerprint,
		nickname:    nickname,
		colorIndex:  colorIndex,
		wrongCells:  make(map[[2]int]time.Time),
		input:       ti,
	}
}

func (s *SudokuView) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *SudokuView) AddMessage(msg chat.Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	s.messages = append(s.messages, msg)
	if len(s.messages) > 50 {
		s.messages = s.messages[len(s.messages)-50:]
	}
}

func (s *SudokuView) MarkWrong(positions []sudoku.Position) {
	now := time.Now()
	for _, p := range positions {
		s.wrongCells[[2]int{p.Row, p.Col}] = now
	}
}

func (s *SudokuView) Tick() {
	now := time.Now()
	for k, t := range s.wrongCells {
		if now.Sub(t) > checkFlashMs*time.Millisecond {
			delete(s.wrongCells, k)
		}
	}
}

func (s SudokuView) Update(msg tea.Msg) (SudokuView, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	key := keyMsg.String()

	if key == "tab" {
		s.focusChat = !s.focusChat
		if s.focusChat {
			s.input.Focus()
		} else {
			s.input.Blur()
		}
		return s, nil
	}

	if s.focusChat {
		switch key {
		case "enter":
			// handled by app
		case "esc":
			s.focusChat = false
			s.input.Blur()
			return s, nil
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
		return s, nil
	}

	switch key {
	case "up", "k":
		if s.cursorRow > 0 {
			s.cursorRow--
		}
		s.game.SetCursor(s.fingerprint, s.cursorRow, s.cursorCol)
	case "down", "j":
		if s.cursorRow < 8 {
			s.cursorRow++
		}
		s.game.SetCursor(s.fingerprint, s.cursorRow, s.cursorCol)
	case "left", "h":
		if s.cursorCol > 0 {
			s.cursorCol--
		}
		s.game.SetCursor(s.fingerprint, s.cursorRow, s.cursorCol)
	case "right", "l":
		if s.cursorCol < 8 {
			s.cursorCol++
		}
		s.game.SetCursor(s.fingerprint, s.cursorRow, s.cursorCol)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		digit := int(key[0] - '0')
		s.game.Place(s.fingerprint, s.cursorRow, s.cursorCol, digit)
	case "x", "backspace", "delete":
		s.game.Clear(s.fingerprint, s.cursorRow, s.cursorCol)
	}

	return s, nil
}

func (s *SudokuView) ChatInput() string {
	val := s.input.Value()
	s.input.Reset()
	return val
}

func (s SudokuView) HasChatInput() bool {
	return s.input.Value() != ""
}

func (s SudokuView) FocusChat() bool {
	return s.focusChat
}

func (s SudokuView) View() string {
	board := s.game.Board()
	cursors := s.game.Cursors()

	// Board takes left side, chat takes the rest
	chatW := s.width - renderedBoardW - 8
	if chatW < 20 {
		chatW = 20
	}

	boardView := s.renderBoard(board, cursors)

	// Board sets the content height
	boardLines := strings.Split(boardView, "\n")
	contentH := len(boardLines)

	chatView := s.renderChat(chatW, contentH)

	// Join side by side
	chatLines := strings.Split(chatView, "\n")

	sep := lipgloss.NewStyle().Foreground(ColorDimmer)
	var combined strings.Builder
	for i := 0; i < contentH; i++ {
		bl := ""
		if i < len(boardLines) {
			bl = boardLines[i]
		}
		// Pad board line to consistent visual width
		visW := lipgloss.Width(bl)
		if visW < renderedBoardW {
			bl += strings.Repeat(" ", renderedBoardW-visW)
		}
		cl := ""
		if i < len(chatLines) {
			cl = chatLines[i]
		}
		combined.WriteString(bl)
		combined.WriteString(sep.Render(" │ "))
		combined.WriteString(cl)
		combined.WriteString("\n")
	}

	dim := lipgloss.NewStyle().Foreground(ColorDim)
	hl := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)

	// Scoreboard — top 5
	topScores := s.game.TopScores(5)
	var scoreParts []string
	for i, e := range topScores {
		medal := dim.Render(fmt.Sprintf("%d.", i+1))
		name := lipgloss.NewStyle().Foreground(ColorSand).Render(truncateWidth(e.Nickname, 18))
		pts := hl.Render(fmt.Sprintf("%d", e.Score))
		scoreParts = append(scoreParts, fmt.Sprintf("%s %s %s", medal, name, pts))
	}
	scoreBoard := ""
	if len(scoreParts) > 0 {
		scoreBoard = "  " + strings.Join(scoreParts, dim.Render("  ·  "))
	}

	// Status
	statusLine := "  " + dim.Render(fmt.Sprintf("%s · %d/81",
		strings.ToUpper(s.game.Difficulty()[:1])+s.game.Difficulty()[1:],
		s.game.Filled()))

	// Help line
	k := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)
	d := lipgloss.NewStyle().Foreground(ColorDim)
	helpLine := "  " + k.Render("←→↑↓") + d.Render(" move  ") +
		k.Render("1-9") + d.Render(" place  ") +
		k.Render("x") + d.Render(" clear  ") +
		k.Render("Tab") + d.Render(" chat  ") +
		k.Render("ESC") + d.Render(" back")

	inner := combined.String()
	if scoreBoard != "" {
		inner += "\n" + scoreBoard
	}
	inner += "\n" + statusLine + "\n" + helpLine

	return ChatBorderStyle.Width(s.width).Height(s.height).Padding(1, 1).Render(inner)
}

func (s SudokuView) renderBoard(board [9][9]sudoku.Cell, cursors map[string]sudoku.Position) string {
	clue := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	empty := lipgloss.NewStyle().Foreground(ColorDimmer)
	wrong := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	cursorBg := lipgloss.NewStyle().Background(ColorBorder).Foreground(lipgloss.Color("255")).Bold(true)
	grid := lipgloss.NewStyle().Foreground(ColorDimmer)
	boxGrid := lipgloss.NewStyle().Foreground(ColorBorder)

	var b strings.Builder

	// Top border
	b.WriteString(grid.Render("╔════════════╦════════════╦════════════╗"))
	b.WriteString("\n")

	for r := 0; r < 9; r++ {
		b.WriteString(grid.Render("║"))
		for c := 0; c < 9; c++ {
			cell := board[r][c]
			_, isWrong := s.wrongCells[[2]int{r, c}]
			isCursor := r == s.cursorRow && c == s.cursorCol

			var cellStr string
			if cell.Value == 0 {
				cellStr = " · "
			} else {
				cellStr = fmt.Sprintf(" %d ", cell.Value)
			}

			// Style the cell content
			var styled string
			switch {
			case isCursor:
				styled = cursorBg.Render(cellStr)
			case isWrong:
				styled = wrong.Render(cellStr)
			case cell.IsClue:
				styled = clue.Render(cellStr)
			case cell.Locked:
				// Correct placement — show in green
				styled = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render(cellStr)
			case cell.Value == 0:
				styled = empty.Render(cellStr)
			default:
				styled = NickStyle(s.colorIndex).Render(cellStr)
			}

			b.WriteString(styled)

			// Column separator
			if c == 2 || c == 5 {
				b.WriteString(boxGrid.Render("║"))
			} else if c < 8 {
				b.WriteString(grid.Render("│"))
			}
		}
		b.WriteString(grid.Render("║"))
		b.WriteString("\n")

		// Row separators
		if r == 2 || r == 5 {
			b.WriteString(boxGrid.Render("╠════════════╬════════════╬════════════╣"))
			b.WriteString("\n")
		} else if r < 8 {
			b.WriteString(grid.Render("║────────────║────────────║────────────║"))
			b.WriteString("\n")
		}
	}

	// Bottom border
	b.WriteString(grid.Render("╚════════════╩════════════╩════════════╝"))

	return b.String()
}

func (s SudokuView) renderChat(width, totalHeight int) string {
	header := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	dimmer := lipgloss.NewStyle().Foreground(ColorDimmer)

	// Fixed structure: header(1) + sep(1) + messages + sep(1) + input(1) = totalHeight
	msgHeight := totalHeight - 4
	if msgHeight < 2 {
		msgHeight = 2
	}

	var lines []string

	// Header
	lines = append(lines, header.Render("GAME CHAT"))
	lines = append(lines, dimmer.Render(strings.Repeat("─", width)))

	// Messages — bottom-aligned
	var msgLines []string
	for _, msg := range s.messages {
		if msg.IsSystem {
			msgLines = append(msgLines, dim.Render(truncateWidth(msg.Text, width)))
		} else {
			nick := NickStyle(msg.ColorIndex).Render(truncateWidth(msg.Nickname, 15))
			text := truncateWidth(msg.Text, width-17)
			msgLines = append(msgLines, fmt.Sprintf("%s %s", nick, dim.Render(text)))
		}
	}
	// Only show the last msgHeight messages
	if len(msgLines) > msgHeight {
		msgLines = msgLines[len(msgLines)-msgHeight:]
	}
	// Pad top with empty lines so messages are bottom-aligned
	for len(msgLines) < msgHeight {
		msgLines = append([]string{""}, msgLines...)
	}
	lines = append(lines, msgLines...)

	// Separator + input
	lines = append(lines, dimmer.Render(strings.Repeat("─", width)))
	if s.focusChat {
		inputStr := s.input.View()
		lines = append(lines, truncateWidth(inputStr, width))
	} else {
		lines = append(lines, dim.Render(" Tab to chat..."))
	}

	return strings.Join(lines, "\n")
}
