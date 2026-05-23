package daemon

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// runTrueCmd returns a command that exits 0 immediately, cross-platform.
func runTrueCmd() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=^$")
}

func TestWriteReadRemovePIDFile(t *testing.T) {
	dir := t.TempDir()

	if err := WritePIDFile(dir); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	pid, err := ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("ReadPIDFile = %d, want %d", pid, os.Getpid())
	}

	RemovePIDFile(dir)

	pid, err = ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile after remove: %v", err)
	}
	if pid != 0 {
		t.Errorf("ReadPIDFile after remove = %d, want 0", pid)
	}
}

func TestReadPIDFileMissing(t *testing.T) {
	dir := t.TempDir()
	pid, err := ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile on missing file: %v", err)
	}
	if pid != 0 {
		t.Errorf("got pid %d, want 0 for missing file", pid)
	}
}

func TestTerminateExistingDaemonHandlesDeadPID(t *testing.T) {
	dir := t.TempDir()

	// Start a process and wait for it to exit so we have a reaped (dead) PID.
	cmd := runTrueCmd()
	if err := cmd.Run(); err != nil {
		t.Skipf("can't run helper command: %v", err)
	}
	deadPID := cmd.ProcessState.Pid()

	// Write the dead PID to the PID file.
	if err := os.WriteFile(pidFilePath(dir), []byte(strconv.Itoa(deadPID)), 0644); err != nil {
		t.Fatal(err)
	}

	if err := TerminateExistingDaemon(dir, 5*time.Second); err != nil {
		t.Errorf("TerminateExistingDaemon with dead PID: %v", err)
	}

	// PID file should be gone.
	if _, err := os.Stat(pidFilePath(dir)); !os.IsNotExist(err) {
		t.Error("PID file should be removed for dead PID")
	}
}
