package main

import (
	"io"
	"os"
	"strings"

	"github.com/arinprajapati/getsloth/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	sessionModeBrandStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	sessionModeTitleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	sessionModeMutedStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sessionModeSelectedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	sessionModeSelectedCardStyle  = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("51")).
					Background(lipgloss.Color("23")).
					Padding(1, 1)
	sessionModeCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(1, 1)
)

type sessionModeOption struct {
	mode        string
	label       string
	description string
}

var sessionModeOptions = []sessionModeOption{
	{
		mode:        protocol.SessionModeRemote,
		label:       "REMOTE",
		description: "One viewer · watch + steer",
	},
	{
		mode:        protocol.SessionModeGroup,
		label:       "GROUP",
		description: "Viewers + chat · host drives",
	},
}

type sessionModePicker struct {
	selected  int
	width     int
	height    int
	confirmed bool
	cancelled bool
}

func newSessionModePicker(width int) sessionModePicker {
	if width <= 0 {
		width = 80
	}
	return sessionModePicker{width: width}
}

func (m sessionModePicker) Init() tea.Cmd {
	return nil
}

func (m sessionModePicker) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.selected = max(m.selected-1, 0)
		case "down", "j":
			m.selected = min(m.selected+1, len(sessionModeOptions)-1)
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m sessionModePicker) View() string {
	return renderSessionModePicker(m)
}

func (m sessionModePicker) selectedMode() string {
	if m.selected < 0 || m.selected >= len(sessionModeOptions) {
		return protocol.SessionModeRemote
	}
	return sessionModeOptions[m.selected].mode
}

func renderSessionModePicker(model sessionModePicker) string {
	width := model.width
	if width <= 0 {
		width = 80
	}

	header := sessionModeBrandStyle.Render("GETSLOTH") + sessionModeMutedStyle.Render(" / START SESSION")
	title := sessionModeTitleStyle.Render("How do you want to connect?")
	cards := renderSessionModeCards(model.selected, width)
	footer := "↑↓ choose  enter select  q quit"
	if lipgloss.Width(footer) > width {
		footer = "↑↓ move  enter select"
	}

	return strings.Join([]string{
		header,
		"",
		title,
		"",
		cards,
		"",
		sessionModeMutedStyle.Render(footer),
	}, "\n")
}

func renderSessionModeCards(selected, width int) string {
	if len(sessionModeOptions) == 0 {
		return ""
	}

	const cardGap = 2
	if width >= 70 {
		cardWidth := (width - cardGap) / 2
		left := renderSessionModeCard(sessionModeOptions[0], selected == 0, cardWidth)
		right := renderSessionModeCard(sessionModeOptions[1], selected == 1, cardWidth)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", cardGap), right)
	}

	cards := make([]string, 0, len(sessionModeOptions))
	for index, option := range sessionModeOptions {
		cards = append(cards, renderSessionModeCard(option, selected == index, width))
	}
	return strings.Join(cards, "\n")
}

func renderSessionModeCard(option sessionModeOption, selected bool, width int) string {
	const horizontalOverhead = 4 // two borders and two padding columns
	innerWidth := max(width-horizontalOverhead, 1)
	cursor := " "
	titleStyle := sessionModeMutedStyle
	cardStyle := sessionModeCardStyle
	if selected {
		cursor = "❯"
		titleStyle = sessionModeSelectedTitleStyle
		cardStyle = sessionModeSelectedCardStyle
	}

	label := titleStyle.Render(cursor + " " + option.label)
	description := truncateDashboardText(option.description, max(innerWidth-2, 1))
	content := strings.Join([]string{label, "  " + sessionModeMutedStyle.Render(description)}, "\n")
	return cardStyle.Width(innerWidth).Height(2).Render(content)
}

func chooseSessionMode(in io.Reader, out io.Writer) (string, bool, error) {
	program := tea.NewProgram(
		newSessionModePicker(80),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	finalModel, err := program.Run()
	if err != nil {
		return "", false, err
	}

	picker, ok := finalModel.(sessionModePicker)
	if !ok || !picker.confirmed || picker.cancelled {
		return "", false, nil
	}
	return picker.selectedMode(), true, nil
}

func hasExplicitSessionMode(args []string) bool {
	return len(args) > 1 && (args[1] == "--remote" || args[1] == "--group")
}

func shouldPromptForSessionMode(args []string, input, output *os.File) bool {
	if hasExplicitSessionMode(args) || os.Getenv("GETSLOTH_NO_TUI") == "1" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if input == nil || output == nil {
		return false
	}
	return term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(output.Fd()))
}
