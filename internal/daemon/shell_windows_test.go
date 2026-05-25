//go:build windows

package daemon

import (
	"testing"
)

func TestShellCommand(t *testing.T) {
	name, args := shellCommand("echo hi")
	if name != "powershell.exe" {
		t.Errorf("name = %q, want powershell.exe", name)
	}
	want := []string{"-NoProfile", "-NonInteractive", "-Command", "echo hi"}
	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d", len(args), len(want))
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}
