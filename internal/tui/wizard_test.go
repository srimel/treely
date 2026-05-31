package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIsGitRepo_DotGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for dir with .git/", dir)
	}
}

func TestIsGitRepo_DotGitFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for dir with .git file (linked worktree)", dir)
	}
}

func TestIsGitRepo_BareLayout(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for bare-repo layout", dir)
	}
}

func TestIsGitRepo_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false for empty dir", dir)
	}
}

func TestIsGitRepo_PartialBareLayout(t *testing.T) {
	// HEAD only, missing objects/ and refs/ — should not be considered a repo.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when only HEAD exists", dir)
	}
}

func TestIsGitRepo_BareLayoutObjectsIsFile(t *testing.T) {
	// objects must be a directory; a regular file at that path should disqualify.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when objects is a file", dir)
	}
}

func TestIsGitRepo_BareLayoutRefsIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refs"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when refs is a file", dir)
	}
}

func TestIsGitRepo_NonExistent(t *testing.T) {
	if isGitRepo(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("isGitRepo on a non-existent path = true, want false")
	}
}

func TestIsTreelyCompatible_DirectRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isTreelyCompatible(dir) {
		t.Errorf("isTreelyCompatible(%q) = false, want true when dir itself is a repo", dir)
	}
}

func TestIsTreelyCompatible_BareSubdir(t *testing.T) {
	parent := t.TempDir()
	bare := filepath.Join(parent, "repo.git")
	if err := os.MkdirAll(filepath.Join(bare, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(bare, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isTreelyCompatible(parent) {
		t.Errorf("isTreelyCompatible(%q) = false, want true when an immediate subdir is a bare repo", parent)
	}
}

func TestIsTreelyCompatible_NoRepoNearby(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "junk", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isTreelyCompatible(parent) {
		t.Errorf("isTreelyCompatible(%q) = true, want false when no repo is at the dir or one level down", parent)
	}
}

// --- WizardModel.Update() state machine ---

func TestWizardUpdate_WindowSizeMsg_SetsSize(t *testing.T) {
	m := NewWizardModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.width != 100 || got.height != 50 {
		t.Errorf("size = %dx%d, want 100x50", got.width, got.height)
	}
}

func TestWizardUpdate_CtrlC_Quits(t *testing.T) {
	m := NewWizardModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) from ctrl+c")
	}
}

func TestWizardUpdate_Tab_SwitchesFocusForwardFrom0(t *testing.T) {
	m := NewWizardModel()
	if m.focused != 0 {
		t.Fatalf("initial focused = %d, want 0", m.focused)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.focused != 1 {
		t.Errorf("focused = %d after Tab from 0, want 1", got.focused)
	}
}

func TestWizardUpdate_Tab_WrapsAroundFrom1(t *testing.T) {
	m := NewWizardModel()
	m.focused = 1
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.focused != 0 {
		t.Errorf("focused = %d after Tab from 1, want 0 (wrap)", got.focused)
	}
}

func TestWizardUpdate_ShiftTab_SwitchesFocusBackwardFrom1(t *testing.T) {
	m := NewWizardModel()
	m.focused = 1
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.focused != 0 {
		t.Errorf("focused = %d after ShiftTab from 1, want 0", got.focused)
	}
}

func TestWizardUpdate_ShiftTab_WrapsAroundFrom0(t *testing.T) {
	m := NewWizardModel()
	// focused == 0 by default
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.focused != 1 {
		t.Errorf("focused = %d after ShiftTab from 0, want 1 (wrap)", got.focused)
	}
}

func TestWizardUpdate_Enter_FocusedOnCmd_BothSet_SetsResultAndQuits(t *testing.T) {
	m := NewWizardModel()
	m.focused = 1
	m.selectedPath = "/some/project"
	m.cmdInput.SetValue("npm run dev")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.Result == nil {
		t.Fatal("Result should be set after entering valid path and command")
	}
	if got.Result.ProjectPath != "/some/project" {
		t.Errorf("Result.ProjectPath = %q, want /some/project", got.Result.ProjectPath)
	}
	if got.Result.StartupCommand != "npm run dev" {
		t.Errorf("Result.StartupCommand = %q, want 'npm run dev'", got.Result.StartupCommand)
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after successful wizard completion")
	}
}

func TestWizardUpdate_Enter_FocusedOnCmd_CmdIsWhitespaceOnly_IsNoOp(t *testing.T) {
	m := NewWizardModel()
	m.focused = 1
	m.selectedPath = "/some/project"
	m.cmdInput.SetValue("   ") // TrimSpace → empty

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.Result != nil {
		t.Error("Result should not be set when trimmed command is empty")
	}
	if cmd != nil {
		t.Error("expected nil cmd when command is blank")
	}
}

func TestWizardUpdate_Enter_FocusedOnCmd_NoPath_IsNoOp(t *testing.T) {
	m := NewWizardModel()
	m.focused = 1
	m.selectedPath = ""
	m.cmdInput.SetValue("npm run dev")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.Result != nil {
		t.Error("Result should not be set when path is missing")
	}
	if cmd != nil {
		t.Error("expected nil cmd when path is empty")
	}
}

func TestWizardUpdate_Enter_FocusedOnPicker_DoesNotCompleteWizard(t *testing.T) {
	m := NewWizardModel()
	// focused == 0 (picker); enter navigates the picker, not the wizard confirm
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(WizardModel)
	if !ok {
		t.Fatalf("expected WizardModel, got %T", next)
	}
	if got.Result != nil {
		t.Error("Result should not be set when pressing enter on the file picker")
	}
}
