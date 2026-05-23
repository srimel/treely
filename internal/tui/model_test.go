package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/srimel/treely/internal/client"
)

func TestUpdate_QuitOnQ_RecordsActiveWorktree(t *testing.T) {
	m := Model{
		worktrees: []client.Worktree{
			{Path: "/x/a", Name: "a", Status: "inactive"},
			{Path: "/x/b", Name: "feature-auth", Status: "active"},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want %v", got.QuitReason, QuitReasonUser)
	}
	if got.ActiveWorktreeName != "feature-auth" {
		t.Errorf("ActiveWorktreeName = %q, want %q", got.ActiveWorktreeName, "feature-auth")
	}
}

func TestUpdate_QuitOnQ_NoActiveWorktree(t *testing.T) {
	m := Model{
		worktrees: []client.Worktree{
			{Path: "/x/a", Name: "a", Status: "inactive"},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want %v", got.QuitReason, QuitReasonUser)
	}
	if got.ActiveWorktreeName != "" {
		t.Errorf("ActiveWorktreeName = %q, want empty", got.ActiveWorktreeName)
	}
}

func TestUpdate_CtrlC_RecordsQuitReasonUser(t *testing.T) {
	m := Model{}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want %v", got.QuitReason, QuitReasonUser)
	}
}

func TestRenderRow(t *testing.T) {
	cases := []struct {
		name         string
		cursor       string
		marker       string
		wtName       string
		status       string
		rowWidth     int
		inactive     bool
		wantTruncate bool
	}{
		{
			name:     "short name pads to right edge",
			cursor:   "  ",
			marker:   "●",
			wtName:   "main",
			status:   "active",
			rowWidth: 40,
		},
		{
			name:         "long name truncates with ellipsis",
			cursor:       "  ",
			marker:       "○",
			wtName:       "feature/a-very-long-branch-name-for-layout-testing",
			status:       "inactive",
			rowWidth:     30,
			wantTruncate: true,
		},
		{
			name:     "ANSI-styled status keeps width correct",
			cursor:   cursorStyle.Render("▶ "),
			marker:   activeStyle.Render("●"),
			wtName:   "main",
			status:   activeStyle.Render("active"),
			rowWidth: 50,
		},
		{
			name:     "inactive-styled name still computes gap correctly",
			cursor:   "  ",
			marker:   inactiveStyle.Render("○"),
			wtName:   "feature-branch",
			status:   inactiveStyle.Render("inactive"),
			rowWidth: 60,
			inactive: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderRow(tc.cursor, tc.marker, tc.wtName, tc.status, tc.rowWidth, tc.inactive)

			if w := lipgloss.Width(got); w != tc.rowWidth {
				t.Errorf("visible width = %d, want %d (row=%q)", w, tc.rowWidth, got)
			}
			if !strings.HasSuffix(got, tc.status) {
				t.Errorf("row does not end with status %q; got %q", tc.status, got)
			}
			if tc.wantTruncate && !strings.Contains(got, "…") {
				t.Errorf("expected truncation marker in %q", got)
			}
		})
	}
}

func TestRenderRow_PreservesMinimumGap(t *testing.T) {
	// Name exactly fills the available space minus 1 cell — gap must still be ≥ 1.
	got := renderRow("  ", "●", strings.Repeat("a", 100), "active", 30, false)
	if w := lipgloss.Width(got); w != 30 {
		t.Errorf("visible width = %d, want 30", w)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("expected truncation marker in %q", got)
	}
	// At least one space separates name from status.
	if !strings.Contains(got, " active") {
		t.Errorf("expected a space before status in %q", got)
	}
}

func TestActiveWorktreeName(t *testing.T) {
	cases := []struct {
		name string
		in   []client.Worktree
		want string
	}{
		{"empty", nil, ""},
		{"none active", []client.Worktree{{Name: "a", Status: "inactive"}}, ""},
		{"one active", []client.Worktree{
			{Name: "a", Status: "inactive"},
			{Name: "b", Status: "active"},
		}, "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := activeWorktreeName(tc.in); got != tc.want {
				t.Errorf("activeWorktreeName = %q, want %q", got, tc.want)
			}
		})
	}
}
