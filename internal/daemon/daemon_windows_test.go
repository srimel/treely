//go:build windows

package daemon

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

// TestRunCleansUpOnStopCommand verifies that the IPC "stop" command causes the
// daemon to exit cleanly and remove its socket and PID file. This is the
// Windows equivalent of daemon_unix_test.go's TestRunCleansUpOnSIGTERM, which
// cannot be used on Windows because SIGTERM is not supported.
func TestRunCleansUpOnStopCommand(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "d.sock")

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

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte("{\"cmd\":\"stop\"}\n")); err != nil {
		conn.Close()
		t.Fatalf("write stop: %v", err)
	}
	conn.Close()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not exit after stop command within 10s")
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("socket should be removed after stop command")
	}
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Error("PID file should be removed after stop command")
	}
}
