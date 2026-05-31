package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/srimel/treely/internal/client"
	"github.com/srimel/treely/internal/config"
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

// --- WindowSizeMsg ---

func TestUpdate_WindowSizeMsg_SetsWidthAndHeight(t *testing.T) {
	m := Model{}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Error("expected nil cmd for WindowSizeMsg")
	}
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.width != 120 || got.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", got.width, got.height)
	}
}

// --- Confirm-switch mode key handling ---

func TestUpdate_ConfirmSwitch_YKey_ClearsMode(t *testing.T) {
	m := Model{
		mode: modeConfirmSwitch,
		pendingSwitch: &client.SwitchInfo{
			FromProject: "/old",
			ToProject:   "/new",
		},
		cfg: &config.Config{StartupCommand: "npm run dev"},
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", got.mode)
	}
	if got.pendingSwitch != nil {
		t.Error("pendingSwitch should be nil after Y confirmation")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (force set_project) after Y")
	}
}

func TestUpdate_ConfirmSwitch_YKey_NilPendingSwitch_ReturnsNilCmd(t *testing.T) {
	m := Model{
		mode:          modeConfirmSwitch,
		pendingSwitch: nil,
		cfg:           &config.Config{},
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if got.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after Y with nil switch", got.mode)
	}
	if cmd != nil {
		t.Error("expected nil cmd when pendingSwitch is nil")
	}
}

func TestUpdate_ConfirmSwitch_UpperYKey_ClearsMode(t *testing.T) {
	m := Model{
		mode:          modeConfirmSwitch,
		pendingSwitch: &client.SwitchInfo{ToProject: "/new"},
		cfg:           &config.Config{},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	got := next.(Model)
	if got.mode != modeNormal {
		t.Errorf("mode = %v after uppercase Y, want modeNormal", got.mode)
	}
}

func TestUpdate_ConfirmSwitch_NKey_Quits(t *testing.T) {
	m := Model{mode: modeConfirmSwitch}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(Model)
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want QuitReasonUser", got.QuitReason)
	}
}

func TestUpdate_ConfirmSwitch_QKey_Quits(t *testing.T) {
	m := Model{mode: modeConfirmSwitch}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := next.(Model)
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want QuitReasonUser", got.QuitReason)
	}
}

func TestUpdate_ConfirmSwitch_CtrlC_Quits(t *testing.T) {
	m := Model{mode: modeConfirmSwitch}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := next.(Model)
	if got.QuitReason != QuitReasonUser {
		t.Errorf("QuitReason = %v, want QuitReasonUser", got.QuitReason)
	}
}

func TestUpdate_ConfirmSwitch_UnhandledKey_IsNoOp(t *testing.T) {
	m := Model{mode: modeConfirmSwitch}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	got := next.(Model)
	if got.mode != modeConfirmSwitch {
		t.Errorf("mode changed on unhandled key in confirm-switch, want modeConfirmSwitch")
	}
	if got.QuitReason != QuitReasonNone {
		t.Errorf("QuitReason = %v, want QuitReasonNone", got.QuitReason)
	}
	if cmd != nil {
		t.Error("expected nil cmd for unhandled key in confirm-switch mode")
	}
}

// --- Cursor navigation ---

func TestUpdate_CursorUp_Decrements(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}, {}}, cursor: 1}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d after up, want 0", got)
	}
}

func TestUpdate_CursorK_Decrements(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}, {}}, cursor: 1}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d after k, want 0", got)
	}
}

func TestUpdate_CursorUp_AtZero_NoChange(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}}, cursor: 0}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d at top boundary, want 0", got)
	}
}

func TestUpdate_CursorDown_Increments(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}, {}}, cursor: 0}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := next.(Model).cursor; got != 1 {
		t.Errorf("cursor = %d after down, want 1", got)
	}
}

func TestUpdate_CursorJ_Increments(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}, {}}, cursor: 0}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := next.(Model).cursor; got != 1 {
		t.Errorf("cursor = %d after j, want 1", got)
	}
}

func TestUpdate_CursorDown_AtBottom_NoChange(t *testing.T) {
	m := Model{worktrees: []client.Worktree{{}, {}}, cursor: 1}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := next.(Model).cursor; got != 1 {
		t.Errorf("cursor = %d at bottom boundary, want 1", got)
	}
}

// --- Enter / Space activation ---

func TestUpdate_Enter_BeginsActivating(t *testing.T) {
	wt := client.Worktree{Path: "/x/main", Name: "main", Status: "inactive"}
	m := Model{worktrees: []client.Worktree{wt}, cursor: 0}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.activating != wt.Path {
		t.Errorf("activating = %q, want %q", got.activating, wt.Path)
	}
	if cmd == nil {
		t.Error("expected non-nil batch cmd (activate + spinner tick)")
	}
}

func TestUpdate_Space_BeginsActivating(t *testing.T) {
	wt := client.Worktree{Path: "/x/feat", Name: "feat", Status: "inactive"}
	m := Model{worktrees: []client.Worktree{wt}, cursor: 0}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	got := next.(Model)
	if got.activating != wt.Path {
		t.Errorf("activating = %q after space, want %q", got.activating, wt.Path)
	}
	if cmd == nil {
		t.Error("expected non-nil batch cmd after space activation")
	}
}

func TestUpdate_Enter_EmptyWorktrees_IsNoOp(t *testing.T) {
	m := Model{worktrees: nil}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.activating != "" {
		t.Errorf("activating = %q, want empty when no worktrees", got.activating)
	}
	if cmd != nil {
		t.Error("expected nil cmd when no worktrees")
	}
}

func TestUpdate_Enter_AlreadyActivating_IsNoOp(t *testing.T) {
	wt := client.Worktree{Path: "/x/main", Name: "main"}
	m := Model{worktrees: []client.Worktree{wt}, cursor: 0, activating: "/x/main"}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.activating != "/x/main" {
		t.Errorf("activating changed to %q, want unchanged /x/main", got.activating)
	}
	if cmd != nil {
		t.Error("expected nil cmd when already activating")
	}
}

// --- R key (daemon restart) ---

func TestUpdate_RKey_ResetsStateForRestart(t *testing.T) {
	m := Model{
		worktrees:  []client.Worktree{{Path: "/x/main"}},
		cursor:     1,
		activating: "/x/main",
		sockPath:   "/fake.sock",
		dir:        "/fake",
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	got := next.(Model)
	if len(got.worktrees) != 0 {
		t.Errorf("worktrees not cleared on restart; got %d", len(got.worktrees))
	}
	if got.cursor != 0 {
		t.Errorf("cursor = %d after restart, want 0", got.cursor)
	}
	if got.activating != "" {
		t.Errorf("activating = %q after restart, want empty", got.activating)
	}
	if got.status != "Restarting daemon..." {
		t.Errorf("status = %q, want 'Restarting daemon...'", got.status)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from R key")
	}
}

// --- K key (kill daemon) ---

func TestUpdate_KKey_SetsKillQuitReason(t *testing.T) {
	m := Model{
		worktrees: []client.Worktree{
			{Name: "main", Status: "active"},
		},
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	got := next.(Model)
	if got.QuitReason != QuitReasonKillDaemon {
		t.Errorf("QuitReason = %v, want QuitReasonKillDaemon", got.QuitReason)
	}
	if got.ActiveWorktreeName != "main" {
		t.Errorf("ActiveWorktreeName = %q, want 'main'", got.ActiveWorktreeName)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (stop + quit batch)")
	}
}

// --- Spinner tick ---

func TestUpdate_SpinnerTick_WhenActivating_ContinuesTicking(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{activating: "/x/main", spinner: s}
	_, cmd := m.Update(m.spinner.Tick())
	if cmd == nil {
		t.Error("expected follow-up spinner cmd while activating")
	}
}

func TestUpdate_SpinnerTick_WhenNotActivating_IsNoOp(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{activating: "", spinner: s}
	_, cmd := m.Update(m.spinner.Tick())
	if cmd != nil {
		t.Errorf("expected nil cmd when spinner ticks but not activating")
	}
}

// --- eventMsg handling ---

func TestUpdate_EventMsg_SetsWorktreesAndNotice(t *testing.T) {
	m := Model{activating: "/x/old"}
	evt := eventMsg{
		Event: "state_changed",
		Worktrees: []client.Worktree{
			{Path: "/x/main", Name: "main", Status: "active"},
		},
		Notice: "switched",
	}
	next, cmd := m.Update(evt)
	got := next.(Model)
	if len(got.worktrees) != 1 {
		t.Errorf("worktrees = %d, want 1", len(got.worktrees))
	}
	if got.status != "switched" {
		t.Errorf("status = %q, want 'switched'", got.status)
	}
	if got.activating != "" {
		t.Error("activating should be cleared by incoming event")
	}
	if cmd == nil {
		t.Error("expected waitForEvent cmd re-armed after event")
	}
}

func TestUpdate_EventMsg_ConfirmSwitch_EntersConfirmMode(t *testing.T) {
	m := Model{}
	evt := eventMsg{
		ConfirmSwitch: &client.SwitchInfo{
			FromProject: "/old",
			ToProject:   "/new",
		},
	}
	next, cmd := m.Update(evt)
	got := next.(Model)
	if got.mode != modeConfirmSwitch {
		t.Errorf("mode = %v, want modeConfirmSwitch", got.mode)
	}
	if got.pendingSwitch == nil {
		t.Error("pendingSwitch should be populated from ConfirmSwitch event")
	}
	if got.pendingSwitch.ToProject != "/new" {
		t.Errorf("pendingSwitch.ToProject = %q, want /new", got.pendingSwitch.ToProject)
	}
	if cmd == nil {
		t.Error("expected waitForEvent cmd after entering confirm-switch mode")
	}
}

// --- errMsg handling ---

func TestUpdate_ErrMsg_SetsErrorAndQuits(t *testing.T) {
	m := Model{}
	next, cmd := m.Update(errMsg(fmt.Errorf("daemon disconnected")))
	got := next.(Model)
	if got.QuitReason != QuitReasonError {
		t.Errorf("QuitReason = %v, want QuitReasonError", got.QuitReason)
	}
	if got.err == nil {
		t.Error("err should be set after errMsg")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after errMsg")
	}
}

// --- daemonRestartedMsg handling ---

func TestUpdate_DaemonRestartedMsg_ClearsStatus(t *testing.T) {
	m := Model{
		status: "Restarting daemon...",
		cfg:    &config.Config{},
	}
	msg := daemonRestartedMsg{client: nil}
	next, cmd := m.Update(msg)
	got := next.(Model)
	if got.status != "" {
		t.Errorf("status = %q after daemon restart, want empty", got.status)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (set_project + waitForEvent) after daemon restart")
	}
}

// --- renderConfirmSwitch ---

func TestRenderConfirmSwitch_Nil_ReturnsEmpty(t *testing.T) {
	if got := renderConfirmSwitch(nil); got != "" {
		t.Errorf("renderConfirmSwitch(nil) = %q, want empty string", got)
	}
}

func TestRenderConfirmSwitch_IncludesProjectsAndCommand(t *testing.T) {
	si := &client.SwitchInfo{
		FromProject:    "/old/project",
		ToProject:      "/new/project",
		RunningCommand: "npm run dev",
	}
	got := renderConfirmSwitch(si)
	for _, want := range []string{"/old/project", "/new/project", "npm run dev"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderConfirmSwitch output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRenderConfirmSwitch_WithActiveWorktree_IncludesActiveLine(t *testing.T) {
	si := &client.SwitchInfo{
		FromProject:    "/old",
		ToProject:      "/new",
		RunningCommand: "make dev",
		ActiveWorktree: "feature-branch",
	}
	got := renderConfirmSwitch(si)
	if !strings.Contains(got, "feature-branch") {
		t.Errorf("expected ActiveWorktree in output; got:\n%s", got)
	}
	if !strings.Contains(got, "Active:") {
		t.Errorf("expected 'Active:' label when ActiveWorktree is set; got:\n%s", got)
	}
}

func TestRenderConfirmSwitch_WithoutActiveWorktree_NoActiveLine(t *testing.T) {
	si := &client.SwitchInfo{
		FromProject:    "/old",
		ToProject:      "/new",
		RunningCommand: "make dev",
		ActiveWorktree: "",
	}
	got := renderConfirmSwitch(si)
	if strings.Contains(got, "Active:") {
		t.Errorf("should not include 'Active:' line when ActiveWorktree is empty; got:\n%s", got)
	}
}
