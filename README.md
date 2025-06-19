# Pickbox - Distributed Storage System

Pickbox is a distributed storage system implemented in Go that provides file operations with replication and consistency guarantees.

## Features

- File operations (OPEN, READ, WRITE, CLOSE)
- Distributed storage with multiple nodes
- Chunk-based storage with replication
- Vector clock-based conflict resolution
- Concurrent request handling
- Structured logging

## Project Structure

```
.
├── cmd/
│   └── server/
│       └── main.go       # Main server implementation
├── pkg/
│   └── storage/
│       └── manager.go    # Storage manager implementation
├── go.mod               # Go module definition
└── README.md           # This file
```

## Building and Running

1. Make sure you have Go 1.21 or later installed
2. Clone the repository
3. Build the server:
   ```bash
   go build -o pickbox-server ./cmd/server
   ```
4. Run the server:
   ```bash
   ./pickbox-server
   ```

The server will start listening on `127.0.0.1:8085`.

## Client Protocol

The server accepts the following commands:

- `OPEN <path> <mode>` - Open a file
  - `mode` can be: "r" (read), "w" (write), "a" (append)
- `READ <path> <offset> <length>` - Read data from a file
- `WRITE <path> <offset> <data>` - Write data to a file
- `CLOSE <path>` - Close a file

Example client interaction:
```
OPEN test.txt w
File opened
WRITE test.txt 0 Hello, World!
Write successful
READ test.txt 0 12
Hello, World!
CLOSE test.txt
File closed
```

## Implementation Details

### Storage System

The storage system is implemented with the following components:

1. **Storage Manager**: Manages multiple storage nodes and coordinates operations
2. **Storage Node**: Handles chunk storage and replication
3. **Vector Clock**: Implements vector clocks for conflict resolution

### Concurrency

- Each client connection is handled in a separate goroutine
- Storage operations are protected by mutexes for thread safety
- Vector clock operations are atomic

### Logging

The system uses structured logging via `logrus` for better observability. Logs include:
- Server startup and shutdown
- Client connections and disconnections
- File operations
- Storage operations
- Error conditions

## License

MIT License
