#!/bin/bash

# Clean up any existing processes and data
pkill -f "raft_demo"
rm -rf data/node{1,2,3}

echo "Starting Raft cluster demo..."
echo "============================="

# Start node1 (leader)
echo "Starting node1 (leader)..."
go run cmd/raft_demo/main.go -node node1 -port 8001 &
NODE1_PID=$!
sleep 3

# Start node2
echo "Starting node2..."
go run cmd/raft_demo/main.go -node node2 -port 8002 -join 127.0.0.1:8001 &
NODE2_PID=$!
sleep 2

# Start node3
echo "Starting node3..."
go run cmd/raft_demo/main.go -node node3 -port 8003 -join 127.0.0.1:8001 &
NODE3_PID=$!
sleep 2

# Add node2 and node3 to the cluster
echo "Adding nodes to cluster..."
echo "ADD_VOTER node2 127.0.0.1:8002" | nc 127.0.0.1 9001
sleep 1
echo "ADD_VOTER node3 127.0.0.1:8003" | nc 127.0.0.1 9001
sleep 2

echo ""
echo "Cluster started! Checking replication..."
echo "========================================"

# Wait a moment for replication
sleep 3

# Check that files were replicated
echo "Checking file replication across nodes:"
for node in node1 node2 node3; do
    if [ -f "data/$node/test.txt" ]; then
        echo "✓ $node: $(cat data/$node/test.txt)"
    else
        echo "✗ $node: File not found!"
    fi
done

echo ""
echo "Demo is running! You can:"
echo "1. Check the data/ directories to see replicated files"
echo "2. Edit files in data/node1/ and see them replicate to other nodes"
echo "3. Press Ctrl+C to stop all nodes"

# Cleanup function
cleanup() {
    echo ""
    echo "Stopping all nodes..."
    kill $NODE1_PID $NODE2_PID $NODE3_PID 2>/dev/null
    wait
    echo "All nodes stopped."
}

trap cleanup EXIT

# Keep running until user interrupts
wait 