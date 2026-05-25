//go:build !windows

package daemon

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Process struct {
	cmd  *exec.Cmd
	pid  int
	pgid int
}

// StartProcess spawns command in a new process group so the entire tree
// (shell + grandchildren) can be signalled as a unit on stop.
func StartProcess(command, dir string) (*Process, error) {
	name, args := shellCommand(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		pgid = cmd.Process.Pid
	}
	return &Process{cmd: cmd, pid: cmd.Process.Pid, pgid: pgid}, nil
}

func (p *Process) Pid() int { return p.pid }

func (p *Process) Wait() error { return p.cmd.Wait() }

// Stop signals the entire process group with SIGTERM, waits up to 5 s for a
// clean exit, escalates to SIGKILL, then blocks until the group is fully gone
// so the caller can be certain no child process still holds a port.
func (p *Process) Stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.pgid, syscall.SIGTERM)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-p.pgid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	slog.Warn("SIGKILL escalation: process group did not exit after SIGTERM", "pgid", p.pgid)
	_ = syscall.Kill(-p.pgid, syscall.SIGKILL)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-p.pgid, 0); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
