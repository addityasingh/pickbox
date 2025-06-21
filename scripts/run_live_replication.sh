#!/bin/bash

# Clean up any existing processes and data
echo "🧹 Cleaning up existing processes and data..."
pkill -f "live_replication" 2>/dev/null || true
pkill -f "cmd/live_replication" 2>/dev/null || true
sleep 2
rm -rf data/node{1,2,3}

echo "🚀 Starting Live Replication Demo"
echo "================================="
echo ""

# Start node1 (leader)
echo "Starting node1 (leader) with file watching..."
go run cmd/live_replication/main.go -node node1 -port 8001 &
NODE1_PID=$!
sleep 4

# Start node2
echo "Starting node2..."
go run cmd/live_replication/main.go -node node2 -port 8002 -join 127.0.0.1:8001 &
NODE2_PID=$!
sleep 2

# Start node3
echo "Starting node3..."
go run cmd/live_replication/main.go -node node3 -port 8003 -join 127.0.0.1:8001 &
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
echo "✅ Live replication cluster is running!"
echo "======================================"
echo ""
echo "🧪 Testing Instructions:"
echo "1. Open another terminal"
echo "2. Edit files in data/node1/ (the leader)"
echo "3. Watch them automatically replicate to data/node2/ and data/node3/"
echo ""
echo "Example commands to try:"
echo "  echo 'Hello World!' > data/node1/hello.txt"
echo "  echo 'Line 2' >> data/node1/hello.txt"
echo "  cp /etc/hosts data/node1/hosts_copy.txt"
echo ""
echo "Then check replication with:"
echo "  cat data/node*/hello.txt"
echo "  cat data/node*/hosts_copy.txt"
echo ""
echo "📁 Node directories:"
echo "  - data/node1/ (leader - edit files here)"
echo "  - data/node2/ (follower - files appear here)"
echo "  - data/node3/ (follower - files appear here)"
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
    sleep 10
    show_status
done 