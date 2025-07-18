#!/bin/bash

echo "🧹 Performing thorough cleanup..."

# Kill all replication-related processes
echo "Killing replication processes..."
pkill -f "raft_demo" 2>/dev/null || true
pkill -f "cmd/raft_demo" 2>/dev/null || true

# Wait a moment
sleep 2

# Check for any processes still using our ports
echo "Checking for processes using ports 8001-8003, 9001-9003..."
if lsof -i :8001-8003 -i :9001-9003 >/dev/null 2>&1; then
    echo "Found processes still using ports, force killing..."
    # Get PIDs and kill them
    pids=$(lsof -ti :8001-8003,:9001-9003 2>/dev/null)
    if [ -n "$pids" ]; then
        echo "Force killing PIDs: $pids"
        kill -9 $pids 2>/dev/null || true
        sleep 1
    fi
fi

# Remove all node data
echo "Removing node data..."
rm -rf data/node{1,2,3} 2>/dev/null || true

# Final check
echo "Final port check..."
if lsof -i :8001-8003 -i :9001-9003 >/dev/null 2>&1; then
    echo "❌ Warning: Some ports may still be in use"
    lsof -i :8001-8003 -i :9001-9003
else
    echo "✅ All ports are now free"
fi

echo "✅ Cleanup complete!" 