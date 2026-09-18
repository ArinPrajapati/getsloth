package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// hostControlRequest stays local to the host process and its separately
// launched control console. It is deliberately not part of the relay
// protocol: viewers must never receive host-only actions or session secrets.
type hostControlRequest struct {
	Action string `json:"action"`
}

type hostControlResponse struct {
	Snapshot *hostControlSnapshot `json:"snapshot,omitempty"`
	Error    string               `json:"error,omitempty"`
}

type hostControlActions struct {
	reclaim func() error
	kill    func() error
}

type hostControlServer struct {
	listener   net.Listener
	socketPath string
	stateDir   string
	snapshot   func() hostControlSnapshot
	actionsMu  sync.RWMutex
	actions    hostControlActions
	closeOnce  sync.Once
	closeErr   error
}

func startHostControlServer(snapshot func() hostControlSnapshot) (*hostControlServer, error) {
	if snapshot == nil {
		return nil, errors.New("host control snapshot is required")
	}

	stateDir, err := os.MkdirTemp("", "getsloth-control-")
	if err != nil {
		return nil, fmt.Errorf("create host control directory: %w", err)
	}

	socketPath := filepath.Join(stateDir, "control.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = os.RemoveAll(stateDir)
		return nil, fmt.Errorf("listen on host control socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.RemoveAll(stateDir)
		return nil, fmt.Errorf("restrict host control socket: %w", err)
	}

	server := &hostControlServer{
		listener:   listener,
		socketPath: socketPath,
		stateDir:   stateDir,
		snapshot:   snapshot,
	}
	go server.serve()
	return server, nil
}

func (s *hostControlServer) setActions(reclaim, kill func() error) {
	s.actionsMu.Lock()
	s.actions = hostControlActions{reclaim: reclaim, kill: kill}
	s.actionsMu.Unlock()
}

func (s *hostControlServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go s.handle(conn)
	}
}

func (s *hostControlServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	var request hostControlRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return
	}

	response := hostControlResponse{}
	switch request.Action {
	case "snapshot":
		// No action needed.
	case "reclaim", "kill":
		if err := s.invoke(request.Action); err != nil {
			response.Error = err.Error()
		}
	default:
		response.Error = "unknown host control action"
	}
	if response.Error == "" {
		snapshot := s.snapshot()
		response.Snapshot = &snapshot
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func (s *hostControlServer) invoke(action string) error {
	s.actionsMu.RLock()
	var invoke func() error
	switch action {
	case "reclaim":
		invoke = s.actions.reclaim
	case "kill":
		invoke = s.actions.kill
	}
	s.actionsMu.RUnlock()
	if invoke == nil {
		return errors.New("host control action is unavailable")
	}
	return invoke()
}

func (s *hostControlServer) Close() error {
	s.closeOnce.Do(func() {
		listenerErr := s.listener.Close()
		removeErr := os.RemoveAll(s.stateDir)
		if listenerErr != nil {
			s.closeErr = listenerErr
			return
		}
		s.closeErr = removeErr
	})
	return s.closeErr
}
