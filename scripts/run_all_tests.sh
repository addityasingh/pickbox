#!/bin/bash

# Comprehensive test runner for Pickbox distributed storage system
# This script runs all tests and generates a detailed test report

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_DIR="coverage"
COVERAGE_FILE="coverage.out"
COVERAGE_HTML="coverage.html"
TEST_REPORT="test_report.txt"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Pickbox Distributed Storage Test Suite${NC}"
echo -e "${BLUE}========================================${NC}"

# Create coverage directory
mkdir -p $COVERAGE_DIR

# Function to run tests for a specific package
run_package_tests() {
    local package=$1
    local package_name=$(basename $package)
    
    echo -e "${YELLOW}Running tests for $package_name...${NC}"
    
    # Run tests with coverage
    if go test -v -race -coverprofile=$COVERAGE_DIR/${package_name}.out ./$package 2>&1 | tee $COVERAGE_DIR/${package_name}_test.log; then
        echo -e "${GREEN}✓ $package_name tests passed${NC}"
        return 0
    else
        echo -e "${RED}✗ $package_name tests failed${NC}"
        return 1
    fi
}

# Function to run benchmarks
run_benchmarks() {
    local package=$1
    local package_name=$(basename $package)
    
    echo -e "${YELLOW}Running benchmarks for $package_name...${NC}"
    
    if go test -bench=. -benchmem ./$package > $COVERAGE_DIR/${package_name}_bench.log 2>&1; then
        echo -e "${GREEN}✓ $package_name benchmarks completed${NC}"
        return 0
    else
        echo -e "${RED}✗ $package_name benchmarks failed${NC}"
        return 1
    fi
}

# Initialize test report
echo "Pickbox Test Report - $(date)" > $TEST_REPORT
echo "=================================" >> $TEST_REPORT
echo "" >> $TEST_REPORT

# Test packages
TEST_PACKAGES=(
    "pkg/storage"
    "pkg/admin"
    "pkg/monitoring"
    "pkg/replication"
    "pkg/watcher"
    "cmd/multi_replication"
    "test"
)

FAILED_PACKAGES=()
PASSED_PACKAGES=()

# Run tests for each package
for package in "${TEST_PACKAGES[@]}"; do
    if [ -d "$package" ]; then
        if run_package_tests $package; then
            PASSED_PACKAGES+=($package)
        else
            FAILED_PACKAGES+=($package)
        fi
        echo ""
    else
        echo -e "${YELLOW}Warning: Package $package not found, skipping...${NC}"
    fi
done

# Run benchmarks
echo -e "${BLUE}Running benchmarks...${NC}"
for package in "${TEST_PACKAGES[@]}"; do
    if [ -d "$package" ]; then
        run_benchmarks $package
    fi
done

# Combine coverage files
echo -e "${BLUE}Generating coverage report...${NC}"
echo "mode: atomic" > $COVERAGE_FILE

for package in "${TEST_PACKAGES[@]}"; do
    package_name=$(basename $package)
    if [ -f "$COVERAGE_DIR/${package_name}.out" ]; then
        tail -n +2 $COVERAGE_DIR/${package_name}.out >> $COVERAGE_FILE
    fi
done

# Generate HTML coverage report
if command -v go &> /dev/null; then
    go tool cover -html=$COVERAGE_FILE -o $COVERAGE_HTML
    echo -e "${GREEN}Coverage report generated: $COVERAGE_HTML${NC}"
fi

# Calculate overall coverage
if command -v go &> /dev/null; then
    COVERAGE_PERCENT=$(go tool cover -func=$COVERAGE_FILE | grep total | awk '{print $3}')
    echo -e "${BLUE}Overall test coverage: $COVERAGE_PERCENT${NC}"
else
    COVERAGE_PERCENT="N/A"
fi

# Generate test summary
echo -e "${BLUE}Test Summary:${NC}"
echo -e "${GREEN}Passed packages: ${#PASSED_PACKAGES[@]}${NC}"
echo -e "${RED}Failed packages: ${#FAILED_PACKAGES[@]}${NC}"

if [ ${#FAILED_PACKAGES[@]} -gt 0 ]; then
    echo -e "${RED}Failed packages:${NC}"
    for package in "${FAILED_PACKAGES[@]}"; do
        echo -e "${RED}  - $package${NC}"
    done
fi

# Write detailed report
echo "Test Results Summary" >> $TEST_REPORT
echo "===================" >> $TEST_REPORT
echo "Total packages tested: ${#TEST_PACKAGES[@]}" >> $TEST_REPORT
echo "Passed: ${#PASSED_PACKAGES[@]}" >> $TEST_REPORT
echo "Failed: ${#FAILED_PACKAGES[@]}" >> $TEST_REPORT
echo "Coverage: $COVERAGE_PERCENT" >> $TEST_REPORT
echo "" >> $TEST_REPORT

echo "Passed Packages:" >> $TEST_REPORT
for package in "${PASSED_PACKAGES[@]}"; do
    echo "  ✓ $package" >> $TEST_REPORT
done

if [ ${#FAILED_PACKAGES[@]} -gt 0 ]; then
    echo "" >> $TEST_REPORT
    echo "Failed Packages:" >> $TEST_REPORT
    for package in "${FAILED_PACKAGES[@]}"; do
        echo "  ✗ $package" >> $TEST_REPORT
    done
fi

# Performance summary
echo "" >> $TEST_REPORT
echo "Benchmark Results:" >> $TEST_REPORT
echo "=================" >> $TEST_REPORT
for package in "${TEST_PACKAGES[@]}"; do
    package_name=$(basename $package)
    if [ -f "$COVERAGE_DIR/${package_name}_bench.log" ]; then
        echo "" >> $TEST_REPORT
        echo "Package: $package" >> $TEST_REPORT
        echo "$(cat $COVERAGE_DIR/${package_name}_bench.log | grep -E '^Benchmark' | head -5)" >> $TEST_REPORT
    fi
done

echo -e "${BLUE}Detailed test report saved to: $TEST_REPORT${NC}"

# Cleanup old coverage files
find $COVERAGE_DIR -name "*.out" -type f -delete

# Exit with error if any tests failed
if [ ${#FAILED_PACKAGES[@]} -gt 0 ]; then
    echo -e "${RED}Some tests failed. Check the logs for details.${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed successfully!${NC}"
    exit 0
fi
