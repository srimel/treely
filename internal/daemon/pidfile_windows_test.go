//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestIsAlive_CurrentProcess(t *testing.T) {
	if !isAlive(os.Getpid()) {
		t.Error("isAlive(os.Getpid()) = false, want true")
	}
}

func TestIsAlive_DeadProcess(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot run cmd /c exit 0: %v", err)
	}
	deadPID := cmd.ProcessState.Pid()
	// Poll briefly — Windows may keep the handle alive for a short window.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !isAlive(deadPID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("isAlive(%d) = true for a dead process", deadPID)
}

func TestLockPIDFile_Serializes(t *testing.T) {
	dir := t.TempDir()
	release1, err := lockPIDFile(dir)
	if err != nil {
		t.Fatalf("first lockPIDFile: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		release2, err := lockPIDFile(dir)
		if err != nil {
			t.Errorf("second lockPIDFile: %v", err)
			return
		}
		close(acquired)
		release2()
	}()

	// Goroutine should not have acquired the lock yet.
	select {
	case <-acquired:
		t.Error("second lockPIDFile returned before first was released")
	case <-time.After(100 * time.Millisecond):
	}

	release1()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Error("second lockPIDFile did not acquire after first release")
	}
}

func TestLockPIDFile_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release, err := lockPIDFile(dir)
			if err != nil {
				errs[i] = err
				return
			}
			time.Sleep(5 * time.Millisecond)
			release()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

func TestProcessNameMatches(t *testing.T) {
	pid := os.Getpid()
	// The test binary name contains "test" or similar — just verify it doesn't
	// panic and returns a bool. The "treely" match is verified by integration.
	_ = processNameMatches(pid, "go")
}
