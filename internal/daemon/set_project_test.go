package daemon

import (
	"strings"
	"testing"

	"github.com/srimel/treely/internal/config"
)

// newSetProjectDaemon builds a Daemon ready for handleSetProject tests in an
// isolated HOME — config.yaml and state.yaml are scoped to a temp dir, so the
// real ~/.treely is never touched.
func newSetProjectDaemon(t *testing.T, projectPath, startupCmd string) *Daemon {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return &Daemon{
		cfg: &config.Config{
			ProjectPath:    projectPath,
			StartupCommand: startupCmd,
		},
	}
}

func TestHandleSetProject_SameProjectSameCommand(t *testing.T) {
	d := newSetProjectDaemon(t, "/proj/a", "echo a")
	evt := d.handleSetProject(Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/a",
		StartupCommand: "echo a",
	})

	if evt.Notice != "" {
		t.Errorf("Notice = %q, want empty", evt.Notice)
	}
	if evt.ConfirmSwitch != nil {
		t.Errorf("ConfirmSwitch = %+v, want nil", evt.ConfirmSwitch)
	}
	if d.cfg.ProjectPath != "/proj/a" || d.cfg.StartupCommand != "echo a" {
		t.Errorf("cfg mutated: %+v", d.cfg)
	}
}

func TestHandleSetProject_SameProjectDifferentCommand(t *testing.T) {
	d := newSetProjectDaemon(t, "/proj/a", "echo a")
	evt := d.handleSetProject(Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/a",
		StartupCommand: "echo updated",
	})

	if evt.Notice == "" {
		t.Error("expected a non-empty Notice for command update")
	}
	if !strings.Contains(evt.Notice, "Startup command updated") {
		t.Errorf("Notice = %q, want it to mention startup command update", evt.Notice)
	}
	if evt.ConfirmSwitch != nil {
		t.Errorf("ConfirmSwitch = %+v, want nil", evt.ConfirmSwitch)
	}
	if d.cfg.StartupCommand != "echo updated" {
		t.Errorf("StartupCommand = %q, want %q", d.cfg.StartupCommand, "echo updated")
	}
	if d.cfg.ProjectPath != "/proj/a" {
		t.Errorf("ProjectPath = %q, want %q", d.cfg.ProjectPath, "/proj/a")
	}
}

func TestHandleSetProject_DifferentProjectNoProc(t *testing.T) {
	d := newSetProjectDaemon(t, "/proj/a", "echo a")
	evt := d.handleSetProject(Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/b",
		StartupCommand: "echo b",
	})

	if evt.Notice != "" {
		t.Errorf("Notice = %q, want empty (no proc was killed)", evt.Notice)
	}
	if evt.ConfirmSwitch != nil {
		t.Errorf("ConfirmSwitch = %+v, want nil", evt.ConfirmSwitch)
	}
	if d.cfg.ProjectPath != "/proj/b" {
		t.Errorf("ProjectPath = %q, want %q", d.cfg.ProjectPath, "/proj/b")
	}
	if d.cfg.StartupCommand != "echo b" {
		t.Errorf("StartupCommand = %q, want %q", d.cfg.StartupCommand, "echo b")
	}
}

func TestHandleSetProject_EmptyFieldsAreNoOp(t *testing.T) {
	d := newSetProjectDaemon(t, "/proj/a", "echo a")

	evt := d.handleSetProject(Command{Cmd: "set_project"})

	if evt.ConfirmSwitch != nil {
		t.Errorf("ConfirmSwitch = %+v, want nil", evt.ConfirmSwitch)
	}
	if evt.Notice != "" {
		t.Errorf("Notice = %q, want empty", evt.Notice)
	}
	if d.cfg.ProjectPath != "/proj/a" || d.cfg.StartupCommand != "echo a" {
		t.Errorf("cfg mutated by empty-field command: %+v", d.cfg)
	}
}
