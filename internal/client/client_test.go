package client

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

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
