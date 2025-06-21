#!/bin/bash

echo "🧪 Testing Live Replication"
echo "============================"

# Clean up any existing processes first
echo "Cleaning up any existing processes..."
chmod +x scripts/cleanup_replication.sh
./scripts/cleanup_replication.sh

# Start the live replication in background
echo "Starting live replication cluster..."
./scripts/run_live_replication.sh > /tmp/live_replication.log 2>&1 &
DEMO_PID=$!

# Wait for cluster to start
echo "Waiting for cluster to initialize..."
sleep 12

# Test file operations
echo ""
echo "🔧 Testing file operations..."

# Test 1: Create a file
echo "Test 1: Creating test1.txt..."
echo "Hello from live replication test!" > data/node1/test1.txt
sleep 4

# Check replication
echo "Checking replication..."
for node in node1 node2 node3; do
    if [ -f "data/$node/test1.txt" ]; then
        echo "✅ $node: $(cat data/$node/test1.txt)"
    else
        echo "❌ $node: File not found!"
    fi
done

echo ""

# Test 2: Modify the file
echo "Test 2: Modifying test1.txt..."
echo "Modified content!" >> data/node1/test1.txt
sleep 4

# Check replication
echo "Checking replication after modification..."
for node in node1 node2 node3; do
    if [ -f "data/$node/test1.txt" ]; then
        lines=$(wc -l < "data/$node/test1.txt")
        echo "✅ $node: $lines lines"
    else
        echo "❌ $node: File not found!"
    fi
done

echo ""

# Test 3: Create another file
echo "Test 3: Creating test2.txt..."
echo "Another test file" > data/node1/test2.txt
sleep 4

# Check file count
echo "Checking total file count..."
for node in node1 node2 node3; do
    if [ -d "data/$node" ]; then
        file_count=$(find data/$node -name "*.txt" ! -name "welcome.txt" 2>/dev/null | wc -l)
        echo "✅ $node: $file_count test files"
    else
        echo "❌ $node: Directory not found!"
    fi
done

echo ""
echo "📊 Test Results Summary:"
echo "========================"

# Final verification
all_good=true

for node in node2 node3; do  # Check followers
    if [ ! -f "data/$node/test1.txt" ] || [ ! -f "data/$node/test2.txt" ]; then
        echo "❌ $node: Missing replicated files"
        all_good=false
    else
        # Check content matches
        if cmp -s "data/node1/test1.txt" "data/$node/test1.txt" && cmp -s "data/node1/test2.txt" "data/$node/test2.txt"; then
            echo "✅ $node: All files replicated correctly"
        else
            echo "❌ $node: File content mismatch"
            all_good=false
        fi
    fi
done

# Cleanup
echo ""
echo "🧹 Cleaning up..."
kill $DEMO_PID 2>/dev/null
wait

if [ "$all_good" = true ]; then
    echo "🎉 SUCCESS: Live replication is working perfectly!"
    echo ""
    echo "📋 What was tested:"
    echo "  ✅ File creation replication"
    echo "  ✅ File modification replication"
    echo "  ✅ Multiple file replication"
    echo "  ✅ Content consistency across nodes"
    echo ""
    echo "🎯 Try the interactive demo with: ./scripts/run_live_replication.sh"
else
    echo "❌ FAILURE: Some replication tests failed"
    echo "Check the logs in /tmp/live_replication.log"
fi 