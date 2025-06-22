# Test Scripts

This directory contains automated test scripts for the distributed file storage system.

## Available Tests

### `test_replication.sh`
- **Purpose**: Comprehensive test suite that runs all replication tests
- **Usage**: `./test_replication.sh`
- **What it tests**: Runs basic, live, and multi-directional replication tests

### `test_live_replication.sh`
- **Purpose**: Tests live file watching and replication
- **Usage**: `./test_live_replication.sh`
- **What it tests**: File creation, modification, and real-time replication from leader to followers

### `test_multi_replication.sh`
- **Purpose**: Tests multi-directional replication capabilities
- **Usage**: `./test_multi_replication.sh`
- **What it tests**: 
  - File creation from any node replicates to all others
  - Content deduplication to prevent infinite loops
  - Multi-directional consistency guarantees

## Running Tests

To run all tests:
```bash
cd scripts/tests
./test_replication.sh
```

To run individual tests:
```bash
cd scripts/tests
./test_live_replication.sh
./test_multi_replication.sh
```

## Test Requirements

- All test scripts should be run from the `scripts/tests/` directory
- Tests automatically clean up processes and create fresh test environments
- Tests verify content consistency across all nodes in the cluster

## Test Output

Tests provide detailed output showing:
- ✅ Successful operations and verifications
- ❌ Failed operations with error details
- 📊 Summary statistics and final results
- 🧹 Cleanup status 