package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/state"
)

type Daemon struct {
	cfg      *config.Config
	srv      *Server
	proc     *Process
	sockPath string
}

func Run(sockPath string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv, err := NewServer(sockPath)
	if err != nil {
		return err
	}
	defer func() {
		srv.Close()
		os.Remove(sockPath)
	}()

	d := &Daemon{cfg: cfg, srv: srv, sockPath: sockPath}
	return srv.Accept(d.handle)
}

func (d *Daemon) handle(cmd Command) (interface{}, bool) {
	switch cmd.Cmd {
	case "list":
		return d.listResponse(), true
	case "activate":
		d.activate(cmd.Worktree)
		return nil, true
	case "stop":
		d.stopProcess()
		return nil, false
	}
	return nil, true
}

func (d *Daemon) listResponse() Event {
	worktrees := d.discoverWorktrees()
	st, _ := state.Load()
	for i := range worktrees {
		if st != nil && worktrees[i].Path == st.ActiveWorktree {
			if d.proc != nil {
				worktrees[i].Status = "active"
			}
		}
	}
	return Event{Event: "state_changed", Worktrees: worktrees}
}

func (d *Daemon) activate(worktreePath string) {
	d.stopProcess()
	proc, err := StartProcess(d.cfg.StartupCommand, worktreePath)
	if err != nil {
		return
	}
	d.proc = proc
	st := &state.State{ActiveWorktree: worktreePath, PID: proc.Pid()}
	state.Save(st)

	// Monitor for crash
	go func() {
		proc.Wait()
		// Process exited (crash or normal stop)
		if d.proc == proc {
			d.proc = nil
			st := &state.State{}
			state.Save(st)
			d.srv.Push(d.listResponse())
		}
	}()

	d.srv.Push(d.listResponse())
}

func (d *Daemon) stopProcess() {
	if d.proc != nil {
		d.proc.Stop()
		d.proc = nil
		state.Save(&state.State{})
	}
}

func (d *Daemon) discoverWorktrees() []Worktree {
	out, err := exec.Command("git", "-C", d.cfg.ProjectPath, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	return parseWorktrees(string(out), d.cfg.ProjectPath)
}

func parseWorktrees(output, projectPath string) []Worktree {
	var result []Worktree
	var current Worktree
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				result = append(result, current)
			}
			path := strings.TrimPrefix(line, "worktree ")
			name := filepath.Base(path)
			if path == projectPath {
				name = filepath.Base(projectPath)
			}
			current = Worktree{Path: path, Name: name, Status: "inactive"}
		}
	}
	if current.Path != "" {
		result = append(result, current)
	}
	return result
}
