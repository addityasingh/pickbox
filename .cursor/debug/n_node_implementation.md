# N-Node Generic Implementation for Pickbox

## Overview

The Pickbox distributed storage system has been enhanced to support **N nodes** instead of being hardcoded for exactly 3 nodes. This implementation provides flexible cluster management with configurable parameters for any number of nodes.

## Key Changes Made

### 1. Generic Cluster Manager (`scripts/cluster_manager.sh`)

**New Features:**
- Supports 1 to 20+ nodes (configurable)
- Parameterized port assignments
- Configuration file support
- Dynamic node discovery
- Comprehensive cluster management (start, stop, restart, status, clean, logs)

**Usage Examples:**
```bash
# Start 5-node cluster
./scripts/cluster_manager.sh start -n 5

# Start 7-node cluster with custom ports
./scripts/cluster_manager.sh start -n 7 -p 9000 -a 10000

# Use configuration file
./scripts/cluster_manager.sh start -c examples/cluster-configs/5-node-cluster.conf

# Check cluster status
./scripts/cluster_manager.sh status -n 5

# View logs from all nodes
./scripts/cluster_manager.sh logs -n 5
```

### 2. Enhanced Node Addition (`scripts/add_nodes.go`)

**Improvements:**
- Generic node count parameter (`-nodes N`)
- Configurable port ranges (`-base-port P`)
- Flexible starting node number (`-start N`)
- Support for remote clusters (`-host H`)

**Usage Examples:**
```bash
# Add 2 nodes (default: node2, node3)
go run scripts/add_nodes.go

# Add 5 nodes starting from node2
go run scripts/add_nodes.go -nodes 5

# Add nodes to cluster with custom ports
go run scripts/add_nodes.go -nodes 3 -base-port 9000 -admin-port 10000

# Add nodes starting from node4 (for expanding clusters)
go run scripts/add_nodes.go -nodes 2 -start 4
```

### 3. Flexible Main Application (`cmd/multi_replication/main.go`)

**Key Change:**
- Removed hardcoded "node1" bootstrap assumption
- Any node can bootstrap when no join address is specified
- More flexible cluster initialization

**Before:**
```go
// Auto-bootstrap if no join address and this is node1
if cfg.JoinAddr == "" && cfg.NodeID == "node1" {
    cfg.BootstrapCluster = true
}
```

**After:**
```go
// Auto-bootstrap if no join address is specified
// This allows any node to bootstrap when it's the first in the cluster
if cfg.JoinAddr == "" {
    cfg.BootstrapCluster = true
}
```

### 4. Generic Test Suite (`scripts/tests/test_n_replication.sh`)

**Features:**
- Tests any number of nodes
- Configurable timeouts and ports
- Comprehensive validation (file operations, deduplication, consistency)
- Verbose output support

**Usage Examples:**
```bash
# Test 5-node cluster
./scripts/tests/test_n_replication.sh -n 5

# Test with custom configuration
./scripts/tests/test_n_replication.sh -n 7 -p 9000 -a 10000 -v

# Quick test with timeout
./scripts/tests/test_n_replication.sh -n 4 -t 120
```

### 5. Configuration File System

**Example Configurations:**
- `examples/cluster-configs/5-node-cluster.conf` - Standard 5-node setup
- `examples/cluster-configs/7-node-cluster.conf` - 7-node cluster
- `examples/cluster-configs/10-node-high-ports.conf` - 10-node with high ports

**Configuration Format:**
```bash
NODE_COUNT=5
BASE_PORT=8001
ADMIN_BASE_PORT=9001
MONITOR_BASE_PORT=6001
DASHBOARD_PORT=8080
HOST=127.0.0.1
DATA_DIR=data
BINARY=cmd/multi_replication/main.go
```

## Port Assignment Schema

The new implementation uses a systematic port assignment:

### Formula
- **Raft Port**: `BASE_PORT + (node_number - 1)`
- **Admin Port**: `ADMIN_BASE_PORT + (node_number - 1)`
- **Monitor Port**: `MONITOR_BASE_PORT + (node_number - 1)`
- **Dashboard Port**: Shared across all nodes

### Example for 5-Node Cluster (BASE_PORT=8001)
```
node1: Raft=8001, Admin=9001, Monitor=6001
node2: Raft=8002, Admin=9002, Monitor=6002
node3: Raft=8003, Admin=9003, Monitor=6003
node4: Raft=8004, Admin=9004, Monitor=6004
node5: Raft=8005, Admin=9005, Monitor=6005
Dashboard: 8080 (shared)
```

## Usage Patterns

### Quick Start (5-Node Cluster)
```bash
# Start cluster
./scripts/cluster_manager.sh start -n 5

# Test replication
echo "Hello from node1!" > data/node1/test.txt
echo "Hello from node3!" > data/node3/test.txt

# Verify replication
ls data/node*/
cat data/node*/test.txt
```

### Production Setup (10-Node Cluster)
```bash
# Use configuration file approach
./scripts/cluster_manager.sh start -c examples/cluster-configs/10-node-high-ports.conf

# Monitor cluster
./scripts/cluster_manager.sh status -c examples/cluster-configs/10-node-high-ports.conf

# Run comprehensive tests
./scripts/tests/test_n_replication.sh -n 10 -p 18001 -a 19001 -v
```

### Development Testing
```bash
# Quick 3-node test (backward compatible)
./scripts/cluster_manager.sh start -n 3

# Test different sizes
for nodes in 3 5 7; do
    echo "Testing $nodes nodes..."
    ./scripts/tests/test_n_replication.sh -n $nodes -t 60
done
```

## Backward Compatibility

### Existing Scripts Still Work
All existing 3-node scripts remain functional:
- `scripts/run_multi_replication.sh` - Still works for 3 nodes
- `scripts/tests/test_multi_replication.sh` - Still tests 3 nodes

### Migration Path
1. **Keep using existing scripts** for current workflows
2. **Gradually adopt generic scripts** for new clusters
3. **Use configuration files** for complex setups

## Advanced Features

### Dynamic Cluster Expansion
```bash
# Start with 3 nodes
./scripts/cluster_manager.sh start -n 3

# Later expand to 5 nodes (in separate terminal)
./scripts/cluster_manager.sh start -n 2 -start 4  # Add node4, node5
go run scripts/add_nodes.go -nodes 2 -start 4    # Add to cluster
```

### Multi-Environment Setup
```bash
# Development cluster (low ports)
./scripts/cluster_manager.sh start -n 3 -p 8001

# Staging cluster (medium ports)
./scripts/cluster_manager.sh start -n 5 -p 12001 --data-dir staging_data

# Testing cluster (high ports)
./scripts/cluster_manager.sh start -n 7 -p 18001 --data-dir test_data
```

### Custom Binary Testing
```bash
# Test with different binary
# [DELETED] ./scripts/cluster_manager.sh start -n 4 --binary cmd/multi_replication/main.go

# Test with configuration
echo "# [BINARY DELETED - use cmd/multi_replication/main.go instead]" >> custom.conf
./scripts/cluster_manager.sh start -c custom.conf
```

## Validation and Testing

### Comprehensive Test Coverage
The new test suite validates:
- ✅ **Cluster Formation**: N-node startup and joining
- ✅ **Multi-directional Replication**: Any node → all nodes
- ✅ **Content Consistency**: Files identical across all nodes
- ✅ **Deduplication**: No infinite loops from simultaneous writes
- ✅ **Fault Tolerance**: Graceful handling of node failures
- ✅ **Performance**: Scaling characteristics with node count

### Test Results Summary
```bash
# Example test output for 5-node cluster
🎉 SUCCESS: All N-node replication tests passed!

✅ Tested successfully:
  • 5-node cluster startup
  • Multi-directional file replication
  • Content consistency across all nodes
  • Deduplication and conflict resolution
```

## Benefits of N-Node Implementation

### 1. **Flexibility**
- Support for any cluster size (1-20+ nodes)
- Configurable port ranges to avoid conflicts
- Environment-specific configurations

### 2. **Scalability**
- Easy horizontal scaling
- Performance testing with different node counts
- Production-ready large clusters

### 3. **Development Efficiency**
- Single toolset for all cluster sizes
- Consistent management interface
- Automated testing for various configurations

### 4. **Production Readiness**
- Port conflict resolution
- Resource isolation between environments
- Comprehensive monitoring and logging

## Future Enhancements

### Potential Improvements
1. **Auto-discovery**: Automatic node detection without manual configuration
2. **Load Balancing**: Intelligent request routing across nodes
3. **Health Monitoring**: Automated failure detection and recovery
4. **Dynamic Reconfiguration**: Runtime cluster resizing
5. **Multi-host Support**: Distributed across multiple machines

### Configuration Management
1. **Kubernetes Integration**: Helm charts for N-node deployments
2. **Docker Compose**: Multi-container orchestration
3. **Environment Variables**: Cloud-native configuration
4. **Service Discovery**: Integration with Consul/etcd

## Conclusion

The N-node implementation transforms Pickbox from a fixed 3-node system into a truly scalable distributed storage solution. With generic tooling, comprehensive testing, and flexible configuration, it's now ready for both development experimentation and production deployment at any scale.

**Key Takeaway**: The same codebase now powers anything from a single-node development setup to a large production cluster, with consistent behavior and management tools across all configurations. 