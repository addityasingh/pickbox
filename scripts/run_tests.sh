#!/bin/bash

# Pickbox Test Runner
# Runs all unit tests, integration tests, and benchmarks

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo -e "${BLUE}🧪 Pickbox Test Suite${NC}"
echo "===================="

# Function to print section headers
print_section() {
    echo -e "\n${BLUE}📋 $1${NC}"
    echo "$(printf '=%.0s' {1..50})"
}

# Function to print test results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✅ $2 - PASSED${NC}"
    else
        echo -e "${RED}❌ $2 - FAILED${NC}"
        FAILED_TESTS+=("$2")
    fi
}

# Array to track failed tests
FAILED_TESTS=()

cd "$PROJECT_ROOT"

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

# Install test dependencies
echo "Installing test dependencies..."
go mod tidy
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require

print_section "Building Project"
echo "Building multi-replication binary..."
cd cmd/multi_replication
go build -o ../../bin/multi_replication .
cd "$PROJECT_ROOT"

if [ ! -f "bin/multi_replication" ]; then
    echo -e "${RED}Failed to build multi_replication binary${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Build successful${NC}"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up test processes...${NC}"
    pkill -f "multi_replication" 2>/dev/null || true
    pkill -f "go run.*multi_replication" 2>/dev/null || true
    rm -rf /tmp/pickbox-test-* /tmp/test-* 2>/dev/null || true
    sleep 1
}

# Set up cleanup trap
trap cleanup EXIT

# Unit Tests
print_section "Running Unit Tests"

echo "Testing storage package..."
cd pkg/storage
go test -v -cover . 2>&1
STORAGE_RESULT=$?
print_result $STORAGE_RESULT "Storage Package Tests"
cd "$PROJECT_ROOT"

echo "Testing multi-replication logic..."
cd cmd/multi_replication
go test -v -cover . 2>&1
MULTI_RESULT=$?
print_result $MULTI_RESULT "Multi-Replication Tests"
cd "$PROJECT_ROOT"

# Integration Tests
print_section "Running Integration Tests"

echo "Preparing for integration tests..."
cleanup

echo "Running integration test suite..."
cd test
# Use gtimeout on macOS if available, otherwise fallback
if command -v gtimeout &> /dev/null; then
    gtimeout 300 go test -v -timeout=240s . 2>&1
elif command -v timeout &> /dev/null; then
    timeout 300 go test -v -timeout=240s . 2>&1
else
    # No timeout available, run directly with go test timeout
    go test -v -timeout=240s . 2>&1
fi
INTEGRATION_RESULT=$?
print_result $INTEGRATION_RESULT "Integration Tests"
cd "$PROJECT_ROOT"

# Benchmark Tests
print_section "Running Benchmark Tests"

echo "Running storage benchmarks..."
cd pkg/storage
go test -bench=. -benchmem . 2>&1
BENCH_STORAGE_RESULT=$?
print_result $BENCH_STORAGE_RESULT "Storage Benchmarks"
cd "$PROJECT_ROOT"

echo "Running multi-replication benchmarks..."
cd cmd/multi_replication
go test -bench=. -benchmem . 2>&1
BENCH_MULTI_RESULT=$?
print_result $BENCH_MULTI_RESULT "Multi-Replication Benchmarks"
cd "$PROJECT_ROOT"

# Code Coverage
print_section "Generating Code Coverage Report"

echo "Generating coverage profile..."
go test -coverprofile=coverage.out ./...
COVERAGE_RESULT=$?

if [ $COVERAGE_RESULT -eq 0 ]; then
    echo "Generating HTML coverage report..."
    go tool cover -html=coverage.out -o coverage.html
    
    # Get coverage percentage
    COVERAGE_PCT=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    echo -e "${GREEN}📊 Total Coverage: $COVERAGE_PCT${NC}"
    echo "HTML report generated: coverage.html"
    
    print_result 0 "Code Coverage Report"
else
    print_result 1 "Code Coverage Report"
fi

# Performance Tests (Quick)
print_section "Running Performance Tests"

echo "Testing hash function performance..."
cd cmd/multi_replication
# Use timeout if available, otherwise fallback
if command -v gtimeout &> /dev/null; then
    gtimeout 30 go test -run=BenchmarkHashContent -bench=BenchmarkHashContent -count=3 . 2>&1
elif command -v timeout &> /dev/null; then
    timeout 30 go test -run=BenchmarkHashContent -bench=BenchmarkHashContent -count=3 . 2>&1
else
    go test -run=BenchmarkHashContent -bench=BenchmarkHashContent -count=3 . 2>&1
fi
PERF_RESULT=$?
print_result $PERF_RESULT "Performance Tests"
cd "$PROJECT_ROOT"

# Stress Tests (Optional - only if specifically requested)
if [ "$1" = "--stress" ]; then
    print_section "Running Stress Tests"
    
    echo "Running concurrent replication stress test..."
    cd test
    # Use timeout if available, otherwise fallback
    if command -v gtimeout &> /dev/null; then
        gtimeout 180 go test -run=TestConcurrentWrites -v . 2>&1
    elif command -v timeout &> /dev/null; then
        timeout 180 go test -run=TestConcurrentWrites -v . 2>&1
    else
        go test -run=TestConcurrentWrites -v . 2>&1
    fi
    STRESS_RESULT=$?
    print_result $STRESS_RESULT "Stress Tests"
    cd "$PROJECT_ROOT"
fi

# Lint and Code Quality (if available)
if command -v golangci-lint &> /dev/null; then
    print_section "Running Code Quality Checks"
    
    echo "Running golangci-lint..."
    golangci-lint run ./... 2>&1
    LINT_RESULT=$?
    print_result $LINT_RESULT "Code Quality Checks"
fi

# Final Results
print_section "Test Summary"

TOTAL_TESTS=7
PASSED_TESTS=$((TOTAL_TESTS - ${#FAILED_TESTS[@]}))

echo "Tests run: $TOTAL_TESTS"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}${#FAILED_TESTS[@]}${NC}"

if [ ${#FAILED_TESTS[@]} -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All tests passed!${NC}"
    
    # Show some fun stats
    echo -e "\n${BLUE}📈 Quick Stats:${NC}"
    echo "• Unit tests with mocking and concurrent testing"
    echo "• End-to-end integration tests with 3-node cluster"
    echo "• Benchmark tests for performance measurement"
    echo "• Code coverage report generated"
    
    if [ -f "coverage.out" ]; then
        echo "• Coverage: $(go tool cover -func=coverage.out | grep total | awk '{print $3}')"
    fi
    
    exit 0
else
    echo -e "\n${RED}❌ Some tests failed:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  • ${RED}$test${NC}"
    done
    
    echo -e "\n${YELLOW}💡 Debugging tips:${NC}"
    echo "• Check the logs above for specific error messages"
    echo "• Ensure no other processes are using ports 8000-8002, 9001-9003"
    echo "• Try running individual test suites with: go test -v ./pkg/storage"
    echo "• For integration tests: cd test && go test -v ."
    
    exit 1
fi 