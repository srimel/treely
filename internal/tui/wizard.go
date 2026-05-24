package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/srimel/treely/internal/config"
)

type WizardModel struct {
	picker       filepicker.Model
	cmdInput     textinput.Model
	selectedPath string
	selectError  string
	focused      int
	width        int
	height       int
	Result       *config.Config
}

func NewWizardModel() WizardModel {
	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	fp.CurrentDirectory = home
	fp.AutoHeight = false
	fp.SetHeight(12)

	cmdInput := textinput.New()
	cmdInput.Placeholder = "npm run dev"
	cmdInput.Prompt = "❯ "
	cmdInput.CharLimit = 256

	return WizardModel{
		picker:   fp,
		cmdInput: cmdInput,
		focused:  0,
	}
}

func (m WizardModel) Init() tea.Cmd {
	return tea.Batch(m.picker.Init(), textinput.Blink)
}

func (m WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if key == "tab" {
				m.focused = (m.focused + 1) % 2
			} else {
				m.focused = (m.focused - 1 + 2) % 2
			}
			if m.focused == 1 {
				m.cmdInput.Focus()
			} else {
				m.cmdInput.Blur()
			}
			return m, nil
		case "enter":
			if m.focused == 1 {
				cmd := strings.TrimSpace(m.cmdInput.Value())
				if m.selectedPath == "" || cmd == "" {
					return m, nil
				}
				m.Result = &config.Config{
					ProjectPath:    m.selectedPath,
					StartupCommand: cmd,
				}
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	if m.focused == 0 {
		m.picker, cmd = m.picker.Update(msg)
		if ok, path := m.picker.DidSelectFile(msg); ok {
			if isTreelyCompatible(path) {
				m.selectedPath = path
				m.selectError = ""
			} else {
				m.selectedPath = ""
				m.selectError = path
			}
			// Bubbles v1 filepicker descends into the directory on the same
			// enter that triggers selection. Send a synthetic Back to undo
			// the descent so the highlighted dir stays where it was. Replace
			// the descent's readDir cmd with Back's so the file list refreshes
			// to the parent rather than the descended dir.
			m.picker, cmd = m.picker.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
	} else {
		m.cmdInput, cmd = m.cmdInput.Update(msg)
	}
	return m, cmd
}

// isTreelyCompatible mirrors daemon.findGitRoot: a directory is acceptable if
// it is itself a git repository, or one of its immediate subdirectories is
// (the bare-repo-plus-linked-worktrees layout).
func isTreelyCompatible(path string) bool {
	if isGitRepo(path) {
		return true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if isGitRepo(filepath.Join(path, e.Name())) {
			return true
		}
	}
	return false
}

// isGitRepo checks whether path is a git directory using git's own layout
// rules: a `.git` entry (dir or file) for regular repos / linked worktrees,
// or the bare-repo markers (HEAD file + objects/ + refs/) at the path itself.
// Stat-based to avoid spawning `git` on every candidate during validation.
func isGitRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false
	}
	if fi, err := os.Stat(filepath.Join(path, "objects")); err != nil || !fi.IsDir() {
		return false
	}
	if fi, err := os.Stat(filepath.Join(path, "refs")); err != nil || !fi.IsDir() {
		return false
	}
	return true
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

	pathLabel := "  Project path:"
	cmdLabel := "  Startup command:"
	if m.focused == 0 {
		pathLabel = cursorStyle.Render("▶ Project path:")
	} else {
		cmdLabel = cursorStyle.Render("▶ Startup command:")
	}

	fmt.Fprintf(&sb, "%s\n", pathLabel)
	if m.focused == 0 {
		fmt.Fprintf(&sb, "%s\n", m.picker.View())
	}
	switch {
	case m.selectedPath != "":
		fmt.Fprintf(&sb, "  %s\n\n", activeStyle.Render("selected: "+m.selectedPath))
	case m.selectError != "":
		fmt.Fprintf(&sb, "  %s\n\n", errorStyle.Render("✗ not a git repository or parent of one: "+m.selectError))
	default:
		fmt.Fprintf(&sb, "  %s\n", inactiveStyle.Render("(none selected yet)"))
		fmt.Fprintf(&sb, "  %s\n\n", hintStyle.Render("press enter on a project directory containing worktrees"))
	}

	fmt.Fprintf(&sb, "%s\n", cmdLabel)
	fmt.Fprintf(&sb, "  %s\n\n", m.cmdInput.View())

	sb.WriteString(footerStyle.Render("tab to switch fields · enter to select dir / confirm · ctrl+c to quit"))
	return lipgloss.NewStyle().MarginLeft(margin).Render(
		borderStyle.Width(contentWidth).Render(sb.String()),
	)
}
