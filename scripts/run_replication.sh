#!/bin/bash

# Kill any existing processes
pkill -f "replication"

# Clean up existing data and Raft state
rm -rf data/node{1,2,3}
rm -f data/*.db
rm -f data/*.snap

# Create fresh data directories
mkdir -p data/node{1,2,3}

# Start node 1 (leader)
echo "Starting node1..."
go run cmd/replication/main.go -node-id node1 -port 8001 &
NODE1_PID=$!
sleep 5

# Start node 2
echo "Starting node2..."
go run cmd/replication/main.go -node-id node2 -port 8002 &
NODE2_PID=$!
sleep 2

# Start node 3
echo "Starting node3..."
go run cmd/replication/main.go -node-id node3 -port 8003 &
NODE3_PID=$!
sleep 2

echo "All nodes started."
echo "Node1 PID: $NODE1_PID"
echo "Node2 PID: $NODE2_PID"
echo "Node3 PID: $NODE3_PID"

# Wait for nodes to initialize, then add them to cluster
echo "Waiting for nodes to initialize..."
sleep 3

echo "Adding nodes to cluster..."
go run scripts/add_nodes.go

echo "Cluster setup complete. Press Ctrl+C to stop all nodes."

# Function to cleanup on exit
cleanup() {
    echo "Stopping all nodes..."
    kill $NODE1_PID $NODE2_PID $NODE3_PID 2>/dev/null
    wait
    echo "All nodes stopped."
}

# Set trap to cleanup on script exit
trap cleanup EXIT

# Wait for user to press Ctrl+C
wait 