package daemon

import (
	"os"
	"os/exec"
	"time"
)

type Process struct {
	cmd  *exec.Cmd
	pid  int
	done chan struct{} // closed when the process exits
}

// StartProcess spawns command in dir and immediately starts a single goroutine
// that calls cmd.Wait(). All callers (Stop, Wait) block on the done channel so
// cmd.Wait() is never called more than once.
func StartProcess(command, dir string) (*Process, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, pid: cmd.Process.Pid, done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *Process) Pid() int { return p.pid }

// Wait blocks until the process exits.
func (p *Process) Wait() {
	<-p.done
}

// Stop sends SIGTERM, waits up to 5s for the process to exit, then SIGKILLs.
func (p *Process) Stop() {
	if p.cmd.Process == nil {
		return
	}
	p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.cmd.Process.Kill()
		<-p.done
	}
}
