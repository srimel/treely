package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/srimel/treely/internal/client"
	"github.com/srimel/treely/internal/config"
	"github.com/srimel/treely/internal/daemon"
	"github.com/srimel/treely/internal/tui"
)

func main() {
	daemonMode := flag.Bool("daemon", false, "run as daemon")
	debugMode := flag.Bool("debug", false, "enable debug logging to ~/.treely/daemon.log")
	projectOverride := flag.String("p", "", "project path override (session only)")
	restartDaemon := flag.Bool("restart-daemon", false, "restart the background daemon")
	flag.Parse()

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

	if *projectOverride != "" {
		cfg.ProjectPath = *projectOverride
	}

	if err := daemon.Fork(sockPath, dir, *debugMode); err != nil {
		log.Fatal(err)
	}

	c, err := client.Connect(sockPath)
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}
	defer c.Close()

	m := tui.NewModel(cfg, c, sockPath, dir, *debugMode)
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
