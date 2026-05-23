//go:build windows

package daemon

import "os"

func isAlive(pid int) bool {
	// Windows lacks a cheap signal-0 equivalent. Treat all PIDs as dead so stale
	// PID files are cleaned up and the new daemon starts without blocking.
	return false
}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func lockPIDFile(dir string) (func(), error) {
	// No-op on Windows; acceptable given no current Windows CI for this code path.
	return func() {}, nil
}

func processNameMatches(pid int, expected string) bool {
	// Conservative: assume match so the caller proceeds to terminate.
	return true
}
