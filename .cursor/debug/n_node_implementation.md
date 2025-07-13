# N-Node Implementation Guide

## Overview

This document describes the implementation of the N-Node cluster functionality in Pickbox, which allows creating and managing clusters of any size (not just 3 nodes) with automatic port assignment and flexible configuration.

## Current Architecture

### Unified CLI Implementation
- **Main CLI**: `cmd/pickbox/main.go` - Unified command-line interface
- **Node Management**: `cmd/pickbox/node.go` - Node lifecycle management
- **Multi-Replication**: `cmd/pickbox/multi_replication.go` - Multi-directional replication logic
- **Cluster Management**: `cmd/pickbox/cluster.go` - Cluster operations

### Binary Usage
```bash
# Build the unified binary
go build -o bin/pickbox ./cmd/pickbox

# Start nodes using the CLI
./bin/pickbox node multi --node-id node1 --port 8001 --bootstrap
./bin/pickbox node multi --node-id node2 --port 8002 --join 127.0.0.1:8001
./bin/pickbox node multi --node-id node3 --port 8003 --join 127.0.0.1:8001
```

## Implementation Components

### 1. **Port Allocation System**
```go
// Port calculation logic in cmd/pickbox/
func calculatePorts(basePort int) (int, int, int) {
    raftPort := basePort
    adminPort := basePort + 1000
    monitorPort := basePort + 2000
    return raftPort, adminPort, monitorPort
}
```

### 2. **Configuration Management**
```go
// Enhanced configuration in cmd/pickbox/multi_replication.go
type MultiConfig struct {
    NodeID        string
    Port          int
    AdminPort     int
    MonitorPort   int
    DashboardPort int
    DataDir       string
    Join          string
    Host          string
    Bootstrap     bool
}
```

### 3. **Cluster Management Scripts**
The `scripts/cluster_manager.sh` script provides comprehensive N-node cluster management:

```bash
# Configuration structure
NODE_COUNT=5
BASE_PORT=8001
ADMIN_BASE_PORT=9001
MONITOR_BASE_PORT=6001
DASHBOARD_PORT=8080
HOST=127.0.0.1
DATA_DIR=data
BINARY=./bin/pickbox
BINARY_ARGS="node multi"
```

## Key Features

### 1. **Dynamic Node Count**
- Support for 1 to 20+ nodes
- Automatic port assignment
- Scalable configuration

### 2. **Flexible Configuration**
- Configuration files for different scenarios
- Environment-specific settings
- Override capabilities

### 3. **Automated Management**
- Cluster lifecycle management
- Health monitoring
- Cleanup utilities

## Usage Examples

### Basic Usage
```bash
# Start a 5-node cluster
./scripts/cluster_manager.sh start -n 5

# Start with custom ports
./scripts/cluster_manager.sh start -n 7 -p 9000 -a 10000

# Use configuration file
./scripts/cluster_manager.sh start -c examples/cluster-configs/5-node-cluster.conf
```

### Configuration Files
Example configuration for different scenarios:

#### Standard 5-Node Setup
```bash
# examples/cluster-configs/5-node-cluster.conf
NODE_COUNT=5
BASE_PORT=8001
ADMIN_BASE_PORT=9001
MONITOR_BASE_PORT=6001
DASHBOARD_PORT=8080
HOST=127.0.0.1
DATA_DIR=data
BINARY=./bin/pickbox
BINARY_ARGS="node multi"
```

#### High-Port Configuration
```bash
# examples/cluster-configs/10-node-high-ports.conf
NODE_COUNT=10
BASE_PORT=18001
ADMIN_BASE_PORT=19001
MONITOR_BASE_PORT=16001
DASHBOARD_PORT=18080
HOST=127.0.0.1
DATA_DIR=data
BINARY=./bin/pickbox
BINARY_ARGS="node multi"
```

## Advanced Features

### 1. **Multi-Environment Support**
```bash
# Development cluster
./scripts/cluster_manager.sh start -n 3 -p 8001 --data-dir dev

# Staging cluster
./scripts/cluster_manager.sh start -n 5 -p 12001 --data-dir staging

# Production cluster
./scripts/cluster_manager.sh start -n 7 -p 18001 --data-dir prod
```

### 2. **Dynamic Node Addition**
```bash
# Start with 3 nodes
./scripts/cluster_manager.sh start -n 3

# Add additional nodes
go run scripts/add_nodes.go -nodes 2 -start 4
```

### 3. **Testing and Validation**
```bash
# Test N-node cluster
./scripts/tests/test_n_replication.sh -n 5 -v

# Test with custom configuration
./scripts/tests/test_n_replication.sh -n 10 -p 18001
```

## Benefits

### 1. **Scalability**
- Support for large clusters
- Efficient resource utilization
- Horizontal scaling capabilities

### 2. **Flexibility**
- Configurable port ranges
- Multiple deployment scenarios
- Environment-specific settings

### 3. **Automation**
- Automated cluster management
- Simplified deployment
- Comprehensive testing

### 4. **Reliability**
- Fault tolerance
- Health monitoring
- Graceful degradation

## Implementation Notes

### Port Allocation Schema
- **Raft Port**: BASE_PORT + node_number - 1
- **Admin Port**: ADMIN_BASE_PORT + node_number - 1
- **Monitor Port**: MONITOR_BASE_PORT + node_number - 1
- **Dashboard Port**: Shared across all nodes

### Configuration Management
- Default values for quick setup
- Override capabilities for custom scenarios
- Validation and error handling

### Testing Strategy
- Unit tests for core functionality
- Integration tests for cluster operations
- Performance benchmarks
- End-to-end validation

## Migration from Legacy Implementation

### Old Structure
```
cmd/
├── multi_replication/
│   └── main.go
└── live_replication/
    └── main.go
```

### New Structure
```
cmd/
└── pickbox/
    ├── main.go
    ├── node.go
    ├── multi_replication.go
    ├── cluster.go
    └── script.go
```

### Migration Steps
1. Replace multiple binaries with single `pickbox` binary
2. Update CLI commands to use new structure
3. Migrate configuration files to new format
4. Update scripts to use new binary and arguments

This unified approach provides a more maintainable and user-friendly system while preserving all the functionality of the original multi-directional replication system. 