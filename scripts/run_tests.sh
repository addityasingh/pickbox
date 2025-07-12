#!/bin/bash

# Pickbox Comprehensive Test Runner
# Runs all unit tests, integration tests, n-node tests, and benchmarks

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Test configuration
export PICKBOX_TEST_TIMEOUT=300s
export PICKBOX_REPLICATION_DELAY=5s
export PICKBOX_DEBUG=false

# Parse command line arguments
RUN_ALL=true
RUN_UNIT=false
RUN_INTEGRATION=false
RUN_N_NODE=false
RUN_BENCHMARKS=false
RUN_STRESS=false
VERBOSE=false
COVERAGE=false
PARALLEL=false
CLEANUP_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --unit)
            RUN_UNIT=true
            RUN_ALL=false
            shift
            ;;
        --integration)
            RUN_INTEGRATION=true
            RUN_ALL=false
            shift
            ;;
        --n-node)
            RUN_N_NODE=true
            RUN_ALL=false
            shift
            ;;
        --benchmarks)
            RUN_BENCHMARKS=true
            RUN_ALL=false
            shift
            ;;
        --stress)
            RUN_STRESS=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --coverage|-c)
            COVERAGE=true
            shift
            ;;
        --parallel|-p)
            PARALLEL=true
            shift
            ;;
        --cleanup)
            CLEANUP_ONLY=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --unit           Run only unit tests"
            echo "  --integration    Run only integration tests"
            echo "  --n-node         Run only n-node tests"
            echo "  --benchmarks     Run only benchmark tests"
            echo "  --stress         Run stress tests"
            echo "  --verbose        Enable verbose output"
            echo "  --coverage       Generate coverage reports"
            echo "  --parallel       Run tests in parallel"
            echo "  --cleanup        Clean up test processes and data"
            echo "  --help           Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}🧪 Pickbox Comprehensive Test Suite${NC}"
echo -e "${BLUE}====================================${NC}"

if [ "$VERBOSE" = true ]; then
    echo "Configuration:"
    echo "  - Test timeout: $PICKBOX_TEST_TIMEOUT"
    echo "  - Replication delay: $PICKBOX_REPLICATION_DELAY"
    echo "  - Debug mode: $PICKBOX_DEBUG"
    echo "  - Coverage: $COVERAGE"
    echo "  - Parallel: $PARALLEL"
    echo ""
fi

# Function to print section headers
print_section() {
    echo -e "\n${BLUE}📋 $1${NC}"
    echo "$(printf '=%.0s' {1..50})"
}

# Function to print test results
print_result() {
    local exit_code=$1
    local test_name="$2"
    local duration="$3"
    
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✅ $test_name - PASSED${NC} ${duration:+($duration)}"
    else
        echo -e "${RED}❌ $test_name - FAILED${NC} ${duration:+($duration)}"
        FAILED_TESTS+=("$test_name")
    fi
}

# Function to run command with timing
run_timed() {
    local start_time=$(date +%s)
    "$@"
    local exit_code=$?
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    echo "${duration}s"
    return $exit_code
}

# Array to track failed tests
FAILED_TESTS=()

cd "$PROJECT_ROOT"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up test processes...${NC}"
    
    # Kill any running processes
    pkill -f "multi_replication" 2>/dev/null || true
    pkill -f "live_replication" 2>/dev/null || true
    pkill -f "go run.*replication" 2>/dev/null || true
    pkill -f "cluster_manager" 2>/dev/null || true
    
    # Remove test data
    rm -rf /tmp/pickbox-test-* /tmp/test-* 2>/dev/null || true
    rm -rf data/node* 2>/dev/null || true
    
    # Clean up any leftover processes by port
    local ports=(8000 8001 8002 8003 8004 8005 8006 8007 8008 8009 8010 9001 9002 9003 9004 9005 9006 9007 9008 9009 9010)
    for port in "${ports[@]}"; do
        if lsof -i :$port >/dev/null 2>&1; then
            echo "Cleaning up process on port $port"
            lsof -ti :$port | xargs kill -9 2>/dev/null || true
        fi
    done
    
    sleep 1
    echo -e "${GREEN}✅ Cleanup completed${NC}"
}

# Set up cleanup trap
trap cleanup EXIT

# Handle cleanup-only mode
if [ "$CLEANUP_ONLY" = true ]; then
    cleanup
    exit 0
fi

# Ensure dependencies are available
print_section "Checking Dependencies"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not installed or not in PATH${NC}"
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo -e "${RED}curl is not installed (needed for integration tests)${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Dependencies available${NC}"

# Install test dependencies
echo "Installing test dependencies..."
go mod tidy
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require

# Build binaries
print_section "Building Project"
echo "Building multi-replication binary..."
cd cmd/multi_replication
go build -o ../../bin/multi_replication .
cd "$PROJECT_ROOT"

cd cmd/live_replication
go build -o ../../bin/live_replication .
cd "$PROJECT_ROOT"

if [ ! -f "bin/multi_replication" ]; then
    echo -e "${RED}Failed to build multi_replication binary${NC}"
    exit 1
fi

if [ ! -f "bin/live_replication" ]; then
    echo -e "${RED}Failed to build live_replication binary${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Build successful${NC}"

# Make scripts executable
chmod +x scripts/*.sh scripts/tests/*.sh scripts/cluster_manager.sh 2>/dev/null || true

# Initialize coverage
if [ "$COVERAGE" = true ]; then
    mkdir -p coverage
    echo "mode: atomic" > coverage/combined.out
fi

# Unit Tests
if [ "$RUN_ALL" = true ] || [ "$RUN_UNIT" = true ]; then
    print_section "Running Unit Tests"
    
    echo "Testing storage package..."
    cd pkg/storage
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -race -coverprofile=../../coverage/storage.out -covermode=atomic .)
        tail -n +2 ../../coverage/storage.out >> ../../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -race .)
    fi
    STORAGE_RESULT=$?
    print_result $STORAGE_RESULT "Storage Package Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Testing admin package..."
    cd pkg/admin
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -race -coverprofile=../../coverage/admin.out -covermode=atomic .)
        tail -n +2 ../../coverage/admin.out >> ../../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -race .)
    fi
    ADMIN_RESULT=$?
    print_result $ADMIN_RESULT "Admin Package Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Testing monitoring package..."
    cd pkg/monitoring
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -race -coverprofile=../../coverage/monitoring.out -covermode=atomic .)
        tail -n +2 ../../coverage/monitoring.out >> ../../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -race .)
    fi
    MONITORING_RESULT=$?
    print_result $MONITORING_RESULT "Monitoring Package Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Testing multi-replication logic..."
    cd cmd/multi_replication
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -race -coverprofile=../../coverage/multi_replication.out -covermode=atomic .)
        tail -n +2 ../../coverage/multi_replication.out >> ../../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -race .)
    fi
    MULTI_RESULT=$?
    print_result $MULTI_RESULT "Multi-Replication Tests" "$duration"
    cd "$PROJECT_ROOT"
fi

# N-Node Tests
if [ "$RUN_ALL" = true ] || [ "$RUN_N_NODE" = true ]; then
    print_section "Running N-Node Tests"
    
    echo "Preparing for N-node tests..."
    cleanup
    
    echo "Running N-node unit tests..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -race -short -run "TestNNode|TestAdminAddress|TestPortCalculation|TestClusterSize|TestNodeID|TestDataDirectory|TestConcurrentNode|TestBootstrap" -coverprofile=../coverage/n_node_unit.out -covermode=atomic .)
        tail -n +2 ../coverage/n_node_unit.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -race -short -run "TestNNode|TestAdminAddress|TestPortCalculation|TestClusterSize|TestNodeID|TestDataDirectory|TestConcurrentNode|TestBootstrap" .)
    fi
    N_NODE_UNIT_RESULT=$?
    print_result $N_NODE_UNIT_RESULT "N-Node Unit Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running N-node integration tests..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=300s -run "TestThreeNodeCluster|TestFiveNodeCluster|TestSevenNodeCluster|TestLargeFileReplication|TestNestedDirectoryReplication|TestFileModificationReplication|TestClusterScaling|TestClusterWithCustomPorts|TestClusterStartStopCycles" -coverprofile=../coverage/n_node_integration.out -covermode=atomic .)
        tail -n +2 ../coverage/n_node_integration.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=300s -run "TestThreeNodeCluster|TestFiveNodeCluster|TestSevenNodeCluster|TestLargeFileReplication|TestNestedDirectoryReplication|TestFileModificationReplication|TestClusterScaling|TestClusterWithCustomPorts|TestClusterStartStopCycles" .)
    fi
    N_NODE_INTEGRATION_RESULT=$?
    print_result $N_NODE_INTEGRATION_RESULT "N-Node Integration Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running N-node replication tests..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=300s -run "TestMultiDirectionalReplication|TestConcurrentMultiNodeWrites|TestFileModificationPropagation|TestFileDeletionPropagation|TestReplicationNestedDirectories|TestReplicationLargeFiles|TestRapidFileOperations|TestReplicationWithDelays|TestBinaryFileReplication|TestFilePermissionReplication" -coverprofile=../coverage/n_node_replication.out -covermode=atomic .)
        tail -n +2 ../coverage/n_node_replication.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=300s -run "TestMultiDirectionalReplication|TestConcurrentMultiNodeWrites|TestFileModificationPropagation|TestFileDeletionPropagation|TestReplicationNestedDirectories|TestReplicationLargeFiles|TestRapidFileOperations|TestReplicationWithDelays|TestBinaryFileReplication|TestFilePermissionReplication" .)
    fi
    N_NODE_REPLICATION_RESULT=$?
    print_result $N_NODE_REPLICATION_RESULT "N-Node Replication Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running N-node failure tests..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=300s -run "TestSingleNodeFailureRecovery|TestMultipleNodeFailures|TestLeaderFailureAndElection|TestClusterPartitioning|TestRapidFailureRecoveryCycles|TestGracefulVsUngracefulShutdown|TestDiskSpaceExhaustion|TestConcurrentFailuresDuringLoad" -coverprofile=../coverage/n_node_failure.out -covermode=atomic .)
        tail -n +2 ../coverage/n_node_failure.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=300s -run "TestSingleNodeFailureRecovery|TestMultipleNodeFailures|TestLeaderFailureAndElection|TestClusterPartitioning|TestRapidFailureRecoveryCycles|TestGracefulVsUngracefulShutdown|TestDiskSpaceExhaustion|TestConcurrentFailuresDuringLoad" .)
    fi
    N_NODE_FAILURE_RESULT=$?
    print_result $N_NODE_FAILURE_RESULT "N-Node Failure Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running N-node performance tests..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=600s -run "TestThroughputScaling|TestConcurrentWritePerformance|TestFileSizePerformance|TestClusterStartupPerformance|TestMemoryUsagePatterns|TestReplicationConsistencyUnderLoad|TestLatencyDistribution|TestPeakLoadResourceUsage|TestEdgeCasePerformance" -coverprofile=../coverage/n_node_performance.out -covermode=atomic .)
        tail -n +2 ../coverage/n_node_performance.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=600s -run "TestThroughputScaling|TestConcurrentWritePerformance|TestFileSizePerformance|TestClusterStartupPerformance|TestMemoryUsagePatterns|TestReplicationConsistencyUnderLoad|TestLatencyDistribution|TestPeakLoadResourceUsage|TestEdgeCasePerformance" .)
    fi
    N_NODE_PERFORMANCE_RESULT=$?
    print_result $N_NODE_PERFORMANCE_RESULT "N-Node Performance Tests" "$duration"
    cd "$PROJECT_ROOT"
fi

# Legacy Integration Tests
if [ "$RUN_ALL" = true ] || [ "$RUN_INTEGRATION" = true ]; then
    print_section "Running Legacy Integration Tests"
    
    echo "Preparing for integration tests..."
    cleanup
    
    echo "Running legacy integration test suite..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=240s -run "TestBasicReplication|TestFileModification|TestFileDeletion|TestConcurrentWrites|TestNestedDirectories|TestLargeFiles|TestNodeFailureRecovery" -coverprofile=../coverage/legacy_integration.out -covermode=atomic .)
        tail -n +2 ../coverage/legacy_integration.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=240s -run "TestBasicReplication|TestFileModification|TestFileDeletion|TestConcurrentWrites|TestNestedDirectories|TestLargeFiles|TestNodeFailureRecovery" .)
    fi
    LEGACY_INTEGRATION_RESULT=$?
    print_result $LEGACY_INTEGRATION_RESULT "Legacy Integration Tests" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running enhanced integration test suite..."
    cd test
    if [ "$COVERAGE" = true ]; then
        duration=$(run_timed go test -v -timeout=240s -run "TestFullSystemIntegration|TestAdminIntegration|TestFileReplicationWorkflow|TestConcurrentFileOperations|TestErrorHandlingScenarios|TestSystemResourceMonitoring" -coverprofile=../coverage/enhanced_integration.out -covermode=atomic .)
        tail -n +2 ../coverage/enhanced_integration.out >> ../coverage/combined.out 2>/dev/null || true
    else
        duration=$(run_timed go test -v -timeout=240s -run "TestFullSystemIntegration|TestAdminIntegration|TestFileReplicationWorkflow|TestConcurrentFileOperations|TestErrorHandlingScenarios|TestSystemResourceMonitoring" .)
    fi
    ENHANCED_INTEGRATION_RESULT=$?
    print_result $ENHANCED_INTEGRATION_RESULT "Enhanced Integration Tests" "$duration"
    cd "$PROJECT_ROOT"
fi

# Benchmark Tests
if [ "$RUN_ALL" = true ] || [ "$RUN_BENCHMARKS" = true ]; then
    print_section "Running Benchmark Tests"
    
    echo "Running storage benchmarks..."
    cd pkg/storage
    duration=$(run_timed go test -bench=. -benchmem -run=^$ .)
    BENCH_STORAGE_RESULT=$?
    print_result $BENCH_STORAGE_RESULT "Storage Benchmarks" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running multi-replication benchmarks..."
    cd cmd/multi_replication
    duration=$(run_timed go test -bench=. -benchmem -run=^$ .)
    BENCH_MULTI_RESULT=$?
    print_result $BENCH_MULTI_RESULT "Multi-Replication Benchmarks" "$duration"
    cd "$PROJECT_ROOT"
    
    echo "Running N-node benchmarks..."
    cd test
    duration=$(run_timed go test -bench=. -benchmem -run=^$ .)
    BENCH_N_NODE_RESULT=$?
    print_result $BENCH_N_NODE_RESULT "N-Node Benchmarks" "$duration"
    cd "$PROJECT_ROOT"
fi

# Stress Tests (Optional)
if [ "$RUN_STRESS" = true ]; then
    print_section "Running Stress Tests"
    
    echo "Running concurrent replication stress test..."
    cd test
    duration=$(run_timed go test -run=TestConcurrentWrites -v -timeout=300s .)
    STRESS_RESULT=$?
    print_result $STRESS_RESULT "Stress Tests" "$duration"
    cd "$PROJECT_ROOT"
fi

# Code Coverage Report
if [ "$COVERAGE" = true ]; then
    print_section "Generating Code Coverage Report"
    
    echo "Generating coverage profile..."
    
    if [ -f "coverage/combined.out" ]; then
        echo "Generating HTML coverage report..."
        go tool cover -html=coverage/combined.out -o coverage/coverage.html
        
        # Get coverage percentage
        COVERAGE_PCT=$(go tool cover -func=coverage/combined.out | grep total | awk '{print $3}')
        echo -e "${GREEN}📊 Total Coverage: $COVERAGE_PCT${NC}"
        echo "HTML report generated: coverage/coverage.html"
        
        print_result 0 "Code Coverage Report"
    else
        print_result 1 "Code Coverage Report"
    fi
fi

# Final Results
print_section "Test Summary"

# Count results
TOTAL_TESTS=0
PASSED_TESTS=0

if [ "$RUN_ALL" = true ] || [ "$RUN_UNIT" = true ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 4))
    [ "${STORAGE_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${ADMIN_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${MONITORING_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${MULTI_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_N_NODE" = true ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 5))
    [ "${N_NODE_UNIT_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${N_NODE_INTEGRATION_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${N_NODE_REPLICATION_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${N_NODE_FAILURE_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${N_NODE_PERFORMANCE_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_INTEGRATION" = true ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 2))
    [ "${LEGACY_INTEGRATION_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${ENHANCED_INTEGRATION_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_BENCHMARKS" = true ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 3))
    [ "${BENCH_STORAGE_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${BENCH_MULTI_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
    [ "${BENCH_N_NODE_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
fi

if [ "$RUN_STRESS" = true ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    [ "${STRESS_RESULT:-0}" -eq 0 ] && PASSED_TESTS=$((PASSED_TESTS + 1))
fi

echo "Tests run: $TOTAL_TESTS"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$((TOTAL_TESTS - PASSED_TESTS))${NC}"

if [ ${#FAILED_TESTS[@]} -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All tests passed!${NC}"
    
    # Show some fun stats
    echo -e "\n${BLUE}📈 Quick Stats:${NC}"
    echo "• Unit tests with comprehensive coverage"
    echo "• N-node tests for 3, 5, 7, 10+ node clusters"
    echo "• Integration tests for end-to-end validation"
    echo "• Benchmark tests for performance measurement"
    echo "• Failure tests for resilience validation"
    
    if [ "$COVERAGE" = true ] && [ -f "coverage/combined.out" ]; then
        echo "• Coverage: $(go tool cover -func=coverage/combined.out | grep total | awk '{print $3}')"
    fi
    
    exit 0
else
    echo -e "\n${RED}❌ Some tests failed:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  • ${RED}$test${NC}"
    done
    
    echo -e "\n${YELLOW}💡 Debugging tips:${NC}"
    echo "• Check the logs above for specific error messages"
    echo "• Ensure no other processes are using ports 8000-8010, 9001-9010"
    echo "• Try running individual test suites with specific flags"
    echo "• For integration tests: check if binaries are built correctly"
    echo "• Use --verbose flag for detailed output"
    echo "• Use --cleanup flag to clean up before running tests"
    
    exit 1
fi 