package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/srimel/treely/internal/config"
)

type WizardModel struct {
	inputs  []textinput.Model
	focused int
	width   int
	height  int
	err     error
	Result  *config.Config
}

func NewWizardModel() WizardModel {
	pathInput := textinput.New()
	pathInput.Placeholder = "~/Source/my-app"
	pathInput.Focus()
	pathInput.CharLimit = 256

	cmdInput := textinput.New()
	cmdInput.Placeholder = "npm run dev"
	cmdInput.CharLimit = 256

	return WizardModel{
		inputs:  []textinput.Model{pathInput, cmdInput},
		focused: 0,
	}
}

func (m WizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if msg.String() == "tab" {
				m.focused = (m.focused + 1) % len(m.inputs)
			} else {
				m.focused = (m.focused - 1 + len(m.inputs)) % len(m.inputs)
			}
			for i := range m.inputs {
				if i == m.focused {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
		case "enter":
			path := strings.TrimSpace(m.inputs[0].Value())
			cmd := strings.TrimSpace(m.inputs[1].Value())
			if path == "" || cmd == "" {
				return m, nil
			}
			m.Result = &config.Config{
				ProjectPath:    path,
				StartupCommand: cmd,
			}
			return m, tea.Quit
		}
	}

	var cmds []tea.Cmd
	for i := range m.inputs {
		var cmd tea.Cmd
		m.inputs[i], cmd = m.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m WizardModel) View() string {
	width := m.width
	if width == 0 {
		width = 80
	}
	const margin = 2
	contentWidth := width - margin*2 - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Treely Setup"))
	sb.WriteString("\n\n")

	labels := []string{"Project path:", "Startup command:"}
	for i, label := range labels {
		sb.WriteString(fmt.Sprintf("  %s\n", label))
		sb.WriteString(fmt.Sprintf("  > %s\n\n", m.inputs[i].View()))
	}

	sb.WriteString(footerStyle.Render("tab to switch fields · enter to confirm"))
	return lipgloss.NewStyle().MarginLeft(margin).Render(
		borderStyle.Width(contentWidth).Render(sb.String()),
	)
}
