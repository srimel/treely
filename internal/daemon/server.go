package daemon

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"sync"
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

type Server struct {
	listener net.Listener
	mu       sync.Mutex
	client   net.Conn
}

func NewServer(sockPath string) (*Server, error) {
	os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, err
	}
	return &Server{listener: ln}, nil
}

func (s *Server) Close() {
	s.listener.Close()
	s.mu.Lock()
	if s.client != nil {
		s.client.Close()
	}
	s.mu.Unlock()
}

// Accept blocks until a client connects, then handles it.
// handler is called for each command; returns false to stop serving.
func (s *Server) Accept(handler func(cmd Command) (interface{}, bool)) error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		s.mu.Lock()
		if s.client != nil {
			s.client.Close()
		}
		s.client = conn
		s.mu.Unlock()
		slog.Debug("client connected")
		go s.handleConn(conn, handler)
	}
}

func (s *Server) handleConn(conn net.Conn, handler func(cmd Command) (interface{}, bool)) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		if s.client == conn {
			s.client = nil
		}
		s.mu.Unlock()
		slog.Debug("client disconnected")
	}()
	scanner := bufio.NewScanner(conn)
	enc := json.NewEncoder(conn)
	for scanner.Scan() {
		var cmd Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			continue
		}
		resp, cont := handler(cmd)
		if resp != nil {
			enc.Encode(resp)
		}
		if !cont {
			return
		}
	}
}

// Push sends an event to the currently connected client (if any).
func (s *Server) Push(evt Event) {
	s.mu.Lock()
	conn := s.client
	s.mu.Unlock()
	if conn == nil {
		return
	}
	slog.Debug("pushing event", "event", evt.Event, "worktrees", len(evt.Worktrees))
	json.NewEncoder(conn).Encode(evt)
}
