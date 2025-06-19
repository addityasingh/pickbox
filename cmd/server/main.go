package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// StorageNode represents a node in the storage system
type StorageNode struct {
	ID         uint32
	Chunks     map[ChunkID]*DataChunk
	ChunkRoles map[ChunkID]ChunkRole
	mu         sync.RWMutex
}

// ChunkID uniquely identifies a chunk of data
type ChunkID struct {
	FileID     uint32
	ChunkIndex uint32
}

// DataChunk represents a chunk of data with metadata
type DataChunk struct {
	ID       ChunkID
	Data     []byte
	Checksum uint64
	Version  uint64
}

// ChunkRole represents the role of a chunk (Primary or Replica)
type ChunkRole int

const (
	Primary ChunkRole = iota
	Replica
)

// NewStorageNode creates a new storage node
func NewStorageNode(id uint32) *StorageNode {
	return &StorageNode{
		ID:         id,
		Chunks:     make(map[ChunkID]*DataChunk),
		ChunkRoles: make(map[ChunkID]ChunkRole),
	}
}

func main() {
	// Initialize logger
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)

	// Initialize storage nodes
	storageNodes := make([]*StorageNode, 3)
	for i := range storageNodes {
		storageNodes[i] = NewStorageNode(uint32(i))
		logrus.Infof("Initialized storage node with ID: %d", i)
	}

	// Start the server
	listener, err := net.Listen("tcp", "127.0.0.1:8085")
	if err != nil {
		logrus.Fatalf("Failed to start server: %v", err)
	}
	defer listener.Close()

	logrus.Info("Server listening on 127.0.0.1:8085")

	for {
		conn, err := listener.Accept()
		if err != nil {
			logrus.Errorf("Failed to accept connection: %v", err)
			continue
		}

		go handleConnection(conn, storageNodes)
	}
}

func handleConnection(conn net.Conn, storageNodes []*StorageNode) {
	defer conn.Close()
	logrus.Info("Processing client request")

	reader := bufio.NewReader(conn)
	for {
		request, err := reader.ReadString('\n')
		if err != nil {
			logrus.Errorf("Failed to read request: %v", err)
			return
		}

		request = strings.TrimSpace(request)
		parts := strings.Fields(request)
		if len(parts) == 0 {
			continue
		}

		var response string
		switch parts[0] {
		case "OPEN":
			response = handleOpen(parts[1:])
		case "READ":
			response = handleRead(parts[1:])
		case "WRITE":
			response = handleWrite(parts[1:])
		case "CLOSE":
			response = handleClose(parts[1:])
		default:
			response = "Invalid command"
		}

		if _, err := fmt.Fprintln(conn, response); err != nil {
			logrus.Errorf("Failed to send response: %v", err)
			return
		}
	}
}

func handleOpen(args []string) string {
	if len(args) != 2 {
		return "Invalid OPEN command format"
	}

	path, mode := args[0], args[1]
	file, err := openFile(path, mode)
	if err != nil {
		logrus.Errorf("Failed to open file %s with mode %s: %v", path, mode, err)
		return fmt.Sprintf("Failed to open file: %v", err)
	}
	defer file.Close()

	logrus.Infof("Opened file: %s with mode: %s", path, mode)
	return "File opened"
}

func handleRead(args []string) string {
	if len(args) != 3 {
		return "Invalid READ command format"
	}

	path := args[0]
	offset, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "Invalid offset"
	}

	length, err := strconv.Atoi(args[2])
	if err != nil {
		return "Invalid length"
	}

	file, err := os.Open(path)
	if err != nil {
		logrus.Errorf("Failed to open file for reading: %v", err)
		return fmt.Sprintf("Failed to open file: %v", err)
	}
	defer file.Close()

	if _, err := file.Seek(offset, 0); err != nil {
		logrus.Errorf("Failed to seek in file: %v", err)
		return fmt.Sprintf("Failed to seek in file: %v", err)
	}

	buffer := make([]byte, length)
	n, err := file.Read(buffer)
	if err != nil {
		logrus.Errorf("Failed to read file: %v", err)
		return fmt.Sprintf("Failed to read file: %v", err)
	}

	logrus.Infof("Read %d bytes from file", n)
	return string(buffer[:n])
}

func handleWrite(args []string) string {
	if len(args) != 3 {
		return "Invalid WRITE command format"
	}

	path := args[0]
	offset, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "Invalid offset"
	}

	data := args[2]
	file, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		logrus.Errorf("Failed to open file for writing: %v", err)
		return fmt.Sprintf("Failed to open file: %v", err)
	}
	defer file.Close()

	if _, err := file.Seek(offset, 0); err != nil {
		logrus.Errorf("Failed to seek in file: %v", err)
		return fmt.Sprintf("Failed to seek in file: %v", err)
	}

	if _, err := file.WriteString(data); err != nil {
		logrus.Errorf("Failed to write to file: %v", err)
		return fmt.Sprintf("Failed to write to file: %v", err)
	}

	logrus.Infof("Wrote data to file: %s", data)
	return "Write successful"
}

func handleClose(args []string) string {
	if len(args) != 1 {
		return "Invalid CLOSE command format"
	}

	path := args[0]
	logrus.Infof("Closing file: %s", path)
	return "File closed"
}

func openFile(path, mode string) (*os.File, error) {
	switch mode {
	case "r":
		return os.Open(path)
	case "w":
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	case "a":
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	default:
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}
}
