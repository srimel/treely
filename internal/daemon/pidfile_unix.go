//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func isAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if err == syscall.EPERM {
		// Alive but owned by a different user.
		return true
	}
	return false // ESRCH or other → process gone
}

func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// lockPIDFile acquires an exclusive flock on a companion lock file so that the
// read-old-PID → terminate → write-new-PID sequence is atomic across racing
// daemon startups.
func lockPIDFile(dir string) (func(), error) {
	lockPath := pidFilePath(dir) + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return func() {}, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return func() {}, err
	}
	release := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
		os.Remove(lockPath)
	}
	return release, nil
}

// processNameMatches returns true if the process with the given PID has a
// command name containing expected. This guards against signalling a recycled
// PID that now belongs to an unrelated process.
func processNameMatches(pid int, expected string) bool {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(out)), expected)
}
