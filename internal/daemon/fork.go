package daemon

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Fork starts a new daemon process if one is not already running on sockPath.
func Fork(sockPath, dir string, debug bool) error {
	if conn, err := net.DialTimeout("unix", sockPath, time.Second); err == nil {
		conn.Close()
		return nil
	}

	// Socket is stale or absent. Terminate any predecessor daemon that may still
	// be alive (e.g. it lost its socket but not its PID file) before spawning a
	// replacement. Best-effort: log and continue if it fails.
	if err := TerminateExistingDaemon(dir, 5*time.Second); err != nil {
		slog.Warn("could not terminate existing daemon", "err", err)
	}

	logPath := filepath.Join(dir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}

	args := []string{"--daemon"}
	if debug {
		args = append(args, "--debug")
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = forkSysProcAttr()
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	logFile.Close()

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
