package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/srimel/treely/internal/client"
	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/daemon"
)

type eventMsg client.Event
type errMsg error
type daemonRestartedMsg struct{ client *client.Client }

type Model struct {
	cfg       *config.Config
	client    *client.Client
	sockPath  string
	dir       string
	worktrees []client.Worktree
	cursor    int
	width     int
	height    int
	status    string
	err       error
}

func NewModel(cfg *config.Config, c *client.Client, sockPath, dir string) Model {
	return Model{cfg: cfg, client: c, sockPath: sockPath, dir: dir}
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
			if len(m.worktrees) > 0 {
				wt := m.worktrees[m.cursor]
				return m, func() tea.Msg {
					m.client.Send(client.Command{Cmd: "activate", Worktree: wt.Path})
					return nil
				}
			}
		case "R":
			m.status = "Restarting daemon..."
			return m, m.restartDaemonCmd()
		case "K":
			return m, tea.Batch(
				func() tea.Msg {
					m.client.Send(client.Command{Cmd: "stop"})
					return nil
				},
				tea.Quit,
			)
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
		m.worktrees = msg.Worktrees
		return m, waitForEvent(m.client)

	case errMsg:
		m.err = msg
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

		if err := daemon.Fork(m.sockPath, m.dir); err != nil {
			return errMsg(err)
		}

		c, err := client.Connect(m.sockPath)
		if err != nil {
			return errMsg(err)
		}
		return daemonRestartedMsg{client: c}
	}
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	projectName := "unknown"
	if m.cfg != nil {
		projectName = m.cfg.ProjectPath
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Treely  %s", projectName)))
	sb.WriteString("\n\n")

	if m.status != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", m.status))
	} else {
		for i, wt := range m.worktrees {
			var line string
			if wt.Status == "active" {
				dot := activeStyle.Render("●")
				status := activeStyle.Render("active")
				line = fmt.Sprintf("  %s %-30s %s", dot, wt.Name, status)
			} else {
				dot := inactiveStyle.Render("○")
				name := inactiveStyle.Render(fmt.Sprintf("%-30s", wt.Name))
				status := inactiveStyle.Render("inactive")
				line = fmt.Sprintf("  %s %s %s", dot, name, status)
			}
			if i == m.cursor {
				line = cursorStyle.Render(line)
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(footerStyle.Render("↑↓/jk navigate · enter/space activate · R restart daemon · K kill daemon · q quit"))

	return borderStyle.Render(sb.String())
}
