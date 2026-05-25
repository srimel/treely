package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/srimel/treely/internal/config"
)

func TestResolvePositionalPath(t *testing.T) {
	t.Chdir(t.TempDir())
	// Use os.Getwd as the reference so we naturally agree with whatever
	// filepath.Join inside resolvePositionalPath produces — sidesteps macOS's
	// /tmp → /private/tmp symlinking.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Build a platform-correct absolute path. On Windows filepath.IsAbs requires
	// a volume prefix (e.g. "C:\"); on Unix the root is just "/".
	vol := filepath.VolumeName(cwd)
	absPath := filepath.Join(vol+string(filepath.Separator), "already", "abs")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"dot becomes cwd", ".", cwd},
		{"absolute is unchanged", absPath, absPath},
		{"relative joins cwd", "child", filepath.Join(cwd, "child")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePositionalPath(tt.in); got != tt.want {
				t.Errorf("resolvePositionalPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestResolvePositionalPathWith covers the testable inner: in particular the
// fallback when Getwd errors out, which the public resolvePositionalPath
// inherits from filepath.Abs but is hard to exercise cross-platform without
// destroying the test process's cwd.
func TestResolvePositionalPathWith(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Derive a volume prefix so all test paths are genuinely absolute on both
	// Unix ("") and Windows ("C:"). filepath.IsAbs requires a volume on Windows.
	vol := filepath.VolumeName(cwd)
	root := vol + string(filepath.Separator)
	fakeCwd := filepath.Join(root, "fixed", "cwd")

	stubErr := errors.New("simulated getwd failure")
	stubFail := func() (string, error) { return "", stubErr }
	stubOK := func() (string, error) { return fakeCwd, nil }

	absPath := filepath.Join(root, "already", "abs")
	// A dirty absolute path that filepath.Clean normalises to absClean.
	sep := string(filepath.Separator)
	absDirty := root + "a" + sep + sep + "b" + sep + ".." + sep + "c"
	absClean := filepath.Join(root, "a", "c")

	tests := []struct {
		name  string
		in    string
		getwd func() (string, error)
		want  string
	}{
		{"empty stays empty", "", stubFail, ""},
		{"absolute skips getwd", absPath, stubFail, absPath},
		{"absolute is cleaned", absDirty, stubFail, absClean},
		{"relative joins cwd", "child", stubOK, filepath.Join(fakeCwd, "child")},
		{"relative falls back on getwd error", "child", stubFail, "child"},
		{"dot falls back on getwd error", ".", stubFail, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePositionalPathWith(tt.in, tt.getwd)
			if got != tt.want {
				t.Errorf("resolvePositionalPathWith(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveConfig(t *testing.T) {
	base := &config.Config{
		ProjectPath:    "/cfg/project",
		StartupCommand: "cfg-cmd",
	}

	tests := []struct {
		name       string
		positional string
		cmd        string
		wantPath   string
		wantCmd    string
	}{
		{"no overrides", "", "", "/cfg/project", "cfg-cmd"},
		{"positional only", "/cli/project", "", "/cli/project", "cfg-cmd"},
		{"cmd only", "", "cli-cmd", "/cfg/project", "cli-cmd"},
		{"both", "/cli/project", "cli-cmd", "/cli/project", "cli-cmd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveConfig(base, tt.positional, tt.cmd)
			if got.ProjectPath != tt.wantPath {
				t.Errorf("ProjectPath = %q, want %q", got.ProjectPath, tt.wantPath)
			}
			if got.StartupCommand != tt.wantCmd {
				t.Errorf("StartupCommand = %q, want %q", got.StartupCommand, tt.wantCmd)
			}
			// resolveConfig must not mutate the caller's config.
			if base.ProjectPath != "/cfg/project" || base.StartupCommand != "cfg-cmd" {
				t.Errorf("base mutated: %+v", base)
			}
		})
	}
}

func TestResolveConfig_NilInput(t *testing.T) {
	if got := resolveConfig(nil, "/x", "y"); got != nil {
		t.Errorf("resolveConfig(nil, ...) = %+v, want nil", got)
	}
}
