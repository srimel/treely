//go:build windows

package daemon

import (
	"os/exec"
	"syscall"
)

// silentCommand wraps exec.Command with CREATE_NO_WINDOW so that git and other
// console-subsystem helpers spawned by the daemon don't produce visible terminal
// windows. Without this flag each git invocation (worktree list, rev-parse, etc.)
// briefly allocates a new console because the daemon itself has no console.
func silentCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	return cmd
}
