package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	trelyDir := filepath.Join(tmpDir, ".treely")

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows uses USERPROFILE instead of HOME

	if err := os.MkdirAll(trelyDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := &State{
		ActiveWorktree: "/tmp/my-project/feature-a",
		PID:            12345,
	}

	if err := Save(s); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ActiveWorktree != s.ActiveWorktree {
		t.Errorf("ActiveWorktree mismatch: got %q, want %q", loaded.ActiveWorktree, s.ActiveWorktree)
	}
	if loaded.PID != s.PID {
		t.Errorf("PID mismatch: got %d, want %d", loaded.PID, s.PID)
	}
}

func TestLoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	trelyDir := filepath.Join(tmpDir, ".treely")

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows uses USERPROFILE instead of HOME

	// Ensure the directory does NOT have state.yaml
	_ = os.MkdirAll(trelyDir, 0755)

	s, err := Load()
	if err != nil {
		t.Fatalf("expected no error for missing state file, got: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil State for missing file")
	}
	if s.ActiveWorktree != "" || s.PID != 0 {
		t.Errorf("expected empty state, got: %+v", s)
	}
}
