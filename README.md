# Pickbox - Distributed Storage System

[![Pickbox CI/CD](https://github.com/addityasingh/pickbox/actions/workflows/go.yml/badge.svg)](https://github.com/addityasingh/pickbox/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/addityasingh/pickbox)](https://goreportcard.com/report/github.com/addityasingh/pickbox)
[![codecov](https://codecov.io/gh/addityasingh/pickbox/branch/main/graph/badge.svg)](https://codecov.io/gh/addityasingh/pickbox)
[![Go Version](https://img.shields.io/github/go-mod/go-version/addityasingh/pickbox)](https://golang.org/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Pickbox is a distributed storage system implemented in Go that provides file operations with replication and consistency guarantees.

## Features

- File operations (OPEN, READ, WRITE, CLOSE)
- Distributed storage with multiple nodes
- Chunk-based storage with replication
- Vector clock-based conflict resolution
- Concurrent request handling
- Structured logging

## Multi-Directional Replication Architecture

The current implementation (Step 3) provides advanced multi-directional file replication where any node can initiate changes that automatically propagate to all other nodes while maintaining strong consistency through Raft consensus.

```mermaid
graph TB
    subgraph "Pickbox Multi-Directional Distributed Storage System"
        subgraph "Node 1 (Leader)"
            N1[Node 1<br/>127.0.0.1:8001]
            FW1[File Watcher<br/>fsnotify]
            FSM1[Enhanced FSM<br/>Content Hash<br/>Deduplication]
            RF1[Raft Instance<br/>Leader]
            FS1[Local Storage<br/>data/node1/]
            ADM1[Admin Server<br/>:9001<br/>FORWARD Support]
            STATE1[File State<br/>SHA-256 Tracking]
            
            N1 --> FW1
            N1 --> FSM1
            N1 --> ADM1
            FW1 -->|"Detect Changes"| FSM1
            FSM1 --> RF1
            FSM1 --> FS1
            FSM1 <--> STATE1
            FSM1 -.->|"Pause During Apply"| FW1
        end
        
        subgraph "Node 2 (Follower + Watcher)"
            N2[Node 2<br/>127.0.0.1:8002]
            FW2[File Watcher<br/>fsnotify]
            FSM2[Enhanced FSM<br/>Content Hash<br/>Deduplication]
            RF2[Raft Instance<br/>Follower]
            FS2[Local Storage<br/>data/node2/]
            ADM2[Admin Server<br/>:9002<br/>FORWARD Support]
            STATE2[File State<br/>SHA-256 Tracking]
            
            N2 --> FW2
            N2 --> FSM2
            N2 --> ADM2
            FW2 -->|"Detect Changes"| FSM2
            FSM2 --> RF2
            FSM2 --> FS2
            FSM2 <--> STATE2
            FSM2 -.->|"Pause During Apply"| FW2
        end
        
        subgraph "Node 3 (Follower + Watcher)"
            N3[Node 3<br/>127.0.0.1:8003]
            FW3[File Watcher<br/>fsnotify]
            FSM3[Enhanced FSM<br/>Content Hash<br/>Deduplication]
            RF3[Raft Instance<br/>Follower]
            FS3[Local Storage<br/>data/node3/]
            ADM3[Admin Server<br/>:9003<br/>FORWARD Support]
            STATE3[File State<br/>SHA-256 Tracking]
            
            N3 --> FW3
            N3 --> FSM3
            N3 --> ADM3
            FW3 -->|"Detect Changes"| FSM3
            FSM3 --> RF3
            FSM3 --> FS3
            FSM3 <--> STATE3
            FSM3 -.->|"Pause During Apply"| FW3
        end
        
        subgraph "Users & Applications"
            USER1[User/App A<br/>Edits Node 1]
            USER2[User/App B<br/>Edits Node 2]
            USER3[User/App C<br/>Edits Node 3]
            CLI[Admin CLI<br/>Cluster Mgmt]
        end
        
        %% User Interactions
        USER1 -->|"Create/Edit/Delete Files"| FS1
        USER2 -->|"Create/Edit/Delete Files"| FS2
        USER3 -->|"Create/Edit/Delete Files"| FS3
        CLI --> ADM1
        CLI --> ADM2
        CLI --> ADM3
        
        %% Multi-Directional Replication Flow
        %% Leader Direct Processing
        FSM1 -->|"Direct Apply (Leader)"| RF1
        
        %% Follower Forwarding to Leader
        FSM2 -->|"TCP FORWARD Command"| ADM1
        FSM3 -->|"TCP FORWARD Command"| ADM1
        
        %% Raft Consensus Distribution
        RF1 -->|"Log Replication"| RF2
        RF1 -->|"Log Replication"| RF3
        RF2 -.->|"Heartbeats/Votes"| RF1
        RF3 -.->|"Heartbeats/Votes"| RF1
        
        %% Apply Commands to All FSMs
        RF1 -->|"Apply Log Entry"| FSM1
        RF1 -->|"Apply Log Entry"| FSM2
        RF1 -->|"Apply Log Entry"| FSM3
        
        %% Smart File System Updates
        FSM1 -->|"Hash-Verified Write"| FS1
        FSM2 -->|"Hash-Verified Write"| FS2
        FSM3 -->|"Hash-Verified Write"| FS3
        
        %% File System Event Detection
        FS1 -.->|"inotify Events"| FW1
        FS2 -.->|"inotify Events"| FW2
        FS3 -.->|"inotify Events"| FW3
        
        %% Result: Synchronized State
        FS1 -.->|"Identical Content"| FS2
        FS2 -.->|"Identical Content"| FS3
        FS3 -.->|"Identical Content"| FS1
        
        %% Key Features Callouts
        subgraph "Key Features"
            FEAT1[✅ Any Node → All Nodes]
            FEAT2[✅ Strong Consistency]
            FEAT3[✅ Content Deduplication]
            FEAT4[✅ Real-time Sync]
            FEAT5[✅ Fault Tolerant]
            FEAT6[✅ Concurrent Users]
        end
    end
```

### Key Architecture Features:

- **🔄 Multi-Directional Replication**: Any node can initiate file changes that replicate to all others
- **🛡️ Strong Consistency**: Raft consensus ensures all nodes maintain identical state
- **⚡ Real-time Synchronization**: File changes detected and replicated within 1-4 seconds
- **🔍 Content Deduplication**: SHA-256 hashing prevents infinite replication loops
- **👥 Concurrent Users**: Multiple users can edit files simultaneously on different nodes
- **🚀 High Performance**: Sub-second change detection with efficient consensus protocol

## Project Structure

```
.
├── cmd/                          # Application entry points
│   ├── replication/             # Step 1: Basic Raft replication
│   ├── live_replication/        # Step 2: Live file watching
│   └── multi_replication/       # Step 3: Multi-directional replication
├── pkg/
│   └── storage/
│       ├── manager.go           # Storage manager implementation
│       ├── raft_manager.go      # Raft consensus implementation
│       └── raft_test.go         # Raft tests
├── scripts/                     # Automation scripts
│   ├── tests/                   # Test scripts
│   │   ├── test_replication.sh
│   │   ├── test_live_replication.sh
│   │   └── test_multi_replication.sh
│   ├── run_replication.sh       # Demo scripts
│   ├── run_live_replication.sh
│   ├── run_multi_replication.sh
│   ├── cleanup_replication.sh   # Utility scripts
│   └── add_nodes.go
├── .cursor/debug/               # Architecture documentation
│   ├── step1_basic_raft_replication.md
│   ├── step2_live_replication.md
│   ├── step3_multi_directional_replication.md
│   └── architecture_evolution_overview.md
├── go.mod                       # Go module definition
├── go.sum                       # Go module checksums
└── README.md                    # This file
```

## Building and Running

### Prerequisites
- Go 1.21 or later
- Git for cloning the repository

### Quick Start (Multi-Directional Replication)

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd pickbox
   ```

2. **Setup development environment** (optional but recommended):
   ```bash
   make setup  # Install tools and pre-commit hooks
   ```

3. **Start the multi-directional cluster**:
   ```bash
   chmod +x scripts/run_multi_replication.sh
   ./scripts/run_multi_replication.sh
   ```

3. **Test the system**:
   ```bash
   # Create files on any node - they replicate everywhere!
   echo "Hello from node1!" > data/node1/test1.txt
   echo "Hello from node2!" > data/node2/test2.txt
   echo "Hello from node3!" > data/node3/test3.txt
   
   # Verify replication (all nodes should have all files)
   ls data/node*/
   ```

4. **Run comprehensive tests**:
   ```bash
   make test-coverage  # or ./scripts/tests/test_multi_replication.sh
   ```

The cluster runs on:
- **Node 1 (Leader)**: 127.0.0.1:8001 (Raft) + 127.0.0.1:9001 (Admin)
- **Node 2 (Follower)**: 127.0.0.1:8002 (Raft) + 127.0.0.1:9002 (Admin)  
- **Node 3 (Follower)**: 127.0.0.1:8003 (Raft) + 127.0.0.1:9003 (Admin)

### Alternative Versions

You can also run earlier evolution steps:

```bash
# Step 1: Basic Raft replication
./scripts/run_replication.sh

# Step 2: Live replication (leader-only)
./scripts/run_live_replication.sh
```

## Usage

### File Operations

The system automatically replicates file operations across all nodes. You can work with files directly through the file system:

**Creating Files**:
```bash
# Create a file on any node
echo "Hello World!" > data/node1/example.txt
echo "Content from node2" > data/node2/another.txt
echo "Data from node3" > data/node3/document.txt
```

**Reading Files**:
```bash
# Read files from any node (content is identical across all nodes)
cat data/node1/example.txt
cat data/node2/example.txt  # Same content as node1
cat data/node3/example.txt  # Same content as node1
```

**Editing Files**:
```bash
# Edit files on any node using any editor
echo "Updated content" >> data/node2/example.txt
nano data/node3/document.txt
vim data/node1/another.txt
```

**Verifying Replication**:
```bash
# Check that all nodes have identical files
find data/ -name "*.txt" -exec echo "=== {} ===" \; -exec cat {} \;
```

### Admin Operations

**Cluster Status**:
```bash
# Check cluster status via admin interface
echo "STATUS" | nc localhost 9001  # Node 1 admin port
echo "STATUS" | nc localhost 9002  # Node 2 admin port  
echo "STATUS" | nc localhost 9003  # Node 3 admin port
```

**Cleanup**:
```bash
# Clean up all processes and data
./scripts/cleanup_replication.sh
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

Pickbox includes a comprehensive test suite covering unit tests, integration tests, and benchmarks. The system provides:

- **Unit Tests**: Storage package, Raft manager, and multi-replication components *(active)*
- **Integration Tests**: End-to-end 3-node cluster testing *(currently disabled for CI/CD stability)*
- **Benchmark Tests**: Performance testing for critical operations *(active)*
- **Test Scripts**: Automated testing for all replication modes *(manual execution only)*

### Quick Test Commands

```bash
# Run all tests with coverage
./scripts/run_tests.sh

# Run integration tests
cd test && go test -v .

# Run unit tests
go test -v ./pkg/storage ./cmd/multi_replication
```

### Test Scripts

- `scripts/tests/test_replication.sh` - Basic Raft replication tests
- `scripts/tests/test_live_replication.sh` - Live file watching tests  
- `scripts/tests/test_multi_replication.sh` - Multi-directional replication tests

**📖 For comprehensive testing documentation, see [`test/README.md`](test/README.md)**

## Code Quality & Linting

Pickbox enforces strict code quality standards through comprehensive linting and automated checks:

### **Linting Tools**
- **golangci-lint**: Comprehensive Go linter with 25+ enabled checks
- **staticcheck**: Advanced static analysis for Go
- **gosec**: Security vulnerability scanner
- **pre-commit**: Automated quality checks on every commit

### **Quality Checks**
- ✅ **Unused Code Detection**: Catches unused variables, functions, and struct fields
- ✅ **Security Scanning**: Detects potential security vulnerabilities
- ✅ **Code Formatting**: Enforces consistent formatting with `gofmt` and `goimports`
- ✅ **Performance Analysis**: Identifies inefficient code patterns
- ✅ **Style Consistency**: Maintains consistent coding style across the project

### **Development Workflow**

```bash
# Setup development environment
make setup                    # Install tools + pre-commit hooks

# Code quality commands
make lint                     # Run all linters
make lint-fix                 # Auto-fix issues where possible
make check-unused             # Check for unused code specifically
make security                 # Run security analysis (go vet + gosec if available)
make security-install         # Install gosec and run full security analysis
make verify-all               # Run all checks (lint + test + security)

# Pre-commit integration
git commit                    # Automatically runs quality checks
make pre-commit               # Run pre-commit hooks manually
```

### **CI Integration**
All quality checks run automatically in GitHub Actions:
- **Pre-commit hooks** prevent bad code from being committed
- **CI pipeline** runs comprehensive linting on every push/PR
- **Security scanning** generates SARIF reports for GitHub Security tab
- **Coverage enforcement** maintains quality thresholds

## CI/CD Pipeline

Pickbox uses GitHub Actions for continuous integration and deployment:

### Pipeline Features
- **Multi-Go Version Testing**: Tests against Go 1.21 and 1.22
- **Comprehensive Test Suite**: Unit tests, integration tests, and benchmarks
- **Code Quality Checks**: `go vet`, `staticcheck`, and security scanning
- **Cross-Platform Builds**: Linux, macOS, and Windows binaries
- **Coverage Reporting**: Automated coverage reports via Codecov
- **Security Scanning**: Gosec security analysis
- **Automated Releases**: Binary releases on main branch pushes

### Pipeline Jobs

1. **Test Suite** (`test`) - Runs unit tests with coverage
2. **Integration Tests** (`integration-test`) - End-to-end testing *(currently disabled - see Improvements section)*
3. **Build** (`build`) - Cross-platform binary compilation
4. **Security** (`security`) - Security vulnerability scanning
5. **Release** (`release`) - Automated GitHub releases
6. **Notify** (`notify`) - Pipeline status notifications

### Artifacts Published
- **Coverage Reports**: HTML and raw coverage data
- **Binaries**: Cross-platform executables for all three modes
- **Security Reports**: SARIF format security scan results
- **Integration Logs**: Debug logs from failed integration tests

### Monitoring
- **Build Status**: [![Pickbox CI/CD](https://github.com/addityasingh/pickbox/actions/workflows/go.yml/badge.svg)](https://github.com/addityasingh/pickbox/actions/workflows/go.yml)
- **Code Coverage**: [![codecov](https://codecov.io/gh/addityasingh/pickbox/branch/main/graph/badge.svg)](https://codecov.io/gh/addityasingh/pickbox)
- **Code Quality**: [![Go Report Card](https://goreportcard.com/badge/github.com/addityasingh/pickbox)](https://goreportcard.com/report/github.com/addityasingh/pickbox)

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
- [ ] Refactor code to be more readable
- [x] Add tests for golang files
- [x] Refactor test bash scripts from scripts folder
- [x] Generate architecture diagram for each of the 3 versions (replication, live_replication, multi_replication)
- [x] Set up comprehensive CI/CD pipeline with GitHub Actions
- [x] Add comprehensive linting with pre-commit hooks and unused field detection
- [ ] Stabilize integration tests for reliable CI/CD execution (currently all disabled due to timing/resource issues)
- [ ] Deploy and create client code for this setup to test end-to-end
- [ ] Make it a generalized solution for N nodes instead of hardcoded 3 nodes
- [ ] Understand the RaftFSM

## License

MIT License
