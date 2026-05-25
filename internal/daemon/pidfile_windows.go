//go:build windows

package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// pidFileMu serializes lockPIDFile within the same process. This handles the
// goroutine-level race; LockFileEx handles the cross-process race.
var pidFileMu sync.Mutex

// isAlive reports whether the process with the given PID is currently running.
// It uses WaitForSingleObject with a zero timeout, which is unambiguous unlike
// GetExitCodeProcess (a process that exits with code 259 looks STILL_ACTIVE).
func isAlive(pid int) bool {
	h, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		// ERROR_INVALID_PARAMETER: PID never existed or fully reaped.
		// ERROR_ACCESS_DENIED: process exists but owned by another user.
		if err == windows.ERROR_ACCESS_DENIED {
			return true
		}
		return false
	}
	defer windows.CloseHandle(h)
	// WAIT_OBJECT_0 (0): process has exited (handle is signaled).
	// WAIT_TIMEOUT (0x102): process still running.
	const waitTimeout = 0x00000102
	event, _ := windows.WaitForSingleObject(h, 0)
	return event == waitTimeout
}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// lockPIDFile acquires both an in-process lock (sync.Mutex) and a cross-process
// file lock (LockFileEx) so that the read-old-PID → terminate → write-new-PID
// sequence is atomic whether the race is within a single process or across two.
//
// Within-process coordination via sync.Mutex is required because LockFileEx on
// Windows does not block between handles owned by the same process.
func lockPIDFile(dir string) (func(), error) {
	pidFileMu.Lock()

	lockPath := pidFilePath(dir) + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		pidFileMu.Unlock()
		return func() {}, err
	}

	ol := new(windows.Overlapped)
	const lockfileExclusiveLock = 0x00000002
	if err := windows.LockFileEx(windows.Handle(f.Fd()), lockfileExclusiveLock, 0, 1, 0, ol); err != nil {
		f.Close()
		pidFileMu.Unlock()
		return func() {}, err
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			ol2 := new(windows.Overlapped)
			_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol2)
			f.Close()
			os.Remove(lockPath)
			pidFileMu.Unlock()
		})
	}
	return release, nil
}

// processNameMatches returns true if the process with the given PID has a
// binary name containing expected. Guards against signalling a recycled PID
// belonging to an unrelated process. strings.Contains("treely.exe", "treely")
// matches the daemonBinaryName constant without any special-casing.
func processNameMatches(pid int, expected string) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32767)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return false
	}
	name := filepath.Base(windows.UTF16ToString(buf[:size]))
	return strings.Contains(name, expected)
}
