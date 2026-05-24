package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/srimel/treely/internal/client"
	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/daemon"
)

type eventMsg client.Event
type errMsg error
type daemonRestartedMsg struct{ client *client.Client }

type QuitReason int

const (
	QuitReasonNone QuitReason = iota
	QuitReasonUser
	QuitReasonKillDaemon
	QuitReasonError
)

type Model struct {
	cfg                *config.Config
	client             *client.Client
	sockPath           string
	dir                string
	debug              bool
	worktrees          []client.Worktree
	cursor             int
	width              int
	height             int
	spinner            spinner.Model
	activating         string // path of the worktree currently being activated
	status             string
	err                error
	QuitReason         QuitReason
	ActiveWorktreeName string
}

func NewModel(cfg *config.Config, c *client.Client, sockPath, dir string, debug bool) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	return Model{cfg: cfg, client: c, sockPath: sockPath, dir: dir, debug: debug, spinner: s}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			m.client.Send(client.Command{Cmd: "list"})
			return nil
		},
		waitForEvent(m.client),
	)
}

func waitForEvent(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-c.Events
		if !ok {
			return errMsg(fmt.Errorf("daemon disconnected"))
		}
		return eventMsg(evt)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.QuitReason = QuitReasonUser
			m.ActiveWorktreeName = activeWorktreeName(m.worktrees)
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.worktrees)-1 {
				m.cursor++
			}
		case "enter", " ":
			if len(m.worktrees) > 0 && m.activating == "" {
				wt := m.worktrees[m.cursor]
				m.activating = wt.Path
				return m, tea.Batch(
					func() tea.Msg {
						m.client.Send(client.Command{Cmd: "activate", Worktree: wt.Path})
						return nil
					},
					m.spinner.Tick,
				)
			}
		case "R":
			m.status = "Restarting daemon..."
			return m, m.restartDaemonCmd()
		case "K":
			m.QuitReason = QuitReasonKillDaemon
			m.ActiveWorktreeName = activeWorktreeName(m.worktrees)
			return m, tea.Batch(
				func() tea.Msg {
					m.client.Send(client.Command{Cmd: "stop"})
					return nil
				},
				tea.Quit,
			)
		}

	case spinner.TickMsg:
		if m.activating != "" {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case daemonRestartedMsg:
		m.client = msg.client
		m.status = ""
		return m, tea.Batch(
			func() tea.Msg {
				m.client.Send(client.Command{Cmd: "list"})
				return nil
			},
			waitForEvent(m.client),
		)

	case eventMsg:
		m.activating = ""
		m.worktrees = msg.Worktrees
		return m, waitForEvent(m.client)

	case errMsg:
		m.err = msg
		m.QuitReason = QuitReasonError
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) restartDaemonCmd() tea.Cmd {
	return func() tea.Msg {
		m.client.Send(client.Command{Cmd: "stop"})
		m.client.Close()

		// Wait for the old daemon to release the socket
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(m.sockPath); os.IsNotExist(err) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if err := daemon.Fork(m.sockPath, m.dir, m.debug); err != nil {
			return errMsg(err)
		}

		c, err := client.Connect(m.sockPath)
		if err != nil {
			return errMsg(err)
		}
		return daemonRestartedMsg{client: c}
	}
}

func activeWorktreeName(worktrees []client.Worktree) string {
	for _, wt := range worktrees {
		if wt.Status == "active" {
			return wt.Name
		}
	}
	return ""
}

// renderRow lays out one worktree row with the status flush against the right
// edge: cursor + marker + " " + name + gap + status. Long names are truncated
// with "…" so a 1-cell gap before status is preserved.
func renderRow(cursor, marker, name, status string, rowWidth int, nameInactive bool) string {
	cursorW := lipgloss.Width(cursor)
	markerW := lipgloss.Width(marker)
	statusW := lipgloss.Width(status)

	fixed := cursorW + markerW + 1 + statusW

	runes := []rune(name)
	if len(runes)+fixed > rowWidth-1 {
		target := rowWidth - fixed - 1
		if target < 1 {
			target = 1
		}
		if target == 1 {
			name = "…"
		} else {
			name = string(runes[:target-1]) + "…"
		}
	}

	styledName := name
	if nameInactive {
		styledName = inactiveStyle.Render(name)
	}

	gap := rowWidth - cursorW - markerW - 1 - lipgloss.Width(styledName) - statusW
	if gap < 1 {
		gap = 1
	}

	return cursor + marker + " " + styledName + strings.Repeat(" ", gap) + status
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	projectName := "unknown"
	if m.cfg != nil {
		projectName = m.cfg.ProjectPath
	}

	width := m.width
	if width == 0 {
		width = 80
	}
	const margin = 2
	// borderStyle has 1px border + 1px padding on each side (4 total overhead).
	// contentWidth is the inner row width visible to renderRow. lipgloss's
	// .Width() includes padding (wraps at Width - padding), so we pass
	// contentWidth+2 to .Width() so the inner wrap point equals contentWidth.
	contentWidth := width - margin*2 - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Treely  %s", projectName)))
	sb.WriteString("\n\n")

	if m.status != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", m.status))
	} else {
		for i, wt := range m.worktrees {
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("▶ ")
			}
			var line string
			switch {
			case m.activating == wt.Path:
				line = renderRow(cursor, m.spinner.View(), wt.Name, spinnerStyle.Render("activating"), contentWidth, false)
			case wt.Status == "active":
				line = renderRow(cursor, activeStyle.Render("●"), wt.Name, activeStyle.Render("active"), contentWidth, false)
			default:
				line = renderRow(cursor, inactiveStyle.Render("○"), wt.Name, inactiveStyle.Render("inactive"), contentWidth, true)
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(footerStyle.Render("↑↓/jk navigate · enter/space activate · R restart daemon · K kill daemon · q quit"))

	return lipgloss.NewStyle().MarginLeft(margin).Render(
		borderStyle.Width(contentWidth + 2).Render(sb.String()),
	)
}
