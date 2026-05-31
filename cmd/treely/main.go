package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"
	"github.com/srimel/treely/internal/client"
	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/daemon"
	"github.com/srimel/treely/internal/tui"
)

// version is set via -ldflags at release time (see .goreleaser.yml).
var version = "dev"

func main() {
	// pflag treats single-dash multi-char args as shorthand chains, so
	// `-version` would be parsed as `-v -e -r -s -i -o -n`. Pre-scan args
	// so that `treely -version` works alongside `--version` and `-v`.
	for _, arg := range os.Args[1:] {
		if arg == "-version" {
			fmt.Printf("treely %s\n", version)
			return
		}
	}

	daemonMode := pflag.Bool("daemon", false, "run as daemon")
	debugMode := pflag.Bool("debug", false, "enable debug logging to ~/.treely/daemon.log")
	cmdOverride := pflag.StringP("command", "c", "", "startup command override (session only)")
	restartDaemon := pflag.Bool("restart-daemon", false, "restart the background daemon")
	showVersion := pflag.BoolP("version", "v", false, "print version and exit")
	pflag.Parse()

	if *showVersion {
		fmt.Printf("treely %s\n", version)
		return
	}

	positionalPath := resolvePositionalPath(pflag.Arg(0))

	dir, err := config.Dir()
	if err != nil {
		log.Fatal(err)
	}
	sockPath := filepath.Join(dir, "daemon.sock")

	if *daemonMode {
		if err := daemon.Run(sockPath, dir, *debugMode); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *debugMode {
		logPath := filepath.Join(dir, "daemon.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open log file: %v\n", err)
		} else {
			defer logFile.Close()
			slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})))
		}
	}

	if *restartDaemon {
		stopDaemon(sockPath)
		if err := daemon.Fork(sockPath, dir, *debugMode); err != nil {
			log.Fatalf("failed to start daemon: %v", err)
		}
		fmt.Println("Daemon restarted.")
		return
	}

	// TUI mode
	cfg, err := config.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	if cfg == nil {
		cfg, err = runWizard()
		if err != nil {
			log.Fatal(err)
		}
		if err := config.Save(cfg); err != nil {
			log.Fatal(err)
		}
	}

	if positionalPath != "" {
		if _, err := os.Stat(positionalPath); err != nil {
			log.Fatalf("project path %q: %v", positionalPath, err)
		}
	}
	cfg = resolveConfig(cfg, positionalPath, *cmdOverride)

	if err := daemon.Fork(sockPath, dir, *debugMode); err != nil {
		log.Fatal(err)
	}

	c, err := client.Connect(sockPath)
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}
	defer c.Close()

	m := tui.NewModel(cfg, c, sockPath, dir, *debugMode, version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		log.Fatal(err)
	}
	if fm, ok := final.(tui.Model); ok {
		printExitFeedback(fm, sockPath, dir)
	}
}

func printExitFeedback(m tui.Model, sockPath, dir string) {
	switch m.QuitReason {
	case tui.QuitReasonUser:
		pid, _ := daemon.ReadPIDFile(dir)
		fmt.Println("Exited Treely.")
		if pid > 0 {
			fmt.Printf("Daemon still running (PID %d).\n", pid)
		} else {
			fmt.Println("Daemon is not running.")
		}
		if m.ActiveWorktreeName != "" {
			fmt.Printf("Dev server still running for %s.\n", m.ActiveWorktreeName)
		} else {
			fmt.Println("No active worktree.")
		}

	case tui.QuitReasonKillDaemon:
		stopped := waitForSocketGone(sockPath, 3*time.Second)
		if !stopped {
			fmt.Println("Sent stop signal to daemon, but it did not exit within 3s.")
			fmt.Println("Check `~/.treely/daemon.log` or run `treely --restart-daemon`.")
			return
		}
		fmt.Println("Stopped Treely daemon.")
		if m.ActiveWorktreeName != "" {
			fmt.Printf("Dev server for %s stopped.\n", m.ActiveWorktreeName)
		} else {
			fmt.Println("No worktree was active.")
		}
	}
}

func waitForSocketGone(sockPath string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// stopDaemon sends a stop command to a running daemon and waits for it to exit.
func stopDaemon(sockPath string) {
	c, err := client.Connect(sockPath)
	if err != nil {
		return
	}
	c.Send(client.Command{Cmd: "stop"})
	c.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// resolvePositionalPath resolves the positional project path against the
// shell's cwd before it crosses the socket. The daemon runs detached with its
// own cwd, so a relative path like "." or "../foo" would otherwise resolve
// against the wrong root. Empty input is returned unchanged.
func resolvePositionalPath(p string) string {
	return resolvePositionalPathWith(p, os.Getwd)
}

// resolvePositionalPathWith is the testable inner of resolvePositionalPath:
// the cwd source is injected so the Getwd-failure fallback can be exercised
// without actually destroying the test process's cwd.
func resolvePositionalPathWith(p string, getwd func() (string, error)) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd, err := getwd()
	if err != nil {
		return p
	}
	return filepath.Join(wd, p)
}

// resolveConfig applies session-only CLI overrides to cfg in memory. It does
// not touch the on-disk config. Empty strings mean "no override".
func resolveConfig(cfg *config.Config, positionalPath, cmdOverride string) *config.Config {
	if cfg == nil {
		return nil
	}
	out := *cfg
	if positionalPath != "" {
		out.ProjectPath = positionalPath
	}
	if cmdOverride != "" {
		out.StartupCommand = cmdOverride
	}
	return &out
}

func runWizard() (*config.Config, error) {
	wm := tui.NewWizardModel()
	p := tea.NewProgram(wm, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	wz, ok := final.(tui.WizardModel)
	if !ok || wz.Result == nil {
		return nil, fmt.Errorf("wizard cancelled")
	}
	return wz.Result, nil
}
