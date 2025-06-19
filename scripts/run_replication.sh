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
go run cmd/replication/main.go -node-id node1 -port 8001 &
sleep 5

# Start node 2
go run cmd/replication/main.go -node-id node2 -port 8005 &
sleep 5

# Start node 3
go run cmd/replication/main.go -node-id node3 -port 8009 &

echo "All nodes started. Press Ctrl+C to stop."
wait 