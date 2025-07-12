#!/bin/bash

# Pickbox Unified Test Runner
# Comprehensive test execution with proper configuration and reporting

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# Configuration
TEST_ENVIRONMENT="development"
VERBOSE=false
COVERAGE=false
PARALLEL=false
CLEANUP_ONLY=false
GENERATE_REPORT=false
UPLOAD_RESULTS=false
DRY_RUN=false

# Test selection
RUN_ALL=true
RUN_UNIT=false
RUN_INTEGRATION=false
RUN_N_NODE=false
RUN_BENCHMARKS=false
RUN_STRESS=false
RUN_LEGACY=false

# Test categories
TEST_CATEGORIES=(
    "unit"
    "integration"
    "n-node"
    "benchmarks"
    "stress"
    "legacy"
)

# Results tracking
declare -A TEST_RESULTS
declare -A TEST_DURATIONS
declare -A TEST_COVERAGE
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Pickbox Unified Test Runner - Comprehensive test execution"
    echo ""
    echo "Test Categories:"
    echo "  --unit               Run unit tests only"
    echo "  --integration        Run integration tests only"
    echo "  --n-node             Run n-node tests only"
    echo "  --benchmarks         Run benchmark tests only"
    echo "  --stress             Run stress tests only"
    echo "  --legacy             Run legacy tests only"
    echo "  --all                Run all tests (default)"
    echo ""
    echo "Environment Options:"
    echo "  -e, --env ENV        Test environment (development, ci, short, performance, stress, local)"
    echo "  -v, --verbose        Enable verbose output"
    echo "  -c, --coverage       Generate coverage reports"
    echo "  -p, --parallel       Run tests in parallel where possible"
    echo "  --cleanup            Clean up test processes and data only"
    echo "  --dry-run            Show what would be executed without running"
    echo ""
    echo "Reporting Options:"
    echo "  --report             Generate comprehensive test report"
    echo "  --upload             Upload results to CI/CD system"
    echo "  --json               Output results in JSON format"
    echo "  --xml                Output results in XML format"
    echo ""
    echo "Examples:"
    echo "  $0                           # Run all tests in development environment"
    echo "  $0 --unit --coverage         # Run unit tests with coverage"
    echo "  $0 --n-node --env ci         # Run n-node tests in CI environment"
    echo "  $0 --stress --env stress     # Run stress tests in stress environment"
    echo "  $0 --parallel --report       # Run all tests in parallel and generate report"
    echo "  $0 --cleanup                 # Clean up test processes and data"
    echo ""
    echo "Environment Variables:"
    echo "  TEST_ENVIRONMENT     Override test environment"
    echo "  TEST_VERBOSE         Enable verbose output (true/false)"
    echo "  TEST_COVERAGE        Enable coverage (true/false)"
    echo "  TEST_PARALLEL        Enable parallel execution (true/false)"
    echo "  CI                   Automatically detected CI environment"
    echo ""
}

# Parse command line arguments
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
            RUN_ALL=false
            shift
            ;;
        --legacy)
            RUN_LEGACY=true
            RUN_ALL=false
            shift
            ;;
        --all)
            RUN_ALL=true
            shift
            ;;
        -e|--env)
            TEST_ENVIRONMENT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -p|--parallel)
            PARALLEL=true
            shift
            ;;
        --cleanup)
            CLEANUP_ONLY=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --report)
            GENERATE_REPORT=true
            shift
            ;;
        --upload)
            UPLOAD_RESULTS=true
            shift
            ;;
        --json)
            OUTPUT_FORMAT="json"
            shift
            ;;
        --xml)
            OUTPUT_FORMAT="xml"
            shift
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Apply environment variable overrides
if [ -n "$TEST_ENVIRONMENT" ]; then
    TEST_ENVIRONMENT="$TEST_ENVIRONMENT"
fi

if [ "$TEST_VERBOSE" = "true" ]; then
    VERBOSE=true
fi

if [ "$TEST_COVERAGE" = "true" ]; then
    COVERAGE=true
fi

if [ "$TEST_PARALLEL" = "true" ]; then
    PARALLEL=true
fi

if [ "$CI" = "true" ]; then
    TEST_ENVIRONMENT="ci"
    GENERATE_REPORT=true
fi

# Print header
echo -e "${CYAN}╔════════════════════════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║                              Pickbox Unified Test Runner                           ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Configuration summary
echo -e "${BLUE}Configuration:${NC}"
echo "  Environment: $TEST_ENVIRONMENT"
echo "  Verbose: $VERBOSE"
echo "  Coverage: $COVERAGE"
echo "  Parallel: $PARALLEL"
echo "  Generate Report: $GENERATE_REPORT"
echo "  Upload Results: $UPLOAD_RESULTS"
echo ""

# Test selection summary
echo -e "${BLUE}Test Selection:${NC}"
if [ "$RUN_ALL" = true ]; then
    echo "  Running all test categories"
else
    echo "  Selected categories:"
    [ "$RUN_UNIT" = true ] && echo "    - Unit tests"
    [ "$RUN_INTEGRATION" = true ] && echo "    - Integration tests"
    [ "$RUN_N_NODE" = true ] && echo "    - N-node tests"
    [ "$RUN_BENCHMARKS" = true ] && echo "    - Benchmark tests"
    [ "$RUN_STRESS" = true ] && echo "    - Stress tests"
    [ "$RUN_LEGACY" = true ] && echo "    - Legacy tests"
fi
echo ""

# Dry run mode
if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}🔍 DRY RUN MODE - No tests will be executed${NC}"
    echo ""
fi

# Load test configuration
if [ -f "test/test_config.go" ]; then
    echo -e "${BLUE}✅ Test configuration loaded${NC}"
else
    echo -e "${YELLOW}⚠️  Test configuration not found, using defaults${NC}"
fi

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up test processes...${NC}"
    
    # Kill test processes
    pkill -f "multi_replication" 2>/dev/null || true
    pkill -f "live_replication" 2>/dev/null || true
    pkill -f "go run.*replication" 2>/dev/null || true
    pkill -f "cluster_manager" 2>/dev/null || true
    
    # Clean up test data
    rm -rf /tmp/pickbox-*-test 2>/dev/null || true
    rm -rf data/node* 2>/dev/null || true
    
    # Clean up port conflicts
    local ports=($(seq 8000 8020) $(seq 9000 9020) $(seq 18000 18020) $(seq 19000 19020))
    for port in "${ports[@]}"; do
        if lsof -i :$port >/dev/null 2>&1; then
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

# Utility functions
print_section() {
    echo -e "\n${CYAN}╔════════════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║ $1${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════════════════════════════════════════════════╝${NC}"
}

print_subsection() {
    echo -e "\n${BLUE}┌─ $1 ─────────────────────────────────────────────────────────────────────────────────┐${NC}"
}

print_result() {
    local category="$1"
    local result="$2"
    local duration="$3"
    local coverage="$4"
    
    TEST_RESULTS["$category"]="$result"
    TEST_DURATIONS["$category"]="$duration"
    TEST_COVERAGE["$category"]="$coverage"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if [ "$result" = "PASSED" ]; then
        echo -e "${GREEN}✅ $category - PASSED${NC} ${duration:+(${duration}s)} ${coverage:+(${coverage}% coverage)}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    elif [ "$result" = "FAILED" ]; then
        echo -e "${RED}❌ $category - FAILED${NC} ${duration:+(${duration}s)} ${coverage:+(${coverage}% coverage)}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    elif [ "$result" = "SKIPPED" ]; then
        echo -e "${YELLOW}⏭️  $category - SKIPPED${NC} ${duration:+(${duration}s)}"
        SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
    fi
}

run_command() {
    local cmd="$1"
    local category="$2"
    
    if [ "$DRY_RUN" = true ]; then
        echo -e "${YELLOW}[DRY RUN] Would execute: $cmd${NC}"
        print_result "$category" "SKIPPED" "0" ""
        return
    fi
    
    local start_time=$(date +%s)
    
    if [ "$VERBOSE" = true ]; then
        echo -e "${CYAN}Executing: $cmd${NC}"
    fi
    
    # Execute command and capture result
    if eval "$cmd" > /tmp/test_output_${category}.log 2>&1; then
        local exit_code=0
    else
        local exit_code=1
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    # Extract coverage if available
    local coverage=""
    if [ "$COVERAGE" = true ] && [ -f "coverage/${category}.out" ]; then
        coverage=$(go tool cover -func=coverage/${category}.out 2>/dev/null | grep total | awk '{print $3}' | sed 's/%//')
    fi
    
    if [ $exit_code -eq 0 ]; then
        print_result "$category" "PASSED" "$duration" "$coverage"
    else
        print_result "$category" "FAILED" "$duration" "$coverage"
        if [ "$VERBOSE" = true ]; then
            echo -e "${RED}Error output:${NC}"
            cat /tmp/test_output_${category}.log
        fi
    fi
}

# Check dependencies
print_section "Dependency Check"
dependencies=("go" "curl" "lsof")
for dep in "${dependencies[@]}"; do
    if command -v "$dep" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ $dep${NC}"
    else
        echo -e "${RED}❌ $dep - not found${NC}"
        exit 1
    fi
done

# Build binaries
print_section "Build Phase"
echo "Building required binaries..."

if [ "$DRY_RUN" = false ]; then
    cd cmd/multi_replication
    go build -o ../../bin/multi_replication . || exit 1
    cd - > /dev/null
    
    cd cmd/live_replication
    go build -o ../../bin/live_replication . || exit 1
    cd - > /dev/null
    
    echo -e "${GREEN}✅ Binaries built successfully${NC}"
else
    echo -e "${YELLOW}[DRY RUN] Would build binaries${NC}"
fi

# Initialize coverage
if [ "$COVERAGE" = true ]; then
    mkdir -p coverage
    echo "mode: atomic" > coverage/combined.out
fi

# Run tests
print_section "Test Execution"

# Unit tests
if [ "$RUN_ALL" = true ] || [ "$RUN_UNIT" = true ]; then
    print_subsection "Unit Tests"
    
    if [ "$PARALLEL" = true ]; then
        run_command "./scripts/run_tests.sh --unit --coverage" "unit_tests" &
        unit_pid=$!
    else
        run_command "./scripts/run_tests.sh --unit --coverage" "unit_tests"
    fi
fi

# Integration tests
if [ "$RUN_ALL" = true ] || [ "$RUN_INTEGRATION" = true ]; then
    print_subsection "Integration Tests"
    
    if [ "$PARALLEL" = true ]; then
        run_command "./scripts/run_tests.sh --integration --coverage" "integration_tests" &
        integration_pid=$!
    else
        run_command "./scripts/run_tests.sh --integration --coverage" "integration_tests"
    fi
fi

# N-node tests
if [ "$RUN_ALL" = true ] || [ "$RUN_N_NODE" = true ]; then
    print_subsection "N-Node Tests"
    
    if [ "$PARALLEL" = true ]; then
        run_command "./scripts/run_tests.sh --n-node --coverage" "n_node_tests" &
        n_node_pid=$!
    else
        run_command "./scripts/run_tests.sh --n-node --coverage" "n_node_tests"
    fi
fi

# Benchmark tests
if [ "$RUN_ALL" = true ] || [ "$RUN_BENCHMARKS" = true ]; then
    print_subsection "Benchmark Tests"
    
    if [ "$PARALLEL" = true ]; then
        run_command "./scripts/run_tests.sh --benchmarks" "benchmark_tests" &
        benchmark_pid=$!
    else
        run_command "./scripts/run_tests.sh --benchmarks" "benchmark_tests"
    fi
fi

# Stress tests
if [ "$RUN_STRESS" = true ]; then
    print_subsection "Stress Tests"
    run_command "./scripts/run_tests.sh --stress" "stress_tests"
fi

# Legacy tests
if [ "$RUN_LEGACY" = true ]; then
    print_subsection "Legacy Tests"
    run_command "./scripts/run_tests.sh --integration" "legacy_tests"
fi

# Wait for parallel jobs
if [ "$PARALLEL" = true ]; then
    echo -e "${YELLOW}⏳ Waiting for parallel test execution to complete...${NC}"
    
    [ ! -z "$unit_pid" ] && wait $unit_pid
    [ ! -z "$integration_pid" ] && wait $integration_pid
    [ ! -z "$n_node_pid" ] && wait $n_node_pid
    [ ! -z "$benchmark_pid" ] && wait $benchmark_pid
fi

# Generate combined coverage report
if [ "$COVERAGE" = true ] && [ "$DRY_RUN" = false ]; then
    print_section "Coverage Report"
    
    # Combine coverage files
    echo "mode: atomic" > coverage/combined.out
    find coverage -name "*.out" -not -name "combined.out" -exec tail -n +2 {} \; >> coverage/combined.out 2>/dev/null || true
    
    # Generate HTML report
    if [ -f "coverage/combined.out" ]; then
        go tool cover -html=coverage/combined.out -o coverage/coverage.html
        
        # Get overall coverage percentage
        OVERALL_COVERAGE=$(go tool cover -func=coverage/combined.out | grep total | awk '{print $3}')
        echo -e "${GREEN}📊 Overall Coverage: $OVERALL_COVERAGE${NC}"
        echo "HTML report: coverage/coverage.html"
    fi
fi

# Generate test report
if [ "$GENERATE_REPORT" = true ]; then
    print_section "Test Report Generation"
    
    REPORT_FILE="test_report_$(date +%Y%m%d_%H%M%S).md"
    
    cat > "$REPORT_FILE" << EOF
# Pickbox Test Report

**Generated:** $(date)
**Environment:** $TEST_ENVIRONMENT
**Configuration:** Verbose=$VERBOSE, Coverage=$COVERAGE, Parallel=$PARALLEL

## Summary

- **Total Tests:** $TOTAL_TESTS
- **Passed:** $PASSED_TESTS
- **Failed:** $FAILED_TESTS
- **Skipped:** $SKIPPED_TESTS
- **Success Rate:** $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%

## Test Results

EOF
    
    for category in "${!TEST_RESULTS[@]}"; do
        result="${TEST_RESULTS[$category]}"
        duration="${TEST_DURATIONS[$category]}"
        coverage="${TEST_COVERAGE[$category]}"
        
        echo "### $category" >> "$REPORT_FILE"
        echo "- **Result:** $result" >> "$REPORT_FILE"
        echo "- **Duration:** ${duration}s" >> "$REPORT_FILE"
        [ ! -z "$coverage" ] && echo "- **Coverage:** ${coverage}%" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    done
    
    if [ "$COVERAGE" = true ] && [ ! -z "$OVERALL_COVERAGE" ]; then
        echo "## Overall Coverage" >> "$REPORT_FILE"
        echo "**Total Coverage:** $OVERALL_COVERAGE" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    fi
    
    echo -e "${GREEN}✅ Test report generated: $REPORT_FILE${NC}"
fi

# Final results
print_section "Final Results"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}🎉 All tests passed!${NC}"
    echo ""
    echo -e "${BLUE}Summary:${NC}"
    echo "  ✅ Total tests: $TOTAL_TESTS"
    echo "  ✅ Passed: $PASSED_TESTS"
    echo "  ✅ Failed: $FAILED_TESTS"
    echo "  ✅ Skipped: $SKIPPED_TESTS"
    
    if [ "$COVERAGE" = true ] && [ ! -z "$OVERALL_COVERAGE" ]; then
        echo "  ✅ Overall coverage: $OVERALL_COVERAGE"
    fi
    
    echo ""
    echo -e "${BLUE}🚀 Pickbox is ready for production!${NC}"
    exit 0
else
    echo -e "${RED}❌ Some tests failed${NC}"
    echo ""
    echo -e "${BLUE}Summary:${NC}"
    echo "  📊 Total tests: $TOTAL_TESTS"
    echo "  ✅ Passed: $PASSED_TESTS"
    echo "  ❌ Failed: $FAILED_TESTS"
    echo "  ⏭️  Skipped: $SKIPPED_TESTS"
    
    echo ""
    echo -e "${RED}Failed test categories:${NC}"
    for category in "${!TEST_RESULTS[@]}"; do
        if [ "${TEST_RESULTS[$category]}" = "FAILED" ]; then
            echo "  • $category (${TEST_DURATIONS[$category]}s)"
        fi
    done
    
    echo ""
    echo -e "${YELLOW}💡 Next steps:${NC}"
    echo "  • Check individual test logs in /tmp/test_output_*.log"
    echo "  • Run specific test categories with --unit, --integration, etc."
    echo "  • Use --verbose for detailed output"
    echo "  • Check the generated test report: $REPORT_FILE"
    
    exit 1
fi 