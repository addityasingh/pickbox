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

## Testing

The project includes comprehensive test scripts in the `scripts/tests/` directory:

- `scripts/tests/test_replication.sh` - Run all replication tests
- `scripts/tests/test_live_replication.sh` - Test live file watching and replication
- `scripts/tests/test_multi_replication.sh` - Test multi-directional replication

See `scripts/tests/README.md` for detailed testing information.

## Scripts Organization

```
scripts/
├── tests/                    # Test scripts
│   ├── README.md
│   ├── test_replication.sh
│   ├── test_live_replication.sh
│   └── test_multi_replication.sh
├── run_replication.sh        # Demo scripts
├── run_live_replication.sh
├── run_multi_replication.sh
├── cleanup_replication.sh    # Utility scripts
└── add_nodes.go
```

## Architecture Documentation

Comprehensive architecture diagrams and documentation are available in `.cursor/debug/`:

- **Step 1**: `step1_basic_raft_replication.md` - Basic Raft consensus replication
- **Step 2**: `step2_live_replication.md` - Live file watching and replication  
- **Step 3**: `step3_multi_directional_replication.md` - Multi-directional replication
- **Overview**: `architecture_evolution_overview.md` - Complete evolution analysis

Each document includes detailed Mermaid diagrams showing:
- Node architecture and communication patterns
- Data flow and command processing
- Component relationships and dependencies
- Evolution from basic consensus to advanced multi-directional replication

## Improvements 
- [] Refactor code to be more readable
- [] Add tests for golang files
- [x] Refactor test bash scripts from scripts folder
- [x] Generate architecture diagram for each of the 3 versions (replication, live_replication, multi_replication)

## License

MIT License
