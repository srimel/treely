package daemon

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/srimel/treely/internal/config"
)

func TestParseWorktrees_SkipsBare(t *testing.T) {
	output := "worktree /path/to/project.git\nbare\n\nworktree /path/to/main\nHEAD abc123\nbranch refs/heads/main\n\nworktree /path/to/feat\nHEAD abc123\nbranch refs/heads/feat/foo\n\n"
	result := parseWorktrees(output, "/path/to/project.git")
	if len(result) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(result))
	}
	for _, wt := range result {
		if wt.Path == "/path/to/project.git" {
			t.Error("bare worktree should be excluded from results")
		}
	}
	if result[0].Path != "/path/to/main" {
		t.Errorf("result[0].Path = %q, want /path/to/main", result[0].Path)
	}
	if result[1].Path != "/path/to/feat" {
		t.Errorf("result[1].Path = %q, want /path/to/feat", result[1].Path)
	}
}

func TestParseWorktrees_NoBare(t *testing.T) {
	output := "worktree /path/to/main\nHEAD abc123\nbranch refs/heads/main\n\nworktree /path/to/feat\nHEAD abc123\nbranch refs/heads/feat/foo\n\n"
	result := parseWorktrees(output, "/path/to/main")
	if len(result) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(result))
	}
}

func TestFindGitRoot_DirectPath(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available")
	}
	got := findGitRoot(dir)
	if got != dir {
		t.Errorf("findGitRoot(%q) = %q, want same path", dir, got)
	}
}

func TestFindGitRoot_SubDirectory(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "repo")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "init", sub).Run(); err != nil {
		t.Skip("git not available")
	}
	got := findGitRoot(parent)
	if got != sub {
		t.Errorf("findGitRoot(%q) = %q, want %q", parent, got, sub)
	}
}

func TestFindGitRoot_BareSubDirectory(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "repo.git")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "init", "--bare", sub).Run(); err != nil {
		t.Skip("git not available")
	}
	got := findGitRoot(parent)
	if got != sub {
		t.Errorf("findGitRoot(%q) = %q, want %q", parent, got, sub)
	}
}

func TestFindGitRoot_NoGitRepo(t *testing.T) {
	dir := t.TempDir()
	got := findGitRoot(dir)
	if got != dir {
		t.Errorf("findGitRoot with no git repo = %q, want original path %q", got, dir)
	}
}

func setupConfig(t *testing.T) {
	t.Helper()

	// Redirect home to a temp dir so the daemon never reads or writes the
	// developer's real ~/.treely files. t.Setenv restores both vars on cleanup.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // Windows uses USERPROFILE instead of HOME

	trelyDir := filepath.Join(tmpHome, ".treely")
	if err := os.MkdirAll(trelyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Point the config at the repo root (two levels up from internal/daemon),
	// which is a valid git repo and will produce real worktree output.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(&config.Config{
		ProjectPath:    repoRoot,
		StartupCommand: "echo test",
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForSocket(t *testing.T, sockPath string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon socket never appeared at %s", sockPath)
}

func TestDaemonListCommand(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "daemon_test.sock")
	go func() { _ = Run(sockPath, dir, false) }()
	waitForSocket(t, sockPath)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("{\"cmd\":\"list\"}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("no response from daemon: %v", scanner.Err())
	}
	line := scanner.Text()

	if !strings.Contains(line, `"event":"state_changed"`) {
		t.Errorf("response missing event=state_changed: %s", line)
	}
	if !strings.Contains(line, `"worktrees"`) {
		t.Errorf("response missing worktrees field: %s", line)
	}

	var evt Event
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if evt.Event != "state_changed" {
		t.Errorf("event=%q, want state_changed", evt.Event)
	}
	if evt.Worktrees == nil {
		t.Error("worktrees is nil, expected non-nil array")
	}
}

func TestRunDebugMode(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "daemon_debug_test.sock")
	go func() { _ = Run(sockPath, dir, true) }()
	waitForSocket(t, sockPath)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("{\"cmd\":\"list\"}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("no response from daemon: %v", scanner.Err())
	}
	var evt Event
	if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if evt.Event != "state_changed" {
		t.Errorf("event=%q, want state_changed", evt.Event)
	}
	if evt.Worktrees == nil {
		t.Error("worktrees is nil, expected non-nil array")
	}

	// Clean up the daemon goroutine.
	conn.Write([]byte("{\"cmd\":\"stop\"}\n"))
}

// TestRunStopCommandCleansUp verifies that the "stop" command triggers the
// unified cleanup path: stopProcess, srv.Close, socket removal, PID file removal.
func TestRunStopCommandCleansUp(t *testing.T) {
	setupConfig(t)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "d.sock") // short name avoids macOS 104-byte socket path limit
	pidPath := filepath.Join(dir, "daemon.pid")

	runDone := make(chan error, 1)
	go func() { runDone <- Run(sockPath, dir, false) }()
	waitForSocket(t, sockPath)

	// Verify PID file was written.
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("PID file should exist after daemon start: %v", err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Write([]byte("{\"cmd\":\"stop\"}\n"))
	conn.Close()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after stop command")
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("socket should be removed after stop command")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after stop command")
	}
}
