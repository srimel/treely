package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/state"
)

type Daemon struct {
	cfg      *config.Config
	srv      *Server
	proc     *Process
	sockPath string
	shutdown chan struct{}
	stopOnce sync.Once
}

// Run starts the daemon, binding to sockPath and writing a PID file to dir.
// It returns after a clean shutdown (signal or stop command) or a fatal error.
func Run(sockPath, dir string, debug bool) error {
	if debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Belt-and-braces: terminate any predecessor that didn't clean up its own
	// PID file (e.g. it was SIGKILLed or crashed).
	if err := TerminateExistingDaemon(dir, 5*time.Second); err != nil {
		slog.Warn("predecessor daemon may still be alive; proceeding", "err", err)
	}

	srv, err := NewServer(sockPath)
	if err != nil {
		if runtime.GOOS == "windows" && isAFUnixError(err) {
			return fmt.Errorf("AF_UNIX socket support requires Windows 10 1803+ or Windows Server 2019+: %w", err)
		}
		return err
	}

	if err := WritePIDFile(dir); err != nil {
		srv.Close()
		os.Remove(sockPath)
		return err
	}

	sigCh, stopSig := notifyShutdown()
	defer stopSig()

	slog.Info("daemon started", "sock", sockPath)
	d := &Daemon{
		cfg:      cfg,
		srv:      srv,
		sockPath: sockPath,
		shutdown: make(chan struct{}),
	}

	acceptDone := make(chan error, 1)
	go func() {
		acceptDone <- srv.Accept(d.handle)
	}()

	var runErr error
	acceptExited := false
	select {
	case <-sigCh:
		slog.Info("daemon shutting down", "reason", "signal")
	case <-d.shutdown:
		slog.Info("daemon shutting down", "reason", "stop command")
	case runErr = <-acceptDone:
		acceptExited = true
		if errors.Is(runErr, net.ErrClosed) {
			runErr = nil
		} else if runErr != nil {
			slog.Error("accept error", "err", runErr)
		}
	}

	// Stop child before closing the listener so it doesn't hold ports across
	// the next worktree activation.
	d.stopProcess()
	srv.Close()
	os.Remove(sockPath)
	RemovePIDFile(dir)

	if !acceptExited {
		select {
		case <-acceptDone:
		case <-time.After(time.Second):
		}
	}

	return runErr
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
	case "set_project":
		return d.handleSetProject(cmd), true
	case "activate":
		d.activate(cmd.Worktree)
		return nil, true
	case "stop":
		d.stopProcess()
		d.stopOnce.Do(func() { close(d.shutdown) })
		return nil, false
	}
	return nil, true
}

// handleSetProject is the TUI's connect-time handshake. It reconciles the
// daemon's in-memory cfg with what the TUI was launched with: same project +
// same command is a no-op; same project + different command hot-updates the
// startup command without killing the running dev server; a different project
// either switches (when nothing is running, or when Force is set) or returns a
// ConfirmSwitch event for the TUI to prompt the user.
//
// The daemon's cfg is mutated here at runtime — config.yaml on disk is never
// touched. Sessions-only overrides are the whole point of this command.
func (d *Daemon) handleSetProject(cmd Command) Event {
	// Defense in depth: an empty ProjectPath or StartupCommand from a
	// malformed message would otherwise blank out d.cfg in the
	// "different project, no proc" branch below.
	if cmd.ProjectPath == "" || cmd.StartupCommand == "" {
		slog.Warn("set_project rejected: empty fields",
			"project_path", cmd.ProjectPath,
			"startup_command", cmd.StartupCommand)
		return d.listResponse()
	}

	sameProject := cmd.ProjectPath == d.cfg.ProjectPath
	sameCommand := cmd.StartupCommand == d.cfg.StartupCommand

	if sameProject {
		if sameCommand {
			return d.listResponse()
		}
		d.cfg.StartupCommand = cmd.StartupCommand
		return d.listResponseWithNotice("Startup command updated; takes effect on next activation.")
	}

	// Different project.
	if d.proc != nil && !cmd.Force {
		st, _ := state.Load()
		active := ""
		if st != nil {
			active = st.ActiveWorktree
		}
		evt := d.listResponse()
		evt.ConfirmSwitch = &SwitchInfo{
			FromProject:    d.cfg.ProjectPath,
			ToProject:      cmd.ProjectPath,
			RunningCommand: d.cfg.StartupCommand,
			ActiveWorktree: active,
		}
		return evt
	}

	// Either no proc, or Force=true. stopProcess is a no-op when d.proc==nil.
	if d.proc != nil {
		notice := fmt.Sprintf("Switched from %s to %s (stopped dev server: %s).",
			d.cfg.ProjectPath, cmd.ProjectPath, d.cfg.StartupCommand)
		d.stopProcess()
		d.cfg.ProjectPath = cmd.ProjectPath
		d.cfg.StartupCommand = cmd.StartupCommand
		return d.listResponseWithNotice(notice)
	}
	d.cfg.ProjectPath = cmd.ProjectPath
	d.cfg.StartupCommand = cmd.StartupCommand
	return d.listResponse()
}

func (d *Daemon) listResponseWithNotice(msg string) Event {
	evt := d.listResponse()
	evt.Notice = msg
	return evt
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
		// Nil d.proc *before* calling Stop so the activate-watcher goroutine
		// (waiting on proc.Wait) sees d.proc != proc the instant the child
		// exits and skips its own Push. Otherwise it could race-fire a
		// listResponse using stale d.cfg between us killing the process and
		// the caller mutating cfg / sending the real response.
		proc := d.proc
		d.proc = nil
		proc.Stop()
		// state.Save after Stop so state.yaml carries the still-valid PID
		// during the kill window — easier to track down an orphan if the
		// daemon itself crashes mid-kill.
		state.Save(&state.State{})
	}
}

func (d *Daemon) discoverWorktrees() []Worktree {
	gitRoot := findGitRoot(d.cfg.ProjectPath)
	out, err := silentCommand("git", "-C", gitRoot, "worktree", "list", "--porcelain").Output()
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
	return silentCommand("git", "-C", path, "rev-parse", "--git-dir").Run() == nil
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
