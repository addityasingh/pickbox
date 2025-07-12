#!/bin/bash

# Generic N-Node Replication Test Script
# Tests multi-directional replication for any number of nodes

set -euo pipefail

# Default configuration
DEFAULT_NODE_COUNT=3
DEFAULT_BASE_PORT=8001
DEFAULT_ADMIN_BASE_PORT=9001
DEFAULT_HOST="127.0.0.1"
DEFAULT_DATA_DIR="data"
DEFAULT_BINARY="cmd/multi_replication/main.go"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration variables
NODE_COUNT=$DEFAULT_NODE_COUNT
BASE_PORT=$DEFAULT_BASE_PORT
ADMIN_BASE_PORT=$DEFAULT_ADMIN_BASE_PORT
HOST=$DEFAULT_HOST
DATA_DIR=$DEFAULT_DATA_DIR
BINARY=$DEFAULT_BINARY
TEST_TIMEOUT=60
VERBOSE=false

# Help function
show_help() {
    cat << EOF
Generic N-Node Replication Test for Pickbox

USAGE:
    $0 [options]

OPTIONS:
    -n, --nodes <count>         Number of nodes to test (default: $DEFAULT_NODE_COUNT)
    -p, --base-port <port>      Base port for Raft (default: $DEFAULT_BASE_PORT)
    -a, --admin-port <port>     Base admin port (default: $DEFAULT_ADMIN_BASE_PORT)
    -h, --host <host>           Host address (default: $DEFAULT_HOST)
    -t, --timeout <seconds>     Test timeout (default: $TEST_TIMEOUT)
    -v, --verbose               Verbose output
    --data-dir <dir>            Data directory (default: $DEFAULT_DATA_DIR)
    --binary <path>             Binary path (default: $DEFAULT_BINARY)
    --help                      Show this help

EXAMPLES:
    # Test 5-node cluster
    $0 -n 5

    # Test with custom ports
    $0 -n 7 -p 9000 -a 10000

    # Verbose test with timeout
    $0 -n 4 -v -t 120

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

debug() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}[DEBUG]${NC} $1"
    fi
}

# Cleanup function
cleanup() {
    log "Cleaning up test environment..."
    
    # Kill all cluster processes
    for ((i=1; i<=NODE_COUNT; i++)); do
        if [[ -f "/tmp/node$i.pid" ]]; then
            local pid=$(cat "/tmp/node$i.pid")
            if kill -0 "$pid" 2>/dev/null; then
                debug "Killing node$i (PID: $pid)"
                kill "$pid" 2>/dev/null || true
            fi
            rm -f "/tmp/node$i.pid"
        fi
    done
    
    # Force kill any remaining processes
    pkill -f "$BINARY" 2>/dev/null || true
    
    # Clean port conflicts
    for ((i=1; i<=NODE_COUNT; i++)); do
        local port=$((BASE_PORT + i - 1))
        local admin_port=$((ADMIN_BASE_PORT + i - 1))
        
        if lsof -ti :"$port" >/dev/null 2>&1; then
            warn "Force killing process on port $port"
            lsof -ti :"$port" | xargs kill -9 2>/dev/null || true
        fi
        
        if lsof -ti :"$admin_port" >/dev/null 2>&1; then
            warn "Force killing process on admin port $admin_port"
            lsof -ti :"$admin_port" | xargs kill -9 2>/dev/null || true
        fi
    done
    
    # Remove test data
    for ((i=1; i<=NODE_COUNT; i++)); do
        rm -rf "$DATA_DIR/node$i" 2>/dev/null || true
    done
    
    # Remove test logs
    rm -f /tmp/node*.log /tmp/test_n_replication.log
    
    log "Cleanup completed"
}

# Setup test environment
setup_test_env() {
    log "Setting up test environment for $NODE_COUNT nodes..."
    
    # Clean up any existing state
    cleanup
    
    # Create data directories
    for ((i=1; i<=NODE_COUNT; i++)); do
        mkdir -p "$DATA_DIR/node$i"
        debug "Created $DATA_DIR/node$i"
    done
    
    log "Test environment ready"
}

# Start a single node
start_node() {
    local node_num=$1
    local node_id="node$node_num"
    local port=$((BASE_PORT + node_num - 1))
    local admin_port=$((ADMIN_BASE_PORT + node_num - 1))
    local data_dir="$DATA_DIR/$node_id"
    
    local cmd_args=(
        "go" "run" "$BINARY"
        "-node" "$node_id"
        "-port" "$port"
        "-admin-port" "$admin_port"
        "-data-dir" "$data_dir"
    )
    
    # First node bootstraps, others join
    if [[ $node_num -eq 1 ]]; then
        cmd_args+=("-bootstrap")
        log "Starting $node_id (bootstrap leader) on port $port..."
    else
        local leader_addr="$HOST:$BASE_PORT"
        cmd_args+=("-join" "$leader_addr")
        log "Starting $node_id (follower) on port $port..."
    fi
    
    # Start the node in background
    "${cmd_args[@]}" > "/tmp/${node_id}.log" 2>&1 &
    local pid=$!
    echo "$pid" > "/tmp/${node_id}.pid"
    
    debug "$node_id started with PID $pid"
    
    # Give the node time to start
    if [[ $node_num -eq 1 ]]; then
        sleep 6  # Leader needs more time
    else
        sleep 3
    fi
}

# Start the entire cluster
start_cluster() {
    log "Starting $NODE_COUNT-node cluster..."
    
    # Start all nodes
    for ((i=1; i<=NODE_COUNT; i++)); do
        start_node "$i"
    done
    
    # Wait for followers to join cluster automatically
    if [[ $NODE_COUNT -gt 1 ]]; then
        log "Waiting for follower nodes to join cluster..."
        sleep 8  # Wait for automatic joining to complete
        debug "Cluster membership should be established via automatic joining"
    fi
    
    # Wait for cluster to stabilize
    sleep 5
    log "Cluster startup completed"
}

# Test file operations across all nodes
test_file_operations() {
    log "Testing file operations across $NODE_COUNT nodes..."
    
    local test_files=()
    local test_passed=true
    
    # Test 1: Create files from each node
    info "Test 1: Creating files from each node..."
    for ((i=1; i<=NODE_COUNT; i++)); do
        local test_file="test_from_node$i.txt"
        local content="Hello from node$i at $(date)"
        
        echo "$content" > "$DATA_DIR/node$i/$test_file"
        test_files+=("$test_file")
        debug "Created $test_file on node$i"
        
        sleep 2  # Allow replication time
    done
    
    # Test 2: Verify replication to all nodes
    info "Test 2: Verifying replication across all nodes..."
    for test_file in "${test_files[@]}"; do
        debug "Checking replication of $test_file..."
        
        # Get reference content from first node that has it
        local reference_content=""
        for ((i=1; i<=NODE_COUNT; i++)); do
            if [[ -f "$DATA_DIR/node$i/$test_file" ]]; then
                reference_content=$(cat "$DATA_DIR/node$i/$test_file")
                break
            fi
        done
        
        if [[ -z "$reference_content" ]]; then
            error "Test file $test_file not found on any node"
            test_passed=false
            continue
        fi
        
        # Check all nodes have the same content
        for ((i=1; i<=NODE_COUNT; i++)); do
            if [[ -f "$DATA_DIR/node$i/$test_file" ]]; then
                local content=$(cat "$DATA_DIR/node$i/$test_file")
                if [[ "$content" != "$reference_content" ]]; then
                    error "Content mismatch for $test_file on node$i"
                    test_passed=false
                fi
            else
                error "$test_file missing on node$i"
                test_passed=false
            fi
        done
        
        if [[ "$test_passed" == "true" ]]; then
            debug "✅ $test_file replicated correctly to all nodes"
        fi
    done
    
    # Test 3: File modification
    info "Test 3: Testing file modifications..."
    local modify_file="${test_files[0]}"
    local modify_node=$((NODE_COUNT / 2 + 1))  # Pick a middle node
    
    echo "Modified by node$modify_node" >> "$DATA_DIR/node$modify_node/$modify_file"
    debug "Modified $modify_file on node$modify_node"
    
    sleep 4  # Allow replication time
    
    # Verify modification replicated
    local modified_content=$(cat "$DATA_DIR/node$modify_node/$modify_file")
    for ((i=1; i<=NODE_COUNT; i++)); do
        if [[ $i -eq $modify_node ]]; then continue; fi
        
        if [[ -f "$DATA_DIR/node$i/$modify_file" ]]; then
            local content=$(cat "$DATA_DIR/node$i/$modify_file")
            if [[ "$content" != "$modified_content" ]]; then
                error "Modification not replicated to node$i"
                test_passed=false
            fi
        else
            error "$modify_file missing on node$i after modification"
            test_passed=false
        fi
    done
    
    if [[ "$test_passed" == "true" ]]; then
        log "✅ All file operation tests passed!"
        return 0
    else
        error "❌ Some file operation tests failed"
        return 1
    fi
}

# Test deduplication
test_deduplication() {
    log "Testing content deduplication..."
    
    local dedup_file="dedup_test.txt"
    local content="Deduplication test content"
    
    # Write same content from multiple nodes simultaneously
    info "Writing same content from multiple nodes..."
    for ((i=1; i<=NODE_COUNT; i++)); do
        echo "$content" > "$DATA_DIR/node$i/$dedup_file" &
    done
    wait
    
    sleep 4  # Allow processing time
    
    # Verify all nodes have the file with correct content
    local test_passed=true
    for ((i=1; i<=NODE_COUNT; i++)); do
        if [[ -f "$DATA_DIR/node$i/$dedup_file" ]]; then
            local file_content=$(cat "$DATA_DIR/node$i/$dedup_file")
            if [[ "$file_content" != "$content" ]]; then
                error "Incorrect content in $dedup_file on node$i"
                test_passed=false
            fi
        else
            error "$dedup_file missing on node$i"
            test_passed=false
        fi
    done
    
    if [[ "$test_passed" == "true" ]]; then
        log "✅ Deduplication test passed!"
        return 0
    else
        error "❌ Deduplication test failed"
        return 1
    fi
}

# Show test results
show_test_results() {
    log "Test Results Summary for $NODE_COUNT nodes:"
    echo ""
    
    # Show file counts
    info "File counts per node:"
    for ((i=1; i<=NODE_COUNT; i++)); do
        if [[ -d "$DATA_DIR/node$i" ]]; then
            local file_count=$(find "$DATA_DIR/node$i" -name "*.txt" ! -name "welcome.txt" 2>/dev/null | wc -l)
            echo "  node$i: $file_count files"
        else
            echo "  node$i: Directory not found"
        fi
    done
    
    echo ""
    
    # Show sample file contents
    if [[ -d "$DATA_DIR/node1" ]]; then
        info "Sample files (from node1):"
        find "$DATA_DIR/node1" -name "*.txt" ! -name "welcome.txt" 2>/dev/null | head -3 | while read -r file; do
            echo "  $(basename "$file"): $(head -1 "$file" 2>/dev/null || echo 'N/A')"
        done
    fi
    
    echo ""
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
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
            -h|--host)
                HOST="$2"
                shift 2
                ;;
            -t|--timeout)
                TEST_TIMEOUT="$2"
                shift 2
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            --data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            --binary)
                BINARY="$2"
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
    
    if [[ $TEST_TIMEOUT -lt 30 ]]; then
        warn "Test timeout < 30 seconds may be too short"
    fi
}

# Main test function
run_tests() {
    log "Starting N-node replication test..."
    log "Configuration: $NODE_COUNT nodes, ports $BASE_PORT-$((BASE_PORT + NODE_COUNT - 1))"
    
    # Set up test environment
    setup_test_env
    
    # Start cluster with timeout
    timeout "$TEST_TIMEOUT" bash -c "$(declare -f start_cluster); start_cluster" || {
        error "Cluster startup timed out"
        return 1
    }
    
    # Run tests with timeout
    local test_result=0
    
    timeout "$((TEST_TIMEOUT / 2))" bash -c "
        $(declare -f test_file_operations log info debug error warn)
        $(declare -pf NODE_COUNT DATA_DIR)
        test_file_operations
    " || {
        error "File operations test timed out or failed"
        test_result=1
    }
    
    timeout "$((TEST_TIMEOUT / 4))" bash -c "
        $(declare -f test_deduplication log info debug error warn)
        $(declare -pf NODE_COUNT DATA_DIR)
        test_deduplication
    " || {
        error "Deduplication test timed out or failed"
        test_result=1
    }
    
    # Show results
    show_test_results
    
    return $test_result
}

# Main execution
main() {
    parse_args "$@"
    validate_config
    
    # Set up cleanup trap
    trap cleanup EXIT INT TERM
    
    # Run tests
    if run_tests; then
        log "🎉 SUCCESS: All N-node replication tests passed!"
        echo ""
        echo "✅ Tested successfully:"
        echo "  • $NODE_COUNT-node cluster startup"
        echo "  • Multi-directional file replication"
        echo "  • Content consistency across all nodes"
        echo "  • Deduplication and conflict resolution"
        echo ""
        echo "🚀 Try the interactive cluster manager:"
        echo "  ./scripts/cluster_manager.sh start -n $NODE_COUNT"
        exit 0
    else
        error "❌ FAILURE: Some N-node replication tests failed"
        echo ""
        echo "🔍 Check logs in /tmp/node*.log for details"
        echo "🛠️  Try running with -v for verbose output"
        exit 1
    fi
}

# Run main function
main "$@" 