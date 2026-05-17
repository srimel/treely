package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	// Use a temp directory as the home directory
	tmpDir := t.TempDir()
	trelyDir := filepath.Join(tmpDir, ".treely")

	// Override Dir() by temporarily patching UserHomeDir via env
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &Config{
		ProjectPath:    "/tmp/my-project",
		StartupCommand: "npm run dev",
	}

	if err := os.MkdirAll(trelyDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ProjectPath != cfg.ProjectPath {
		t.Errorf("ProjectPath mismatch: got %q, want %q", loaded.ProjectPath, cfg.ProjectPath)
	}
	if loaded.StartupCommand != cfg.StartupCommand {
		t.Errorf("StartupCommand mismatch: got %q, want %q", loaded.StartupCommand, cfg.StartupCommand)
	}
}
