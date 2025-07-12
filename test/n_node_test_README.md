# N-Node Test Suite for Pickbox

This directory contains comprehensive tests for the N-node implementation of the Pickbox distributed storage system. The test suite validates cluster formation, replication, failure scenarios, and performance characteristics across different cluster sizes.

## Test Structure

### Test Files Overview

| File | Purpose | Test Types |
|------|---------|------------|
| `n_node_test.go` | Unit tests for N-node configuration and logic | Unit Tests |
| `n_node_integration_test.go` | Integration tests for actual cluster behavior | Integration Tests |
| `n_node_replication_test.go` | Multi-directional replication testing | Replication Tests |
| `n_node_failure_test.go` | Node failure and recovery scenarios | Failure Tests |
| `n_node_performance_test.go` | Performance and scaling benchmarks | Performance Tests |

## Running Tests

### Prerequisites

1. **Go Environment**: Go 1.19+ installed
2. **Dependencies**: Run `go mod tidy` to install test dependencies
3. **System Tools**: Ensure `pkill`, `lsof`, and `nc` (netcat) are available
4. **Clean Environment**: No existing Pickbox processes running

### Basic Test Commands

```bash
# Run all N-node tests
go test ./test/n_node*

# Run specific test file
go test ./test/n_node_test.go
go test ./test/n_node_integration_test.go

# Run with verbose output
go test -v ./test/n_node*

# Run only unit tests (fast)
go test ./test/n_node_test.go

# Skip integration tests (for CI/CD)
go test -short ./test/n_node*
```

### Test Categories

#### 1. Unit Tests (`n_node_test.go`)

**Purpose**: Validate configuration logic, port calculations, and basic N-node functionality.

```bash
# Run unit tests only
go test -v ./test/n_node_test.go

# Run specific unit test
go test -v -run TestNNodeConfiguration ./test/n_node_test.go
```

**Key Tests**:
- `TestNNodeConfiguration` - Configuration validation
- `TestPortCalculation` - Port assignment logic
- `TestAdminAddressDerivation` - Admin port derivation
- `TestClusterSizeValidation` - Cluster size limits
- `TestBootstrapLogic` - Bootstrap decision logic

#### 2. Integration Tests (`n_node_integration_test.go`)

**Purpose**: Test actual cluster formation and basic operations with real processes.

```bash
# Run integration tests
go test -v ./test/n_node_integration_test.go

# Test specific cluster size
go test -v -run TestFiveNodeCluster ./test/n_node_integration_test.go
```

**Key Tests**:
- `TestThreeNodeCluster` - Basic 3-node cluster
- `TestFiveNodeCluster` - 5-node cluster with concurrent operations
- `TestSevenNodeCluster` - Larger cluster testing
- `TestClusterScaling` - Different cluster sizes
- `TestClusterWithCustomPorts` - Port configuration

**Test Environment**:
- Uses `scripts/cluster_manager.sh` for cluster management
- Creates temporary test directories
- Automatic cleanup after tests

#### 3. Replication Tests (`n_node_replication_test.go`)

**Purpose**: Comprehensive testing of multi-directional file replication.

```bash
# Run replication tests
go test -v ./test/n_node_replication_test.go

# Test specific replication scenario
go test -v -run TestMultiDirectionalReplication ./test/n_node_replication_test.go
```

**Key Tests**:
- `TestMultiDirectionalReplication` - File replication from each node
- `TestConcurrentMultiNodeWrites` - Concurrent write operations
- `TestFileModificationPropagation` - File updates across nodes
- `TestFileDeletionPropagation` - File deletion replication
- `TestReplicationLargeFiles` - Large file handling
- `TestBinaryFileReplication` - Binary file integrity

**Replication Features Tested**:
- ✅ Multi-directional replication (any node → all nodes)
- ✅ Content consistency across all nodes
- ✅ Nested directory structures
- ✅ Large file handling (up to 1MB)
- ✅ Binary file integrity
- ✅ Concurrent operations
- ✅ File modification tracking

#### 4. Failure Tests (`n_node_failure_test.go`)

**Purpose**: Test cluster resilience and recovery capabilities.

```bash
# Run failure tests
go test -v ./test/n_node_failure_test.go

# Test specific failure scenario
go test -v -run TestSingleNodeFailureRecovery ./test/n_node_failure_test.go
```

**Key Tests**:
- `TestSingleNodeFailureRecovery` - Single node failure and recovery
- `TestMultipleNodeFailures` - Multiple simultaneous failures
- `TestLeaderFailureAndElection` - Leader election after failure
- `TestClusterPartitioning` - Network partition simulation
- `TestGracefulVsUngracefulShutdown` - Different shutdown types
- `TestConcurrentFailuresDuringLoad` - Failures under load

**Failure Scenarios**:
- ✅ Single node failure and recovery
- ✅ Multiple node failures
- ✅ Leader failure and election
- ✅ Network partitioning
- ✅ Graceful vs ungraceful shutdowns
- ✅ Rapid failure/recovery cycles
- ✅ Failures during heavy load

#### 5. Performance Tests (`n_node_performance_test.go`)

**Purpose**: Measure performance characteristics and scaling behavior.

```bash
# Run performance tests
go test -v ./test/n_node_performance_test.go

# Run specific performance test
go test -v -run TestThroughputScaling ./test/n_node_performance_test.go
```

**Key Tests**:
- `TestThroughputScaling` - Throughput vs cluster size
- `TestConcurrentWritePerformance` - Concurrent operation performance
- `TestFileSizePerformance` - Performance vs file size
- `TestClusterStartupPerformance` - Startup time scaling
- `TestLatencyDistribution` - Latency characteristics
- `TestPeakLoadResourceUsage` - Resource usage under load

**Performance Metrics**:
- ✅ Operations per second
- ✅ Bytes per second throughput
- ✅ Average operation latency
- ✅ Average replication latency
- ✅ Cluster startup time
- ✅ Memory usage patterns
- ✅ Latency distribution (min/max/avg/percentiles)

## Test Configuration

### Environment Variables

```bash
# Set test timeouts (optional)
export PICKBOX_TEST_TIMEOUT=120s
export PICKBOX_CLUSTER_START_DELAY=15s

# Enable debug logging
export PICKBOX_TEST_DEBUG=true

# Custom test data directory
export PICKBOX_TEST_DATA_DIR=/tmp/pickbox-test-data
```

### Test Flags

```bash
# Skip slow integration tests
go test -short ./test/n_node*

# Run with race detection
go test -race ./test/n_node*

# Run specific test pattern
go test -run "TestThree.*" ./test/n_node*

# Set custom timeout
go test -timeout 300s ./test/n_node*

# Generate coverage report
go test -cover ./test/n_node*
```

## Cluster Sizes Tested

The test suite validates the following cluster configurations:

| Cluster Size | Test Coverage | Use Case |
|--------------|---------------|----------|
| 1 node | Basic functionality | Development |
| 3 nodes | Standard cluster | Production minimum |
| 5 nodes | Fault tolerance | Recommended production |
| 7 nodes | Large cluster | High availability |
| 10+ nodes | Scalability | Enterprise scale |

## Test Results Interpretation

### Success Criteria

✅ **Unit Tests**: All configuration and logic tests pass
✅ **Integration Tests**: Clusters form successfully and handle basic operations
✅ **Replication Tests**: Files replicate consistently across all nodes
✅ **Failure Tests**: Cluster recovers gracefully from node failures
✅ **Performance Tests**: Throughput and latency meet acceptable thresholds

### Performance Benchmarks

Based on test results, typical performance characteristics:

| Metric | 3-Node Cluster | 5-Node Cluster | Notes |
|--------|----------------|----------------|-------|
| Startup Time | ~15 seconds | ~20 seconds | Includes cluster formation |
| File Replication | ~100ms | ~150ms | 1KB files |
| Throughput | ~50 ops/sec | ~40 ops/sec | Under load |
| Large Files (100KB) | ~500ms | ~750ms | Replication time |

## Troubleshooting

### Common Issues

1. **Port Conflicts**
   ```bash
   # Check for conflicting processes
   lsof -i :8001-8010
   lsof -i :9001-9010
   
   # Kill conflicting processes
   ./scripts/cluster_manager.sh stop -n 5
   ```

2. **Test Timeouts**
   ```bash
   # Increase test timeout
   go test -timeout 600s ./test/n_node*
   
   # Check system resources
   top
   df -h
   ```

3. **Flaky Tests**
   ```bash
   # Run tests multiple times
   for i in {1..5}; do go test ./test/n_node_integration_test.go; done
   
   # Check for resource constraints
   ulimit -n  # File descriptor limit
   free -h    # Memory usage
   ```

### Debug Information

Enable verbose logging for troubleshooting:

```bash
# Enable debug output
go test -v -run TestFailingTest ./test/n_node*

# Check cluster logs
./scripts/cluster_manager.sh logs -n 5

# Check individual node logs
tail -f /tmp/node1.log
tail -f /tmp/node2.log
```

### Test Data Cleanup

```bash
# Manual cleanup if tests fail
./scripts/cluster_manager.sh clean
rm -rf /tmp/pickbox-*-test
pkill -f "multi_replication"
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: N-Node Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.19'
    
    - name: Run Unit Tests
      run: go test ./test/n_node_test.go
    
    - name: Run Integration Tests
      run: go test -short ./test/n_node_integration_test.go
    
    - name: Run Performance Tests
      run: go test -short ./test/n_node_performance_test.go
```

### Jenkins Pipeline

```groovy
pipeline {
    agent any
    stages {
        stage('Test') {
            parallel {
                stage('Unit Tests') {
                    steps {
                        sh 'go test ./test/n_node_test.go'
                    }
                }
                stage('Integration Tests') {
                    steps {
                        sh 'go test -short ./test/n_node_integration_test.go'
                    }
                }
            }
        }
    }
}
```

## Best Practices

### Running Tests Locally

1. **Clean Environment**: Always start with a clean environment
2. **Resource Monitoring**: Monitor CPU and memory usage during tests
3. **Serial Execution**: Run integration tests serially to avoid conflicts
4. **Log Collection**: Save logs for failed tests for debugging

### Writing New Tests

1. **Use Test Suites**: Leverage existing test suite patterns
2. **Cleanup**: Always implement proper cleanup in `t.Cleanup()`
3. **Timeouts**: Set appropriate timeouts for operations
4. **Deterministic**: Ensure tests are deterministic and repeatable

### Performance Testing

1. **Baseline**: Establish performance baselines for different cluster sizes
2. **Resource Limits**: Test under various resource constraints
3. **Load Patterns**: Test different load patterns (burst, sustained, etc.)
4. **Metrics Collection**: Collect comprehensive metrics for analysis

## Contributing

When adding new tests:

1. Follow the existing test suite patterns
2. Add comprehensive documentation
3. Include both positive and negative test cases
4. Ensure proper cleanup and error handling
5. Update this README with new test descriptions

## Support

For issues with the test suite:

1. Check existing logs and debug output
2. Verify system requirements and dependencies
3. Review troubleshooting section
4. Create issues with detailed reproduction steps

---

This test suite provides comprehensive validation of the N-node Pickbox implementation across multiple dimensions: functionality, reliability, performance, and scalability. 