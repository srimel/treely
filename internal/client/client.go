package client

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net"
)

type Command struct {
	Cmd            string `json:"cmd"`
	Worktree       string `json:"worktree,omitempty"`
	ProjectPath    string `json:"project_path,omitempty"`
	StartupCommand string `json:"startup_command,omitempty"`
	Force          bool   `json:"force,omitempty"`
}

type Worktree struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type SwitchInfo struct {
	FromProject    string `json:"from_project"`
	ToProject      string `json:"to_project"`
	RunningCommand string `json:"running_command"`
	ActiveWorktree string `json:"active_worktree"`
}

type Event struct {
	Event         string      `json:"event"`
	Worktrees     []Worktree  `json:"worktrees,omitempty"`
	Notice        string      `json:"notice,omitempty"`
	ConfirmSwitch *SwitchInfo `json:"confirm_switch,omitempty"`
}

type Client struct {
	conn    net.Conn
	enc     *json.Encoder
	scanner *bufio.Scanner
	Events  chan Event
}

func Connect(sockPath string) (*Client, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("connected to daemon", "sock", sockPath)
	c := &Client{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		scanner: bufio.NewScanner(conn),
		Events:  make(chan Event, 16),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	defer close(c.Events)
	for c.scanner.Scan() {
		var evt Event
		if err := json.Unmarshal(c.scanner.Bytes(), &evt); err != nil {
			continue
		}
		slog.Debug("received event", "event", evt.Event, "worktrees", len(evt.Worktrees))
		c.Events <- evt
	}
}

func (c *Client) Send(cmd Command) error {
	slog.Debug("sent command", "cmd", cmd.Cmd)
	return c.enc.Encode(cmd)
}

func (c *Client) Close() {
	c.conn.Close()
}
