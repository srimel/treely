package client

import (
	"bufio"
	"encoding/json"
	"net"
)

type Command struct {
	Cmd      string `json:"cmd"`
	Worktree string `json:"worktree,omitempty"`
}

type Worktree struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Event struct {
	Event     string     `json:"event"`
	Worktrees []Worktree `json:"worktrees,omitempty"`
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
		c.Events <- evt
	}
}

func (c *Client) Send(cmd Command) error {
	return c.enc.Encode(cmd)
}

func (c *Client) Close() {
	c.conn.Close()
}
