#!/bin/bash

# Generic Cluster Manager for Pickbox Distributed Storage
# Supports N nodes with configurable parameters

set -euo pipefail

# Default configuration
DEFAULT_NODE_COUNT=3
DEFAULT_BASE_PORT=8001
DEFAULT_ADMIN_BASE_PORT=9001
DEFAULT_MONITOR_BASE_PORT=6001
DEFAULT_DASHBOARD_PORT=8080
DEFAULT_HOST="127.0.0.1"
DEFAULT_DATA_DIR="data"
DEFAULT_BINARY="cmd/multi_replication/main.go"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Global variables
NODE_COUNT=$DEFAULT_NODE_COUNT
BASE_PORT=$DEFAULT_BASE_PORT
ADMIN_BASE_PORT=$DEFAULT_ADMIN_BASE_PORT
MONITOR_BASE_PORT=$DEFAULT_MONITOR_BASE_PORT
DASHBOARD_PORT=$DEFAULT_DASHBOARD_PORT
HOST=$DEFAULT_HOST
DATA_DIR=$DEFAULT_DATA_DIR
BINARY=$DEFAULT_BINARY
PIDS=()
ACTION=""
CONFIG_FILE=""

# Help function
show_help() {
    cat << EOF
Pickbox Cluster Manager - Generic N-node cluster management

USAGE:
    $0 <action> [options]

ACTIONS:
    start       Start N-node cluster
    stop        Stop all nodes in cluster
    restart     Restart cluster
    status      Show cluster status
    clean       Clean all data and processes
    logs        Show logs from all nodes

OPTIONS:
    -n, --nodes <count>         Number of nodes (default: $DEFAULT_NODE_COUNT)
    -p, --base-port <port>      Base port for Raft (default: $DEFAULT_BASE_PORT)
    -a, --admin-port <port>     Base admin port (default: $DEFAULT_ADMIN_BASE_PORT)
    -m, --monitor-port <port>   Base monitor port (default: $DEFAULT_MONITOR_BASE_PORT)
    -d, --dashboard-port <port> Dashboard port (default: $DEFAULT_DASHBOARD_PORT)
    -h, --host <host>           Host address (default: $DEFAULT_HOST)
    --data-dir <dir>            Data directory (default: $DEFAULT_DATA_DIR)
    --binary <path>             Binary path (default: $DEFAULT_BINARY)
    -c, --config <file>         Load configuration from file
    --help                      Show this help

EXAMPLES:
    # Start 5-node cluster
    $0 start -n 5

    # Start with custom ports
    $0 start -n 3 -p 9000 -a 10000

    # Use configuration file
    $0 start -c cluster.conf

    # Clean everything
    $0 clean

CONFIGURATION FILE FORMAT (cluster.conf):
    NODE_COUNT=5
    BASE_PORT=9000
    ADMIN_BASE_PORT=10000
    MONITOR_BASE_PORT=7000
    DASHBOARD_PORT=8080
    HOST=127.0.0.1
    DATA_DIR=data
    BINARY=cmd/multi_replication/main.go

EOF
}

# Logging functions
log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Load configuration from file
load_config() {
    local config_file="$1"
    if [[ ! -f "$config_file" ]]; then
        error "Configuration file not found: $config_file"
        exit 1
    fi
    
    source "$config_file"
    log "Configuration loaded from $config_file"
}

# Generate node configuration
get_node_config() {
    local node_num=$1
    local node_id="node$node_num"
    local port=$((BASE_PORT + node_num - 1))
    local admin_port=$((ADMIN_BASE_PORT + node_num - 1))
    local monitor_port=$((MONITOR_BASE_PORT + node_num - 1))
    local data_dir="$DATA_DIR/$node_id"
    
    echo "$node_id:$port:$admin_port:$monitor_port:$data_dir"
}

# Get leader address for joining
get_leader_address() {
    echo "$HOST:$BASE_PORT"
}

# Get leader admin address for adding nodes
get_leader_admin_address() {
    echo "$HOST:$ADMIN_BASE_PORT"
}

# Clean up function
cleanup_cluster() {
    log "Cleaning up cluster..."
    
    # Kill processes
    local process_patterns=(
        "multi_replication"
        "cmd/multi_replication"
        "live_replication"
        "cmd/live_replication"
    )
    
    for pattern in "${process_patterns[@]}"; do
        pkill -f "$pattern" 2>/dev/null || true
    done
    
    # Wait for processes to die
    sleep 2
    
    # Force kill any remaining processes using our ports
    local ports=()
    for ((i=1; i<=NODE_COUNT; i++)); do
        ports+=($((BASE_PORT + i - 1)))
        ports+=($((ADMIN_BASE_PORT + i - 1)))
        ports+=($((MONITOR_BASE_PORT + i - 1)))
    done
    ports+=($DASHBOARD_PORT)
    
    for port in "${ports[@]}"; do
        if lsof -ti :"$port" >/dev/null 2>&1; then
            warn "Force killing processes using port $port"
            lsof -ti :"$port" | xargs kill -9 2>/dev/null || true
        fi
    done
    
    # Remove data directories
    if [[ -d "$DATA_DIR" ]]; then
        log "Removing data directories..."
        for ((i=1; i<=NODE_COUNT; i++)); do
            rm -rf "$DATA_DIR/node$i" 2>/dev/null || true
        done
    fi
    
    log "Cleanup completed"
}

# Create data directories
create_data_dirs() {
    log "Creating data directories for $NODE_COUNT nodes..."
    
    for ((i=1; i<=NODE_COUNT; i++)); do
        local node_id="node$i"
        local data_dir="$DATA_DIR/$node_id"
        mkdir -p "$data_dir"
        info "Created $data_dir"
    done
}

# Start a single node
start_node() {
    local node_num=$1
    local config
    config=$(get_node_config "$node_num")
    IFS=':' read -r node_id port admin_port monitor_port data_dir <<< "$config"
    
    local cmd_args=(
        "go" "run" "$BINARY"
        "-node" "$node_id"
        "-port" "$port"
        "-admin-port" "$admin_port"
        "-monitor-port" "$monitor_port"
        "-dashboard-port" "$DASHBOARD_PORT"
        "-data-dir" "$data_dir"
    )
    
    # First node bootstraps, others join
    if [[ $node_num -eq 1 ]]; then
        cmd_args+=("-bootstrap")
        log "Starting $node_id (bootstrap leader) on port $port..."
    else
        local leader_addr
        leader_addr=$(get_leader_address)
        cmd_args+=("-join" "$leader_addr")
        log "Starting $node_id (follower) joining at $leader_addr on port $port..."
    fi
    
    # Start the node in background
    "${cmd_args[@]}" > "/tmp/${node_id}.log" 2>&1 &
    local pid=$!
    PIDS+=("$pid")
    
    info "$node_id started with PID $pid"
    
    # Give the node time to start
    if [[ $node_num -eq 1 ]]; then
        sleep 6  # Leader needs more time to bootstrap
    else
        sleep 3
    fi
}

# Add follower nodes to cluster
add_nodes_to_cluster() {
    if [[ $NODE_COUNT -le 1 ]]; then
        return 0
    fi
    
    log "Waiting for follower nodes to join cluster..."
    
    # Wait for nodes to join automatically via the -join flag
    # The join logic is now handled by the application itself
    sleep 5
    
    log "Cluster membership should be established via automatic joining"
}

# Start the cluster
start_cluster() {
    log "Starting $NODE_COUNT-node Pickbox cluster..."
    
    # Clean up any existing processes
    cleanup_cluster
    
    # Create data directories
    create_data_dirs
    
    # Start all nodes
    for ((i=1; i<=NODE_COUNT; i++)); do
        start_node "$i"
    done
    
    # Add nodes to cluster
    add_nodes_to_cluster
    
    # Show cluster information
    show_cluster_info
    
    # Set up signal handling for cleanup
    trap 'stop_cluster' EXIT INT TERM
    
    log "Cluster startup completed successfully!"
    
    # If running interactively, wait for user input
    if [[ -t 0 ]]; then
        echo ""
        echo "Press Ctrl+C to stop the cluster..."
        wait
    fi
}

# Stop the cluster
stop_cluster() {
    log "Stopping cluster..."
    
    # Kill all node processes
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping process $pid..."
            kill "$pid" 2>/dev/null || true
        fi
    done
    
    # Wait for graceful shutdown
    sleep 3
    
    # Force kill if needed
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            warn "Force killing process $pid..."
            kill -9 "$pid" 2>/dev/null || true
        fi
    done
    
    log "All nodes stopped"
}

# Show cluster status
show_cluster_status() {
    log "Cluster Status ($NODE_COUNT nodes):"
    echo ""
    
    for ((i=1; i<=NODE_COUNT; i++)); do
        local config
        config=$(get_node_config "$i")
        IFS=':' read -r node_id port admin_port monitor_port data_dir <<< "$config"
        
        echo "Node $i ($node_id):"
        echo "  Raft:     $HOST:$port"
        echo "  Admin:    $HOST:$admin_port"
        echo "  Monitor:  $HOST:$monitor_port"
        echo "  Data:     $data_dir"
        
        # Check if process is running
        if pgrep -f "$node_id" >/dev/null 2>&1; then
            echo -e "  Status:   ${GREEN}RUNNING${NC}"
        else
            echo -e "  Status:   ${RED}STOPPED${NC}"
        fi
        
        # Check if port is listening
        if lsof -i :"$port" >/dev/null 2>&1; then
            echo -e "  Port:     ${GREEN}LISTENING${NC}"
        else
            echo -e "  Port:     ${RED}NOT LISTENING${NC}"
        fi
        
        echo ""
    done
    
    echo "Dashboard: http://$HOST:$DASHBOARD_PORT"
    echo ""
}

# Show cluster information
show_cluster_info() {
    echo ""
    echo -e "${BLUE}🚀 Pickbox $NODE_COUNT-Node Cluster Running!${NC}"
    echo "========================================"
    echo ""
    echo "🌐 Access URLs:"
    echo "   Dashboard:       http://$HOST:$DASHBOARD_PORT"
    echo "   Leader Admin:    http://$HOST:$ADMIN_BASE_PORT"
    echo "   Leader Monitor:  http://$HOST:$MONITOR_BASE_PORT"
    echo ""
    echo "📁 Node Directories (ALL support editing):"
    for ((i=1; i<=NODE_COUNT; i++)); do
        echo "   - $DATA_DIR/node$i/ (edit files here and they replicate everywhere)"
    done
    echo ""
    echo "🧪 Example Commands:"
    echo "   # Create files on any node - they replicate everywhere!"
    for ((i=1; i<=NODE_COUNT; i++)); do
        echo "   echo 'Hello from node$i!' > $DATA_DIR/node$i/test_from_node$i.txt"
    done
    echo ""
    echo "   # Verify replication:"
    echo "   ls -la $DATA_DIR/node*/"
    echo "   cat $DATA_DIR/node*/test_from_*.txt"
    echo ""
    echo "🔄 Key Features:"
    echo "   ✅ Multi-directional replication (any node → all nodes)"
    echo "   ✅ Content hash deduplication (no infinite loops)"
    echo "   ✅ Per-node file state tracking"
    echo "   ✅ Automatic cluster management"
    echo ""
}

# Show logs from all nodes
show_logs() {
    log "Showing logs from all nodes:"
    echo ""
    
    for ((i=1; i<=NODE_COUNT; i++)); do
        local node_id="node$i"
        local log_file="/tmp/${node_id}.log"
        
        if [[ -f "$log_file" ]]; then
            echo -e "${BLUE}=== $node_id logs ===${NC}"
            tail -20 "$log_file" || true
            echo ""
        else
            warn "Log file not found for $node_id"
        fi
    done
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            start|stop|restart|status|clean|logs)
                ACTION="$1"
                shift
                ;;
            -n|--nodes)
                NODE_COUNT="$2"
                shift 2
                ;;
            -p|--base-port)
                BASE_PORT="$2"
                shift 2
                ;;
            -a|--admin-port)
                ADMIN_BASE_PORT="$2"
                shift 2
                ;;
            -m|--monitor-port)
                MONITOR_BASE_PORT="$2"
                shift 2
                ;;
            -d|--dashboard-port)
                DASHBOARD_PORT="$2"
                shift 2
                ;;
            -h|--host)
                HOST="$2"
                shift 2
                ;;
            --data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            --binary)
                BINARY="$2"
                shift 2
                ;;
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Validate configuration
validate_config() {
    if [[ $NODE_COUNT -lt 1 ]]; then
        error "Node count must be at least 1"
        exit 1
    fi
    
    if [[ $NODE_COUNT -gt 20 ]]; then
        warn "Node count > 20 may be excessive for local testing"
    fi
    
    if [[ $BASE_PORT -lt 1024 ]]; then
        error "Base port must be >= 1024"
        exit 1
    fi
    
    # Check for port conflicts
    local max_port=$((BASE_PORT + NODE_COUNT))
    local admin_max_port=$((ADMIN_BASE_PORT + NODE_COUNT))
    local monitor_max_port=$((MONITOR_BASE_PORT + NODE_COUNT))
    
    if [[ $max_port -gt 65535 ]] || [[ $admin_max_port -gt 65535 ]] || [[ $monitor_max_port -gt 65535 ]]; then
        error "Port range exceeds 65535"
        exit 1
    fi
}

# Main function
main() {
    if [[ $# -eq 0 ]]; then
        show_help
        exit 1
    fi
    
    parse_args "$@"
    
    if [[ -n "$CONFIG_FILE" ]]; then
        load_config "$CONFIG_FILE"
    fi
    
    validate_config
    
    case "$ACTION" in
        start)
            start_cluster
            ;;
        stop)
            cleanup_cluster
            ;;
        restart)
            cleanup_cluster
            sleep 2
            start_cluster
            ;;
        status)
            show_cluster_status
            ;;
        clean)
            cleanup_cluster
            ;;
        logs)
            show_logs
            ;;
        *)
            error "Unknown action: $ACTION"
            show_help
            exit 1
            ;;
    esac
}

# Run main function
main "$@" 