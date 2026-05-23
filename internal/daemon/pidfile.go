package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// daemonBinaryName is matched against the running process name before sending
// SIGTERM, to guard against signalling a recycled PID that now belongs to an
// unrelated process.
const daemonBinaryName = "treely"

func pidFilePath(dir string) string {
	return filepath.Join(dir, "daemon.pid")
}

// WritePIDFile writes the current process's PID to <dir>/daemon.pid.
func WritePIDFile(dir string) error {
	return os.WriteFile(pidFilePath(dir), []byte(strconv.Itoa(os.Getpid())), 0644)
}

// ReadPIDFile returns the PID from <dir>/daemon.pid.
// Returns (0, nil) if the file does not exist.
func ReadPIDFile(dir string) (int, error) {
	data, err := os.ReadFile(pidFilePath(dir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}
	return pid, nil
}

// RemovePIDFile removes <dir>/daemon.pid (best-effort, ignores errors).
func RemovePIDFile(dir string) {
	os.Remove(pidFilePath(dir))
}

// TerminateExistingDaemon reads the PID file, sends SIGTERM if the process is
// alive and named daemonBinaryName, then polls until it exits or timeout.
// The flock is released before the polling loop so concurrent daemon startups
// are not stalled for the full timeout duration.
func TerminateExistingDaemon(dir string, timeout time.Duration) error {
	release, err := lockPIDFile(dir)
	if err != nil {
		return err
	}

	pid, err := ReadPIDFile(dir)
	if err != nil {
		release()
		return err
	}
	if pid == 0 {
		release()
		return nil
	}
	if !isAlive(pid) {
		RemovePIDFile(dir)
		release()
		return nil
	}
	if !processNameMatches(pid, daemonBinaryName) {
		RemovePIDFile(dir)
		release()
		return nil
	}
	if err := terminate(pid); err != nil {
		release()
		return err
	}
	release() // release before polling so concurrent daemons don't stall

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("pid %d did not exit within %s", pid, timeout)
}
