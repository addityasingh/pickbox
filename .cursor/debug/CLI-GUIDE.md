# Pickbox CLI Guide

This guide covers the new Cobra-based CLI for Pickbox, a distributed file storage system.

## Installation

### Build from Source
```bash
# Clone the repository
git clone https://github.com/addityasingh/pickbox
cd pickbox

# Build the CLI
make build

# Install to PATH (optional)
make install
```

### Quick Start
```bash
# Show help
./bin/pickbox --help

# Start a single node cluster
./bin/pickbox node start --node-id node1 --port 8001 --bootstrap

# Start a 3-node cluster quickly
make demo-3-nodes
```

## Command Structure

The CLI is organized into logical command groups:

```
pickbox
├── node        # Node management
│   ├── start   # Start full-featured node
│   └── multi   # Start multi-directional replication node
├── cluster     # Cluster management
│   ├── join    # Join node to cluster
│   └── status  # Check cluster status
└── script      # Common operations
    ├── demo-3-nodes # Demo 3-node cluster
    └── cleanup      # Clean up data
```

## Commands

### Node Commands

#### Start Full-Featured Node
```bash
pickbox node start [flags]
```

**Flags:**
- `--node-id, -n`: Node ID (required)
- `--port, -p`: Raft port (default: 8001)
- `--admin-port`: Admin API port (default: 9001)
- `--monitor-port`: Monitor port (default: 9002)
- `--dashboard-port`: Dashboard port (default: 9003)
- `--join, -j`: Address of node to join
- `--bootstrap, -b`: Bootstrap new cluster
- `--data-dir, -d`: Data directory (default: "data")
- `--log-level, -l`: Log level (default: "info")

**Examples:**
```bash
# Bootstrap a new cluster
pickbox node start --node-id node1 --port 8001 --bootstrap

# Join an existing cluster
pickbox node start --node-id node2 --port 8002 --join 127.0.0.1:8001

# Custom ports and directories
pickbox node start --node-id node3 --port 8003 --admin-port 9010 --data-dir /tmp/pickbox
```

#### Start Multi-Directional Replication Node
```bash
pickbox node multi [flags]
```

**Flags:**
- `--node-id, -n`: Node ID (required)
- `--port, -p`: Port (default: 8001)
- `--join, -j`: Address of node to join

**Features:**
- Multi-directional file replication (edit files on any node!)
- Real-time file watching and replication
- Automatic leader forwarding for non-leader nodes
- Raft consensus for consistency

**Examples:**
```bash
# Start multi-directional replication node
pickbox node multi --node-id multi1 --port 8010

# Join existing multi-directional cluster
pickbox node multi --node-id multi2 --port 8011 --join 127.0.0.1:8010
```

### Cluster Commands

#### Join Node to Cluster
```bash
pickbox cluster join [flags]
```

**Flags:**
- `--leader, -l`: Leader address (required)
- `--node-id, -n`: Node ID to join (required)
- `--node-addr, -a`: Node address (required)

**Examples:**
```bash
# Join node to cluster
pickbox cluster join --leader 127.0.0.1:8001 --node-id node4 --node-addr 127.0.0.1:8004
```

#### Check Cluster Status
```bash
pickbox cluster status [flags]
```

**Flags:**
- `--addr, -a`: Admin address to check (default: "127.0.0.1:9001")

**Examples:**
```bash
# Check default cluster status
pickbox cluster status

# Check specific admin server
pickbox cluster status --addr 127.0.0.1:9002
```

### Script Commands

#### Demo 3-Node Cluster
```bash
pickbox script demo-3-nodes
```

Automatically:
- Cleans up old data
- Starts node1 as bootstrap
- Starts node2 and node3 joining the cluster
- Shows access URLs and data directories

#### Cleanup Data
```bash
pickbox script cleanup
```

Removes all data directories from previous runs.

## Common Use Cases

### 1. Quick Testing (3-Node Cluster)
```bash
# Start demo cluster
make demo-3-nodes

# Or manually
pickbox script demo-3-nodes

# Access URLs will be shown:
# - Admin APIs: http://localhost:9001, 9002, 9003
# - Dashboards: http://localhost:9003, 9006, 9009
# - Data dirs: data/node1, data/node2, data/node3
```

### 2. Manual Cluster Setup
```bash
# Terminal 1: Start bootstrap node
pickbox node start --node-id node1 --port 8001 --bootstrap

# Terminal 2: Start second node
pickbox node start --node-id node2 --port 8002 --join 127.0.0.1:8001

# Terminal 3: Start third node
pickbox node start --node-id node3 --port 8003 --join 127.0.0.1:8001
```

### 3. Multi-Directional Replication Testing
```bash
# Terminal 1: Start multi-directional node
pickbox node multi --node-id multi1 --port 8010

# Terminal 2: Join another multi-directional node
pickbox node multi --node-id multi2 --port 8011 --join 127.0.0.1:8010

# Multi-directional: Edit files in data/multi1/ OR data/multi2/ and watch them replicate to all nodes!
# Files can be edited on any node and will automatically replicate to all others
```

### 4. Dynamic Cluster Management
```bash
# Add a new node to running cluster
pickbox cluster join --leader 127.0.0.1:8001 --node-id node4 --node-addr 127.0.0.1:8004

# Check cluster status
pickbox cluster status --addr 127.0.0.1:9001
```

## Port Allocation

The CLI uses predictable port allocation:

**Full Node (node start):**
- Raft port: Specified by `--port` (default: 8001)
- Admin port: Specified by `--admin-port` (default: 9001)
- Monitor port: Specified by `--monitor-port` (default: 9002)
- Dashboard port: Specified by `--dashboard-port` (default: 9003)

**Multi-Directional Node (node multi):**
- Raft port: Specified by `--port` (default: 8001)
- Admin port: Raft port + 1000 (e.g., 8001 → 9001)

**Demo 3-Node Cluster:**
- Node1: Raft 8001, Admin 9001, Monitor 9002, Dashboard 9003
- Node2: Raft 8002, Admin 9002, Monitor 9003, Dashboard 9006
- Node3: Raft 8003, Admin 9003, Monitor 9004, Dashboard 9009

## Data Directories

By default, data is stored in:
- `data/node1/` for node1
- `data/node2/` for node2
- etc.

Each node's data directory contains:
- File storage
- Raft logs
- Snapshots
- Configuration

## Monitoring and Admin

### Admin API
Access admin APIs at `http://localhost:9001` (or specified admin port).

### Monitoring
Access monitoring at `http://localhost:9002/metrics` (or specified monitor port).

### Dashboard
Access dashboard at `http://localhost:9003` (or specified dashboard port).

## File Operations

### Full Node
- Real-time file watching and replication
- Admin interface for cluster management
- Monitoring and metrics
- Dashboard UI

### Multi-Directional Node
- Multi-directional file watching and replication
- Real-time file sync across all nodes
- Automatic leader forwarding for consistency
- Basic admin interface
- Optimized for performance

## Cleanup

```bash
# Stop all nodes
pkill pickbox

# Clean up data
pickbox script cleanup

# Or using make
make demo-cleanup
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Use different ports with `--port`, `--admin-port`, etc.
2. **Data directory conflicts**: Use `--data-dir` to specify different directories
3. **Join failures**: Ensure the leader node is running and accessible

### Debug Mode
```bash
# Enable debug logging
pickbox node start --node-id node1 --log-level debug --bootstrap
```

### Check Logs
All nodes log to stdout with structured logging. Look for:
- `🚀` - Node startup
- `👑` - Leadership changes
- `📡` - File replication
- `✅` - Success messages
- `❌` - Errors

## Migration from Old Commands

### Old vs New Commands

| Old Command | New Command |
|-------------|-------------|
| `./bin/multi_replication` | `pickbox node start` |
| `./bin/live_replication` | `pickbox node multi` |
| Custom scripts | `pickbox script demo-3-nodes` |

### Example Migration
```bash
# Old way
./bin/multi_replication -node node1 -port 8001 -bootstrap

# New way
pickbox node start --node-id node1 --port 8001 --bootstrap
```

## Advanced Usage

### Custom Configuration
```bash
# Production-like setup
pickbox node start \
  --node-id prod-node1 \
  --port 8001 \
  --admin-port 9001 \
  --monitor-port 9002 \
  --dashboard-port 9003 \
  --data-dir /opt/pickbox/data \
  --log-level info \
  --bootstrap
```

### Scripted Deployment
```bash
#!/bin/bash
# Deploy 5-node cluster
for i in {1..5}; do
  port=$((8000 + i))
  admin_port=$((9000 + i))
  
  if [ $i -eq 1 ]; then
    pickbox node start --node-id node$i --port $port --admin-port $admin_port --bootstrap &
  else
    pickbox node start --node-id node$i --port $port --admin-port $admin_port --join 127.0.0.1:8001 &
  fi
  
  sleep 2
done
```

## Next Steps

1. Try the quick start: `make demo-3-nodes`
2. Explore the admin interface: `http://localhost:9001`
3. Check the monitoring dashboard: `http://localhost:9003`
4. Test file replication by editing files in `data/node1/`
5. Scale up by adding more nodes with `pickbox cluster join`

For more information, see the main README and package documentation. 