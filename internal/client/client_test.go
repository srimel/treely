package client

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// mustListen starts a Unix listener at sockPath and returns it.
func mustListen(t *testing.T, sockPath string) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestClientSendAndReceive(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "c.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	received := make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			received <- scanner.Text()
		}

		conn.Write([]byte("{\"event\":\"state_changed\",\"worktrees\":[]}\n"))
	}()

	c, err := Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.Send(Command{Cmd: "list"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case line := <-received:
		var cmd Command
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			t.Fatalf("invalid JSON from client: %v", err)
		}
		if cmd.Cmd != "list" {
			t.Errorf("cmd=%q, want list", cmd.Cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for command")
	}

	select {
	case evt, ok := <-c.Events:
		if !ok {
			t.Fatal("Events channel closed unexpectedly")
		}
		if evt.Event != "state_changed" {
			t.Errorf("event=%q, want state_changed", evt.Event)
		}
		if evt.Worktrees == nil {
			t.Error("worktrees is nil, expected non-nil slice from []")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestClient_MalformedJSON_IsSkippedAndValidEventArrives(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "c.sock")
	ln := mustListen(t, sockPath)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send garbage followed by a valid event; client should skip the garbage.
		conn.Write([]byte("not valid json at all\n"))
		conn.Write([]byte("{\"event\":\"state_changed\",\"worktrees\":[]}\n"))
	}()

	c, err := Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	select {
	case evt, ok := <-c.Events:
		if !ok {
			t.Fatal("Events channel closed before valid event arrived")
		}
		if evt.Event != "state_changed" {
			t.Errorf("event = %q, want state_changed", evt.Event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: malformed JSON should be skipped; valid event never arrived")
	}
}

func TestClient_Close_ClosesEventsChannel(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "c.sock")
	ln := mustListen(t, sockPath)
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			<-done
			conn.Close()
		}
	}()

	c, err := Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	c.Close()
	close(done)

	select {
	case _, ok := <-c.Events:
		if ok {
			t.Error("Events channel still open after Close(); want closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Events channel not closed after Close()")
	}
}

func TestClient_ServerDisconnect_ClosesEventsChannel(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "c.sock")
	ln := mustListen(t, sockPath)
	defer ln.Close()

	serverClosed := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close() // immediately close the server side
		close(serverClosed)
	}()

	c, err := Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	<-serverClosed

	select {
	case _, ok := <-c.Events:
		if ok {
			t.Error("received unexpected event after server disconnect")
		}
		// ok == false means channel closed as expected
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Events channel not closed after server disconnect")
	}
}

func TestClient_Send_EncodesCommandAsJSON(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "c.sock")
	ln := mustListen(t, sockPath)
	defer ln.Close()

	received := make(chan Command, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			var cmd Command
			_ = json.Unmarshal(scanner.Bytes(), &cmd)
			received <- cmd
		}
	}()

	c, err := Connect(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.Send(Command{Cmd: "activate", Worktree: "/x/main", Force: true}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case cmd := <-received:
		if cmd.Cmd != "activate" {
			t.Errorf("Cmd = %q, want activate", cmd.Cmd)
		}
		if cmd.Worktree != "/x/main" {
			t.Errorf("Worktree = %q, want /x/main", cmd.Worktree)
		}
		if !cmd.Force {
			t.Error("Force should be true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent command")
	}
}
