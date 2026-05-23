//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/srimel/treely/internal/config"
)

// TestHelperDaemonProcess is a helper that the test binary re-invokes as a
// daemon subprocess. It must not run as a regular test.
func TestHelperDaemonProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	sockPath := os.Getenv("DAEMON_SOCK")
	dir := os.Getenv("DAEMON_DIR")
	if sockPath == "" || dir == "" {
		os.Exit(1)
	}
	_ = Run(sockPath, dir, false)
	os.Exit(0)
}

// TestRunCleansUpOnSIGTERM verifies that SIGTERM causes the daemon to exit
// cleanly and remove its socket and PID file. This reproduces the first bug:
// daemon.Run() exiting via signal without calling stopProcess or cleaning up.
func TestRunCleansUpOnSIGTERM(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "d.sock") // short name avoids macOS 104-byte socket path limit

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperDaemonProcess", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"DAEMON_SOCK="+sockPath,
		"DAEMON_DIR="+dir,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon helper: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill() }) //nolint:errcheck

	waitForSocket(t, sockPath)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not exit after SIGTERM within 10s")
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("socket should be removed after SIGTERM")
	}
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Error("PID file should be removed after SIGTERM")
	}
}

// TestPIDFileLockSerializes verifies that concurrent calls to TerminateExistingDaemon
// are serialized by the flock: all return nil with no races and the PID file is removed.
func TestPIDFileLockSerializes(t *testing.T) {
	dir := t.TempDir()

	cmd := runTrueCmd()
	if err := cmd.Run(); err != nil {
		t.Skipf("can't run helper command: %v", err)
	}
	deadPID := cmd.ProcessState.Pid()
	if err := os.WriteFile(pidFilePath(dir), []byte(strconv.Itoa(deadPID)), 0644); err != nil {
		t.Fatal(err)
	}

	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = TerminateExistingDaemon(dir, 5*time.Second)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: TerminateExistingDaemon returned error: %v", i, err)
		}
	}
	if _, err := os.Stat(pidFilePath(dir)); !os.IsNotExist(err) {
		t.Error("PID file should be removed after concurrent TerminateExistingDaemon calls")
	}
}

// TestDaemonStopsChildOnShutdown verifies that stopProcess sends SIGTERM to the
// child's entire process group, guaranteeing grandchildren (e.g., a dev server's
// subprocess) release their ports before the next worktree is activated.
func TestDaemonStopsChildOnShutdown(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "d.sock")

	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sockPath) })

	d := &Daemon{
		cfg: &config.Config{
			StartupCommand: "sleep 30",
			ProjectPath:    dir,
		},
		srv:      srv,
		sockPath: sockPath,
		shutdown: make(chan struct{}),
	}

	d.activate(dir)
	time.Sleep(50 * time.Millisecond)

	if d.proc == nil {
		t.Fatal("process should be running after activate")
	}
	pgid := d.proc.pgid

	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Fatalf("process group %d should be alive before stop: %v", pgid, err)
	}

	d.stopProcess()

	if err := syscall.Kill(-pgid, 0); err == nil {
		t.Error("process group should be dead after stopProcess")
	}
}

// TestForkTerminatesStalePredecessor verifies the Fork-time cleanup path: a stale
// PID file from a dead predecessor is removed so a new daemon can start cleanly.
func TestForkTerminatesStalePredecessor(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	// Use a short-prefix temp dir for the socket to avoid the macOS 104-byte
	// sockaddr_un.sun_path limit: this test's name is too long for t.TempDir().
	sockDir, err := os.MkdirTemp("", "d")
	if err != nil {
		t.Fatalf("create sock dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "d.sock")

	// Write a dead PID to simulate a predecessor that was SIGKILLed or crashed.
	cmd := runTrueCmd()
	if err := cmd.Run(); err != nil {
		t.Skipf("can't run helper command: %v", err)
	}
	deadPID := cmd.ProcessState.Pid()
	if err := os.WriteFile(pidFilePath(dir), []byte(strconv.Itoa(deadPID)), 0644); err != nil {
		t.Fatal(err)
	}

	if err := TerminateExistingDaemon(dir, 5*time.Second); err != nil {
		t.Fatalf("TerminateExistingDaemon with stale PID: %v", err)
	}
	if _, err := os.Stat(pidFilePath(dir)); !os.IsNotExist(err) {
		t.Error("stale PID file should be removed before new daemon starts")
	}

	// A new daemon should start successfully after stale state is cleared.
	helper := exec.Command(os.Args[0], "-test.run=TestHelperDaemonProcess", "-test.v=false")
	helper.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"DAEMON_SOCK="+sockPath,
		"DAEMON_DIR="+dir,
	)
	helper.Stdout = os.Stderr
	helper.Stderr = os.Stderr
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper daemon: %v", err)
	}
	t.Cleanup(func() { helper.Process.Kill() })

	waitForSocket(t, sockPath)

	// TODO: extend with a live treely-named process to verify that processNameMatches
	// guards against signalling recycled PIDs; requires building the real binary in tests.
}
