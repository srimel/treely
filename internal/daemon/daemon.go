package daemon

import (
	"log/slog"
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

func Run(sockPath string, debug bool) error {
	if debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	}
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

	slog.Info("daemon started", "sock", sockPath)
	d := &Daemon{cfg: cfg, srv: srv, sockPath: sockPath}
	return srv.Accept(d.handle)
}

func (d *Daemon) handle(cmd Command) (interface{}, bool) {
	if cmd.Worktree != "" {
		slog.Debug("command received", "cmd", cmd.Cmd, "worktree", cmd.Worktree)
	} else {
		slog.Debug("command received", "cmd", cmd.Cmd)
	}
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
	slog.Info("activating worktree", "path", worktreePath)
	proc, err := StartProcess(d.cfg.StartupCommand, worktreePath)
	if err != nil {
		slog.Error("process start failed", "path", worktreePath, "err", err)
		return
	}
	slog.Info("process started", "pid", proc.Pid())
	d.proc = proc
	st := &state.State{ActiveWorktree: worktreePath, PID: proc.Pid()}
	state.Save(st)

	// Monitor for crash
	go func() {
		proc.Wait()
		// Process exited (crash or normal stop)
		if d.proc == proc {
			slog.Info("process exited", "path", worktreePath)
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
		slog.Info("stopping process")
		d.proc.Stop()
		d.proc = nil
		state.Save(&state.State{})
	}
}

func (d *Daemon) discoverWorktrees() []Worktree {
	gitRoot := findGitRoot(d.cfg.ProjectPath)
	out, err := exec.Command("git", "-C", gitRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		slog.Warn("git worktree list failed", "err", err)
		return nil
	}
	result := parseWorktrees(string(out), d.cfg.ProjectPath)
	slog.Debug("discovered worktrees", "count", len(result))
	return result
}

// findGitRoot returns path if it is a git repo, otherwise searches one level
// of subdirectories — handles the bare-repo-plus-linked-worktrees layout where
// the user points treely at the parent directory (e.g. ~/Source/my-app) rather
// than the bare repo itself (e.g. ~/Source/my-app/my-app.git).
func findGitRoot(path string) string {
	if isGitRepo(path) {
		return path
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return path
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(path, e.Name())
		if isGitRepo(sub) {
			return sub
		}
	}
	return path
}

func isGitRepo(path string) bool {
	return exec.Command("git", "-C", path, "rev-parse", "--git-dir").Run() == nil
}

func parseWorktrees(output, projectPath string) []Worktree {
	var result []Worktree
	var current Worktree
	bare := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" && !bare {
				result = append(result, current)
			}
			path := strings.TrimPrefix(line, "worktree ")
			name := filepath.Base(path)
			if path == projectPath {
				name = filepath.Base(projectPath)
			}
			current = Worktree{Path: path, Name: name, Status: "inactive"}
			bare = false
		} else if line == "bare" {
			bare = true
		}
	}
	if current.Path != "" && !bare {
		result = append(result, current)
	}
	return result
}
