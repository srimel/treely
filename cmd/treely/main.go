package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
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
	projectOverride := flag.String("p", "", "project path override (session only)")
	flag.Parse()

	dir, err := config.Dir()
	if err != nil {
		log.Fatal(err)
	}
	sockPath := filepath.Join(dir, "daemon.sock")

	if *daemonMode {
		if err := daemon.Run(sockPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	// TUI mode
	cfg, err := config.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	if cfg == nil {
		// First run: show wizard
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

	// Ensure daemon is running
	if err := ensureDaemon(sockPath, dir); err != nil {
		log.Fatal(err)
	}

	// Connect client
	c, err := client.Connect(sockPath)
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}
	defer c.Close()

	m := tui.NewModel(cfg, c)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
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

func ensureDaemon(sockPath, dir string) error {
	// Check if socket already exists and is connectable
	if conn, err := net.DialTimeout("unix", sockPath, time.Second); err == nil {
		conn.Close()
		return nil
	}

	// Fork daemon
	logPath := filepath.Join(dir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(self, "--daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = daemonSysProcAttr()
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	logFile.Close()

	// Wait for socket to appear
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start in time")
}
