// Package admin provides administrative interfaces for cluster management.
package admin

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

// RaftInterface defines the interface for Raft operations needed by the admin server
type RaftInterface interface {
	State() raft.RaftState
	AddVoter(id raft.ServerID, address raft.ServerAddress, prevIndex uint64, timeout time.Duration) raft.IndexFuture
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
}

const (
	// Admin commands
	AddVoterCmd = "ADD_VOTER"
	ForwardCmd  = "FORWARD"

	// Default timeout for applying commands
	DefaultApplyTimeout = 5 * time.Second
)

// Command represents a file operation that can be forwarded to the leader.
type Command struct {
	Op       string `json:"op"`
	Path     string `json:"path"`
	Data     []byte `json:"data"`
	Hash     string `json:"hash"`
	NodeID   string `json:"node_id"`
	Sequence int64  `json:"sequence"`
}

// Server provides administrative interfaces for cluster management.
type Server struct {
	raft   RaftInterface
	logger *logrus.Logger
	port   int
}

// NewServer creates a new admin server instance.
func NewServer(raftNode RaftInterface, port int, logger *logrus.Logger) *Server {
	if logger == nil {
		logger = logrus.New()
	}

	return &Server{
		raft:   raftNode,
		port:   port,
		logger: logger,
	}
}

// Start begins listening for admin connections.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("starting admin server: %w", err)
	}

	s.logger.Infof("🔧 Admin server listening on port %d", s.port)

	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				s.logger.WithError(err).Warn("Failed to accept admin connection")
				continue
			}

			go s.handleConnection(conn)
		}
	}()

	return nil
}

// handleConnection processes a single admin connection.
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to read from admin connection")
		return
	}

	command := strings.TrimSpace(string(buffer[:n]))

	if strings.HasPrefix(command, AddVoterCmd) {
		s.handleAddVoter(conn, command)
	} else if strings.HasPrefix(command, ForwardCmd) {
		s.handleForward(conn, command)
	} else {
		s.writeResponse(conn, "ERROR: Unknown command")
	}
}

// handleAddVoter processes ADD_VOTER admin commands.
func (s *Server) handleAddVoter(conn net.Conn, command string) {
	parts := strings.Fields(command)
	if len(parts) != 3 {
		s.writeResponse(conn, "ERROR: Invalid ADD_VOTER command format. Usage: ADD_VOTER <nodeID> <address>")
		return
	}

	nodeID, address := parts[1], parts[2]
	future := s.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 0)

	if err := future.Error(); err != nil {
		s.writeResponse(conn, fmt.Sprintf("ERROR: %v", err))
		s.logger.WithError(err).Errorf("Failed to add voter: %s at %s", nodeID, address)
	} else {
		s.writeResponse(conn, "OK")
		s.logger.Infof("➕ Added voter: %s at %s", nodeID, address)
	}
}

// handleForward processes FORWARD admin commands.
func (s *Server) handleForward(conn net.Conn, command string) {
	if s.raft.State() != raft.Leader {
		s.writeResponse(conn, "ERROR: Not leader")
		return
	}

	// Extract JSON command from "FORWARD {...}"
	jsonStart := strings.Index(command, "{")
	if jsonStart == -1 {
		s.writeResponse(conn, "ERROR: Invalid FORWARD command format")
		return
	}

	cmdData := []byte(command[jsonStart:])
	future := s.raft.Apply(cmdData, DefaultApplyTimeout)

	if err := future.Error(); err != nil {
		s.writeResponse(conn, fmt.Sprintf("ERROR: %v", err))
		s.logger.WithError(err).Error("Failed to apply forwarded command")
	} else {
		s.writeResponse(conn, "OK")
		s.logger.Debug("Successfully applied forwarded command")
	}
}

// writeResponse writes a response to an admin connection.
func (s *Server) writeResponse(conn net.Conn, response string) {
	if _, err := conn.Write([]byte(response + "\n")); err != nil {
		s.logger.WithError(err).Warn("Failed to write admin response")
	}
}

// ForwardToLeader sends a command to the leader via admin interface.
func ForwardToLeader(leaderAddr string, cmd Command) error {
	// Extract host from raft address and construct admin address
	host, _, err := net.SplitHostPort(leaderAddr)
	if err != nil {
		return fmt.Errorf("parsing leader address %q: %w", leaderAddr, err)
	}

	// Try different admin ports (incremental from 9001)
	adminPorts := []int{9001, 9002, 9003}
	for _, port := range adminPorts {
		adminAddr := fmt.Sprintf("%s:%d", host, port)
		if err := sendForwardCommand(adminAddr, cmd); err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to forward command to leader at %s", leaderAddr)
}

// sendForwardCommand sends a FORWARD command to the specified admin address.
func sendForwardCommand(adminAddr string, cmd Command) error {
	conn, err := net.DialTimeout("tcp", adminAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshaling command: %w", err)
	}

	message := fmt.Sprintf("%s %s", ForwardCmd, string(cmdData))
	if _, err := conn.Write([]byte(message)); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	return nil
}
