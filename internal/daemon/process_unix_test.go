//go:build !windows

package daemon

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// stopAndDrain calls Stop then drains the Wait channel. Every test that calls
// StartProcess must drain Wait so the zombie is reaped and kill(-pgid,0) sees
// ESRCH rather than a lingering zombie entry.
func stopAndDrain(proc *Process) chan error {
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	proc.Stop()
	return done
}

func TestStopKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	// Shell runs sleep 30 in the background then blocks in wait —
	// sh stays alive as a separate process so sleep 30 is a grandchild.
	proc, err := StartProcess("sleep 30 & wait", dir)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	pgid := proc.pgid
	done := stopAndDrain(proc)

	if err := syscall.Kill(-pgid, 0); err == nil {
		t.Error("process group still alive after Stop()")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Wait() did not return after Stop()")
	}
}

// TestStopKillsCompoundCommand reproduces the exact pattern that the playground
// missed: a semicolon-separated startup command (like "npm install ; npm start")
// keeps the shell alive as the parent while the real work runs in a foreground
// child. The shell cannot exec the child because there is a subsequent command,
// so the child is a true grandchild of the daemon. Before the process-group fix
// a SIGTERM to the shell left the child running and holding its port.
func TestStopKillsCompoundCommand(t *testing.T) {
	dir := t.TempDir()
	// "true" completes immediately; "; true" at the end prevents sh from
	// exec'ing sleep 30, so sh stays alive waiting for it to finish.
	proc, err := StartProcess("sleep 30; true", dir)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	pgid := proc.pgid
	done := stopAndDrain(proc)

	if err := syscall.Kill(-pgid, 0); err == nil {
		t.Error("process group still alive after Stop()")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Wait() did not return after Stop()")
	}
}

// TestStopReleasesPort is the end-to-end reproduction of the EADDRINUSE bug.
// It starts a compound command whose grandchild binds a TCP port (mirroring a
// webpack/vite dev server), calls Stop(), then verifies that the port can be
// rebound immediately — which is exactly what must succeed before the next
// worktree's server can start.
func TestStopReleasesPort(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH")
	}

	// Grab a free port, then release it so python3 can bind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// The "; true" suffix prevents sh from exec'ing python3 directly, so sh
	// stays alive as a separate process — the same topology as "npm i ; npm start"
	// where sh waits on npm while npm's children (webpack/vite) hold the port.
	script := fmt.Sprintf(
		`python3 -c "import socket,time; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.bind(('127.0.0.1', %d)); s.listen(1); time.sleep(30)"; true`,
		port,
	)
	proc, err := StartProcess(script, t.TempDir())
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	// Wait for python3 to bind the port.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond); err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Confirm the port is held before we stop (guards against a flaky setup).
	if ln2, err := net.Listen("tcp", addr); err == nil {
		ln2.Close()
		t.Fatal("grandchild never claimed the port — test setup is broken")
	}

	done := stopAndDrain(proc)
	<-done

	// Before the fix this would return EADDRINUSE because the python3 grandchild
	// survived the SIGTERM that only reached the shell.
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Errorf("port %d still held after Stop(): %v", port, err)
		return
	}
	ln2.Close()
}
