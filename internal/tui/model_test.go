package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
