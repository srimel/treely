//go:build !windows

package daemon

import (
	"bufio"
	"encoding/json"
	"net"
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

// newRunningDaemon spins up a Daemon with a real "sleep 30" child process so
// the proc-running set_project branches exercise the actual process state
// rather than a fake sentinel.
func newRunningDaemon(t *testing.T, projectPath, startupCmd string) *Daemon {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	sockDir, err := os.MkdirTemp("", "d")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "d.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sockPath) })

	d := &Daemon{
		cfg: &config.Config{
			ProjectPath:    projectPath,
			StartupCommand: startupCmd,
		},
		srv:      srv,
		sockPath: sockPath,
		shutdown: make(chan struct{}),
	}
	d.activate(projectPath)
	time.Sleep(50 * time.Millisecond)
	if d.proc == nil {
		t.Fatal("process should be running after activate")
	}
	t.Cleanup(func() { d.stopProcess() })
	return d
}

// TestHandleSetProject_DifferentProjectProcRunningNoForce verifies that a
// running dev server blocks a project switch when Force is false: d.cfg stays
// put, the process keeps running, and a ConfirmSwitch payload is returned.
func TestHandleSetProject_DifferentProjectProcRunningNoForce(t *testing.T) {
	dir := t.TempDir()
	d := newRunningDaemon(t, dir, "sleep 30")
	pgid := d.proc.pgid

	evt := d.handleSetProject(Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/elsewhere",
		StartupCommand: "echo new",
		Force:          false,
	})

	if evt.ConfirmSwitch == nil {
		t.Fatal("expected ConfirmSwitch payload, got nil")
	}
	if evt.ConfirmSwitch.FromProject != dir {
		t.Errorf("FromProject = %q, want %q", evt.ConfirmSwitch.FromProject, dir)
	}
	if evt.ConfirmSwitch.ToProject != "/proj/elsewhere" {
		t.Errorf("ToProject = %q, want %q", evt.ConfirmSwitch.ToProject, "/proj/elsewhere")
	}
	if evt.ConfirmSwitch.RunningCommand != "sleep 30" {
		t.Errorf("RunningCommand = %q, want %q", evt.ConfirmSwitch.RunningCommand, "sleep 30")
	}
	if d.cfg.ProjectPath != dir {
		t.Errorf("ProjectPath mutated to %q despite Force=false", d.cfg.ProjectPath)
	}
	if d.cfg.StartupCommand != "sleep 30" {
		t.Errorf("StartupCommand mutated to %q despite Force=false", d.cfg.StartupCommand)
	}
	if d.proc == nil {
		t.Error("proc cleared despite Force=false")
	}
	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Errorf("process group should still be alive after Force=false: %v", err)
	}
}

// TestHandleSetProject_DifferentProjectProcRunningForce verifies that a forced
// project switch goes through the existing stopProcess path (process group
// SIGTERM/SIGKILL), mutates d.cfg, and surfaces a notice mentioning the
// stopped command.
func TestHandleSetProject_DifferentProjectProcRunningForce(t *testing.T) {
	dir := t.TempDir()
	d := newRunningDaemon(t, dir, "sleep 30")
	pgid := d.proc.pgid

	evt := d.handleSetProject(Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/elsewhere",
		StartupCommand: "echo new",
		Force:          true,
	})

	if evt.ConfirmSwitch != nil {
		t.Errorf("ConfirmSwitch = %+v, want nil after forced switch", evt.ConfirmSwitch)
	}
	if evt.Notice == "" {
		t.Error("expected non-empty Notice after forced switch")
	}
	if d.cfg.ProjectPath != "/proj/elsewhere" {
		t.Errorf("ProjectPath = %q, want %q", d.cfg.ProjectPath, "/proj/elsewhere")
	}
	if d.cfg.StartupCommand != "echo new" {
		t.Errorf("StartupCommand = %q, want %q", d.cfg.StartupCommand, "echo new")
	}
	if d.proc != nil {
		t.Error("proc should be nil after forced switch")
	}
	if err := syscall.Kill(-pgid, 0); err == nil {
		t.Error("process group should be dead after forced switch")
	}
}

// TestHandleSetProject_ForceSwitchEmitsOneEvent locks in the push-race fix in
// stopProcess: with the fix, a Force-true switch produces exactly one event
// (the direct response). Without it, the watcher goroutine waiting on
// proc.Wait can race-fire a stale listResponse Push *before* d.cfg is
// mutated, and the client would see two events back-to-back.
func TestHandleSetProject_ForceSwitchEmitsOneEvent(t *testing.T) {
	dir := t.TempDir()
	d := newRunningDaemon(t, dir, "sleep 30")

	go func() { _ = d.srv.Accept(d.handle) }()

	conn, err := net.Dial("unix", d.sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	cmd := Command{
		Cmd:            "set_project",
		ProjectPath:    "/proj/elsewhere",
		StartupCommand: "echo new",
		Force:          true,
	}
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		t.Fatalf("encode: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if !scanner.Scan() {
		t.Fatalf("no first event: %v", scanner.Err())
	}
	var evt Event
	if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
		t.Fatalf("unmarshal first event: %v", err)
	}
	if evt.Notice == "" {
		t.Errorf("first event Notice is empty; want the force-switch notice")
	}
	if evt.ConfirmSwitch != nil {
		t.Errorf("first event ConfirmSwitch = %+v, want nil", evt.ConfirmSwitch)
	}

	// Drain for spurious extra events. A second event here would mean the
	// watcher-goroutine race regressed.
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if scanner.Scan() {
		t.Errorf("unexpected second event after force switch: %s", scanner.Text())
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
