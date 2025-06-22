#!/bin/bash

# Clean up any existing processes and data
echo "🧹 Cleaning up existing processes and data..."
./scripts/cleanup_replication.sh

echo "🚀 Starting Multi-Directional Replication Demo"
echo "=============================================="
echo ""

# Start node1 (leader)
echo "Starting node1 (leader) with multi-directional file watching..."
go run cmd/multi_replication/main.go -node node1 -port 8001 &
NODE1_PID=$!
sleep 4

# Start node2
echo "Starting node2 with multi-directional file watching..."
go run cmd/multi_replication/main.go -node node2 -port 8002 -join 127.0.0.1:8001 &
NODE2_PID=$!
sleep 2

# Start node3
echo "Starting node3 with multi-directional file watching..."
go run cmd/multi_replication/main.go -node node3 -port 8003 -join 127.0.0.1:8001 &
NODE3_PID=$!
sleep 2

# Add nodes to cluster
echo "🔗 Adding nodes to cluster..."
if command -v nc &> /dev/null; then
    echo "ADD_VOTER node2 127.0.0.1:8002" | nc 127.0.0.1 9001
    sleep 1
    echo "ADD_VOTER node3 127.0.0.1:8003" | nc 127.0.0.1 9001
    sleep 2
else
    echo "⚠️  netcat not available, cluster will run in single-node mode"
fi

echo ""
echo "✅ Multi-directional replication cluster is running!"
echo "===================================================="
echo ""
echo "🧪 Testing Instructions:"
echo "1. Open another terminal"
echo "2. Edit files in ANY node directory: data/node1/, data/node2/, or data/node3/"
echo "3. Watch them automatically replicate to ALL other nodes"
echo ""
echo "Example commands to try:"
echo "  # Test from node1:"
echo "  echo 'Hello from node1!' > data/node1/test_from_node1.txt"
echo ""
echo "  # Test from node2:"
echo "  echo 'Hello from node2!' > data/node2/test_from_node2.txt"
echo ""
echo "  # Test from node3:"
echo "  echo 'Hello from node3!' > data/node3/test_from_node3.txt"
echo ""
echo "Then check replication with:"
echo "  ls -la data/node*/"
echo "  cat data/node*/test_from_*.txt"
echo ""
echo "📁 Node directories (ALL support editing):"
echo "  - data/node1/ (edit files here and they replicate everywhere)"
echo "  - data/node2/ (edit files here and they replicate everywhere)"
echo "  - data/node3/ (edit files here and they replicate everywhere)"
echo ""
echo "🔄 Key improvements:"
echo "  ✅ Multi-directional replication (any node → all nodes)"
echo "  ✅ Content hash deduplication (no infinite loops)"
echo "  ✅ Per-node file state tracking"
echo "  ✅ Improved conflict resolution"
echo ""
echo "🛑 Press Ctrl+C to stop all nodes"

# Function to show live status
show_status() {
    echo ""
    echo "📊 Current Status:"
    echo "=================="
    for node in node1 node2 node3; do
        if [ -d "data/$node" ]; then
            file_count=$(find data/$node -name "*.txt" 2>/dev/null | wc -l)
            echo "  $node: $file_count text files"
            # Show latest file for each node
            latest=$(find data/$node -name "*.txt" -not -name "welcome.txt" 2>/dev/null | head -1)
            if [ -n "$latest" ]; then
                echo "    Latest: $(basename "$latest")"
            fi
        fi
    done
}

# Cleanup function
cleanup() {
    echo ""
    echo "🛑 Stopping all nodes..."
    kill $NODE1_PID $NODE2_PID $NODE3_PID 2>/dev/null
    wait
    echo "✅ All nodes stopped."
    show_status
}

trap cleanup EXIT

# Show periodic status updates
while true; do
    sleep 15
    show_status
done 