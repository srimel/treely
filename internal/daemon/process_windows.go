//go:build windows

package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

type Process struct {
	cmd *exec.Cmd
	pid int
}

func StartProcess(command, dir string) (*Process, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Process{cmd: cmd, pid: cmd.Process.Pid}, nil
}

func (p *Process) Pid() int { return p.pid }

func (p *Process) Wait() error { return p.cmd.Wait() }

// Stop kills the entire process tree via taskkill then waits for the root
// process to exit.
func (p *Process) Stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if err := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", p.pid)).Run(); err != nil {
		slog.Warn("taskkill failed", "pid", p.pid, "err", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		p.cmd.Process.Kill()
		<-done
	}
}
