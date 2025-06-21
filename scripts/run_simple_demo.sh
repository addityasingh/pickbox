#!/bin/bash

echo "Running simple replication demo..."
echo "=================================="

# Build and run the demo
go run cmd/simple_demo/main.go

echo ""
echo "Demo completed! Check the results:"
echo ""

# Show the file contents to verify replication
for node in node1 node2 node3; do
    echo "=== Contents of data/$node/test.txt ==="
    if [ -f "data/$node/test.txt" ]; then
        cat "data/$node/test.txt"
    else
        echo "File not found!"
    fi
    echo ""
done

echo "If all files show identical content, replication is working!" 