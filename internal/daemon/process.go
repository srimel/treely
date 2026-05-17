package daemon

import (
	"os"
	"os/exec"
	"time"
)

type Process struct {
	cmd *exec.Cmd
	pid int
}

// StartProcess spawns cmd in dir, returns the running Process.
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

// Wait waits for the process to exit.
func (p *Process) Wait() error { return p.cmd.Wait() }

// Stop sends SIGTERM, waits 5s, then SIGKILLs.
func (p *Process) Stop() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	p.cmd.Process.Signal(os.Interrupt) // SIGTERM on unix
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		p.cmd.Process.Kill()
		<-done
	}
}
