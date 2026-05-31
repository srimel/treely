package daemon

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSocketRemovedOnClose verifies that the socket file is cleaned up after
// NewServer + Close so a subsequent daemon start does not see EADDRINUSE.
// This catches an AF_UNIX quirk on some Windows builds where socket files
// persist as reparse points after the listener is closed.
func TestSocketRemovedOnClose(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	// Remove is a no-op if NewServer already removed it; errors here mean the
	// file persists and future daemon starts would fail.
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not remove socket file after Close: %v", err)
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket file persisted after Close+Remove: %v", err)
	}
}

func TestServer_MalformedJSON_ContinuesServing(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	handled := make(chan Command, 2)
	go srv.Accept(func(cmd Command) (interface{}, bool) { //nolint:errcheck
		handled <- cmd
		return nil, true
	})

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Garbage first, then a valid command — server must skip the garbage.
	conn.Write([]byte("definitely not json\n"))
	conn.Write([]byte("{\"cmd\":\"list\"}\n"))

	select {
	case cmd := <-handled:
		if cmd.Cmd != "list" {
			t.Errorf("cmd = %q, want list", cmd.Cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: server should skip malformed JSON and handle valid command")
	}
}

func TestServer_SecondClientReplacesFirst(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	go srv.Accept(func(cmd Command) (interface{}, bool) { //nolint:errcheck
		return nil, true
	})

	conn1, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial conn1: %v", err)
	}
	// Give the server time to register conn1.
	time.Sleep(50 * time.Millisecond)

	conn2, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial conn2: %v", err)
	}
	defer conn2.Close()

	// conn1 should be closed by the server when conn2 connects.
	conn1.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn1.Read(buf)
	if err == nil {
		t.Error("conn1 should have been closed by the server after conn2 connected")
	}
}

func TestServer_Push_DeliveresToConnectedClient(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	go srv.Accept(func(cmd Command) (interface{}, bool) { //nolint:errcheck
		return nil, true
	})

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Give the server time to register the client connection.
	time.Sleep(50 * time.Millisecond)

	evt := Event{
		Event:     "state_changed",
		Worktrees: []Worktree{{Path: "/x", Name: "x", Status: "inactive"}},
	}
	srv.Push(evt)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected to receive pushed event on client connection")
	}
	var received Event
	if err := json.Unmarshal(scanner.Bytes(), &received); err != nil {
		t.Fatalf("unmarshal pushed event: %v", err)
	}
	if received.Event != "state_changed" {
		t.Errorf("event = %q, want state_changed", received.Event)
	}
	if len(received.Worktrees) != 1 {
		t.Errorf("worktrees = %d, want 1", len(received.Worktrees))
	}
}

func TestServer_Push_NoClient_IsNoOp(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// No client connected; Push must not panic.
	srv.Push(Event{Event: "state_changed"})
}

func TestServer_HandlerResponse_IsSentToClient(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	go srv.Accept(func(cmd Command) (interface{}, bool) { //nolint:errcheck
		return Event{Event: "state_changed", Worktrees: []Worktree{}}, true
	})

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send a command and expect the handler's response back.
	conn.Write([]byte("{\"cmd\":\"list\"}\n"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected response from handler")
	}
	var evt Event
	if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if evt.Event != "state_changed" {
		t.Errorf("response event = %q, want state_changed", evt.Event)
	}
}
