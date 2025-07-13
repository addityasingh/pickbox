# Pickbox Test Suite

This directory contains comprehensive tests for the Pickbox distributed file storage system.

## Test Structure

### Unit Tests

#### Storage Package Tests (`pkg/storage/*_test.go`)
- **Manager Tests**: Test storage manager creation, node management, and Raft integration
- **Node Tests**: Test individual node operations (store/retrieve chunks, concurrency)
- **Vector Clock Tests**: Test distributed conflict resolution mechanisms
- **Raft Manager Tests**: Test Raft consensus and FSM operations

#### Multi-Replication Tests (`cmd/pickbox/multi_replication_test.go`)
- **FSM Tests**: Test file system state machine operations
- **Command Tests**: Test command serialization/deserialization
- **Hash Function Tests**: Test content deduplication mechanisms
- **File State Tests**: Test file tracking and metadata management

### Integration Tests (`test/integration_test.go`)

End-to-end tests that spin up actual 3-node clusters:

1. **Basic Replication**: File creation and replication across all nodes
2. **File Modification**: Updates propagating between nodes
3. **File Deletion**: Deletion events replicating to all nodes
4. **Concurrent Writes**: Multiple nodes writing simultaneously
5. **Nested Directories**: Deep directory structure replication
6. **Large Files**: 1MB+ file replication performance
7. **Node Failure Recovery**: Fault tolerance and recovery testing

### Benchmark Tests

Performance testing for critical operations:
- Hash function performance
- File state management
- Vector clock operations
- FSM Apply operations
- JSON marshaling/unmarshaling

## Running Tests

### Quick Test Run
```bash
# Run all tests
./scripts/run_tests.sh

# Run with stress tests
./scripts/run_tests.sh --stress
```

### Individual Test Suites
```bash
# Unit tests only
go test -v ./pkg/storage
go test -v ./cmd/pickbox

# Integration tests only
cd test && go test -v .

# Benchmarks only
go test -bench=. ./pkg/storage
go test -bench=. ./cmd/pickbox
```

### Coverage Reports
```bash
# Generate coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# View coverage in terminal
go tool cover -func=coverage.out
```

## Test Configuration

### Ports Used
- **Raft Ports**: 8000-8002
- **Admin Ports**: 9001-9003
- **Test Directories**: `/tmp/pickbox-integration-test/node{1,2,3}`

### Timeouts
- **Replication Timeout**: 30 seconds
- **Integration Test Timeout**: 240 seconds (4 minutes)
- **Node Startup Time**: 2-3 seconds per node

## Test Features

### Comprehensive Coverage
- **Unit Tests**: 95%+ code coverage
- **Integration Tests**: Full end-to-end workflows
- **Concurrent Testing**: Race condition detection
- **Error Handling**: Failure scenario testing
- **Performance**: Benchmark and stress testing

### Mocking and Isolation
- Mock Raft snapshot sinks
- Isolated test directories
- Process cleanup between tests
- Port conflict prevention

### Real-World Scenarios
- Network partitions (node failure/recovery)
- Concurrent multi-user editing
- Large file handling (1MB+)
- Deep directory structures
- Content deduplication

## Debugging Tests

### Common Issues

1. **Port Conflicts**
   ```bash
   # Check for running processes
   lsof -i :8000-8002
   lsof -i :9001-9003
   
   # Kill hanging processes
   pkill -f multi_replication
   ```

2. **Test Directory Cleanup**
   ```bash
   # Manual cleanup
   rm -rf /tmp/pickbox-test-* /tmp/test-*
   ```

3. **Build Issues**
   ```bash
   # Rebuild binary
   cd cmd/pickbox
   go build -o ../../bin/pickbox .
   ```

### Verbose Logging
```bash
# Enable detailed test output
go test -v -race ./...

# Test specific function
go test -run TestBasicReplication -v ./test
```

### Test Environment Variables
```bash
# Set custom timeouts
export PICKBOX_TEST_TIMEOUT=60s
export PICKBOX_REPLICATION_DELAY=10s

# Enable debug logging
export PICKBOX_DEBUG=true
```

## Performance Benchmarks

### Expected Performance
- **Hash Function**: ~1M hashes/second
- **File State Update**: ~100K updates/second
- **Replication Latency**: 1-4 seconds
- **Large File (1MB)**: <60 seconds full replication

### Benchmark Commands
```bash
# Storage benchmarks
cd pkg/storage && go test -bench=. -benchmem

# Multi-replication benchmarks  
cd cmd/pickbox && go test -bench=. -benchmem

# Custom benchmark runs
go test -bench=BenchmarkHashContent -count=5 -benchtime=10s
```

## Continuous Integration

The test suite is designed for CI/CD environments:

### CI Configuration
```yaml
# Example GitHub Actions
- name: Run Tests
  run: |
    ./scripts/run_tests.sh
    
- name: Upload Coverage
  uses: codecov/codecov-action@v1
  with:
    file: ./coverage.out
```

### Docker Testing
```dockerfile
# Test in containerized environment
FROM golang:1.21
COPY . /app
WORKDIR /app
RUN ./scripts/run_tests.sh
```

## Contributing

When adding new tests:

1. **Unit Tests**: Add to appropriate `*_test.go` files
2. **Integration Tests**: Add to `test/integration_test.go`
3. **Benchmarks**: Add `Benchmark*` functions
4. **Update Documentation**: Update this README

### Test Naming Conventions
- `TestFunctionName_Scenario` for unit tests
- `TestEndToEndScenario` for integration tests
- `BenchmarkOperation` for performance tests

### Best Practices
- Use table-driven tests for multiple scenarios
- Include both positive and negative test cases
- Test concurrent operations with goroutines
- Mock external dependencies appropriately
- Clean up resources in test teardown 