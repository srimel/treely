package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/Source/my-app", filepath.Join(home, "Source/my-app")},
		{"~/", home},
		{"~", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~user/path", "~user/path"}, // only leading ~/ is expanded
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadExpandsHomeTilde(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows uses USERPROFILE instead of HOME

	trelyDir := filepath.Join(tmpDir, ".treely")
	if err := os.MkdirAll(trelyDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := Save(&Config{
		ProjectPath:    "~/Source/my-app",
		StartupCommand: "npm run dev",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if strings.HasPrefix(loaded.ProjectPath, "~") {
		t.Errorf("Load() did not expand tilde: got %q", loaded.ProjectPath)
	}
	want := filepath.Join(tmpDir, "Source/my-app")
	if loaded.ProjectPath != want {
		t.Errorf("ProjectPath = %q, want %q", loaded.ProjectPath, want)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	trelyDir := filepath.Join(tmpDir, ".treely")

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows uses USERPROFILE instead of HOME

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
