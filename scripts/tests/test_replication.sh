#!/bin/bash

echo "Testing Replication System"
echo "=========================="

echo ""
echo "1. Running basic replication demo..."
echo "------------------------------------"
../run_replication.sh

echo ""
echo "2. Testing live replication..."
echo "------------------------------"
./test_live_replication.sh

echo ""
echo "3. Testing multi-directional replication..."
echo "-------------------------------------------"
./test_multi_replication.sh

echo ""
echo "4. Summary"
echo "----------"
echo "✓ Basic replication demo completed"
echo "✓ Live replication tests completed"
echo "✓ Multi-directional replication tests completed"

echo ""
echo "Check the data/ directory to see the replicated files:"
ls -la data/ 2>/dev/null || echo "No data directory found"

echo ""
echo "All tests completed!" 