//go:build windows

package daemon

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestStopKillsJobObject verifies that Stop() terminates the process tree via
// the Job Object, equivalent to the Unix TestStopKillsProcessGroup test.
// A long-running PowerShell command is started, Stop() is called, and then
// the process handle is inspected to confirm the process has exited.
func TestStopKillsJobObject(t *testing.T) {
	dir := t.TempDir()
	proc, err := StartProcess("Start-Sleep -Seconds 300", dir)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	pid := uint32(proc.pid)

	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	proc.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("Wait() did not return after Stop()")
	}

	// Confirm the process is gone. WAIT_OBJECT_0 (0) = exited, WAIT_TIMEOUT (0x102) = still alive.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return // process fully reaped — that's fine
	}
	defer windows.CloseHandle(h)
	const waitTimeout = 0x00000102
	event, _ := windows.WaitForSingleObject(h, 0)
	if event == waitTimeout {
		t.Error("process still alive after Stop()")
	}
}
