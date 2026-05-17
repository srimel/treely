package main

import (
	"flag"
	"fmt"
	"log"
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
	projectOverride := flag.String("p", "", "project path override (session only)")
	restartDaemon := flag.Bool("restart-daemon", false, "restart the background daemon")
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

	if *restartDaemon {
		stopDaemon(sockPath)
		if err := daemon.Fork(sockPath, dir); err != nil {
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

	if err := daemon.Fork(sockPath, dir); err != nil {
		log.Fatal(err)
	}

	c, err := client.Connect(sockPath)
	if err != nil {
		log.Fatalf("failed to connect to daemon: %v", err)
	}
	defer c.Close()

	m := tui.NewModel(cfg, c, sockPath, dir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
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
