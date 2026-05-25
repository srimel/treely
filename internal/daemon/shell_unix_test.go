//go:build !windows

package daemon

import (
	"testing"
)

func TestShellCommand(t *testing.T) {
	name, args := shellCommand("echo hi")
	if name != "sh" {
		t.Errorf("name = %q, want sh", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
		t.Errorf("args = %v, want [-c echo hi]", args)
	}
}
