//go:build !windows

package daemon

import "os/exec"

func silentCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
