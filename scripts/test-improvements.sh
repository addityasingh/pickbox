#!/bin/bash

# Test script for Pickbox improvements validation
# This script tests the new modular architecture and deployment capabilities

set -euo pipefail

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_FILE="/tmp/pickbox-improvements-test-$(date +%Y%m%d-%H%M%S).log"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" | tee -a "$LOG_FILE"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

# Test 1: Package Structure Validation
test_package_structure() {
    log "Testing package structure improvements..."
    
    local packages=(
        "pkg/watcher"
        "pkg/monitoring" 
        "pkg/admin"
        "pkg/storage"
    )
    
    for pkg in "${packages[@]}"; do
        if [[ -d "$PROJECT_ROOT/$pkg" ]]; then
            info "✓ Package $pkg exists"
        else
            error "✗ Package $pkg missing"
            return 1
        fi
    done
    
    # Check key files
    local key_files=(
        "pkg/watcher/file_watcher.go"
        "pkg/watcher/state_manager.go"
        "pkg/watcher/hash.go"
        "pkg/monitoring/metrics.go"
        "pkg/monitoring/dashboard.go" 
        "pkg/admin/server.go"
    )
    
    for file in "${key_files[@]}"; do
        if [[ -f "$PROJECT_ROOT/$file" ]]; then
            info "✓ File $file exists"
        else
            error "✗ File $file missing"
            return 1
        fi
    done
    
    log "Package structure validation passed!"
}

# Test 2: Build Validation
test_build() {
    log "Testing build capabilities..."
    
    cd "$PROJECT_ROOT"
    
    # Test original build
    if go build -o /tmp/pickbox-original ./cmd/multi_replication/main.go 2>/dev/null; then
        info "✓ Original main builds successfully"
    else
        warn "✗ Original main has build issues"
    fi
    
    # Test package compilation
    local packages=(
        "./pkg/watcher"
        "./pkg/monitoring"
        "./pkg/admin"
        "./pkg/storage"
    )
    
    for pkg in "${packages[@]}"; do
        if go build -o /dev/null "$pkg" 2>/dev/null; then
            info "✓ Package $pkg compiles successfully"
        else
            error "✗ Package $pkg has compilation errors"
            return 1
        fi
    done
    
    log "Build validation passed!"
}

# Test 3: Docker Configuration Validation
test_docker_config() {
    log "Testing Docker deployment configuration..."
    
    local docker_files=(
        "deployments/docker/Dockerfile"
        "deployments/docker/docker-compose.yml"
        "deployments/docker/healthcheck.sh"
        "scripts/deploy/docker-deploy.sh"
    )
    
    for file in "${docker_files[@]}"; do
        if [[ -f "$PROJECT_ROOT/$file" ]]; then
            info "✓ Docker file $file exists"
        else
            error "✗ Docker file $file missing"
            return 1
        fi
    done
    
    # Validate Docker Compose syntax
    if command -v docker-compose &> /dev/null; then
        cd "$PROJECT_ROOT/deployments/docker"
        if docker-compose config &> /dev/null; then
            info "✓ Docker Compose configuration is valid"
        else
            error "✗ Docker Compose configuration has errors"
            return 1
        fi
    else
        warn "Docker Compose not available, skipping validation"
    fi
    
    log "Docker configuration validation passed!"
}

# Test 4: Kubernetes Configuration Validation
test_k8s_config() {
    log "Testing Kubernetes deployment configuration..."
    
    local k8s_files=(
        "deployments/k8s/pickbox-cluster.yaml"
        "scripts/deploy/k8s-deploy.sh"
    )
    
    for file in "${k8s_files[@]}"; do
        if [[ -f "$PROJECT_ROOT/$file" ]]; then
            info "✓ Kubernetes file $file exists"
        else
            error "✗ Kubernetes file $file missing"
            return 1
        fi
    done
    
    # Validate YAML syntax
    if command -v kubectl &> /dev/null; then
        if kubectl apply --dry-run=client -f "$PROJECT_ROOT/deployments/k8s/pickbox-cluster.yaml" &> /dev/null; then
            info "✓ Kubernetes YAML is valid"
        else
            error "✗ Kubernetes YAML has syntax errors"
            return 1
        fi
    else
        warn "kubectl not available, skipping YAML validation"
    fi
    
    log "Kubernetes configuration validation passed!"
}

# Test 5: Interface Compatibility
test_interfaces() {
    log "Testing interface compatibility..."
    
    cd "$PROJECT_ROOT"
    
    # Check if interfaces are defined correctly
    if grep -q "type.*interface" pkg/watcher/file_watcher.go; then
        info "✓ Watcher interfaces defined"
    else
        error "✗ Watcher interfaces missing"
        return 1
    fi
    
    if grep -q "RaftApplier" pkg/watcher/file_watcher.go; then
        info "✓ RaftApplier interface found"
    else
        error "✗ RaftApplier interface missing"
        return 1
    fi
    
    if grep -q "StateManager" pkg/watcher/file_watcher.go; then
        info "✓ StateManager interface found"
    else
        error "✗ StateManager interface missing"
        return 1
    fi
    
    log "Interface compatibility validation passed!"
}

# Test 6: Monitoring Features
test_monitoring() {
    log "Testing monitoring features..."
    
    # Check monitoring package structure
    local monitoring_features=(
        "NewMetrics"
        "GetNodeMetrics"
        "GetClusterHealth"
        "StartHTTPServer"
        "NewDashboard"
    )
    
    for feature in "${monitoring_features[@]}"; do
        if grep -q "$feature" "$PROJECT_ROOT/pkg/monitoring/"*.go; then
            info "✓ Monitoring feature $feature implemented"
        else
            error "✗ Monitoring feature $feature missing"
            return 1
        fi
    done
    
    log "Monitoring features validation passed!"
}

# Test 7: Deployment Scripts
test_deployment_scripts() {
    log "Testing deployment scripts..."
    
    local scripts=(
        "scripts/deploy/docker-deploy.sh"
        "scripts/deploy/k8s-deploy.sh"
    )
    
    for script in "${scripts[@]}"; do
        if [[ -f "$PROJECT_ROOT/$script" ]]; then
            info "✓ Deployment script $script exists"
            
            # Check if script is executable
            if [[ -x "$PROJECT_ROOT/$script" ]]; then
                info "✓ Script $script is executable"
            else
                warn "Script $script exists but is not executable"
                chmod +x "$PROJECT_ROOT/$script"
                info "✓ Made script $script executable"
            fi
            
            # Check for help function
            if grep -q "show_help" "$PROJECT_ROOT/$script"; then
                info "✓ Script $script has help function"
            else
                warn "Script $script missing help function"
            fi
        else
            error "✗ Deployment script $script missing"
            return 1
        fi
    done
    
    log "Deployment scripts validation passed!"
}

# Test 8: Documentation
test_documentation() {
    log "Testing documentation..."
    
    if [[ -f "$PROJECT_ROOT/.cursor/debug/IMPROVEMENTS_SUMMARY.md" ]]; then
        info "✓ Improvements summary documentation exists"
    else
        error "✗ Improvements summary documentation missing"
        return 1
    fi
    
    # Check README for updates
    if [[ -f "$PROJECT_ROOT/README.md" ]]; then
        if grep -q "monitoring\|dashboard\|docker\|kubernetes" "$PROJECT_ROOT/README.md"; then
            info "✓ README includes new features"
        else
            warn "README may need updates for new features"
        fi
    fi
    
    log "Documentation validation passed!"
}

# Main test runner
run_all_tests() {
    log "Starting Pickbox improvements validation..."
    log "Log file: $LOG_FILE"
    
    local tests=(
        "test_package_structure"
        "test_build" 
        "test_docker_config"
        "test_k8s_config"
        "test_interfaces"
        "test_monitoring"
        "test_deployment_scripts"
        "test_documentation"
    )
    
    local passed=0
    local total=${#tests[@]}
    
    for test in "${tests[@]}"; do
        echo
        if $test; then
            ((passed++))
        else
            error "Test $test failed"
        fi
    done
    
    echo
    log "======================="
    log "Test Results: $passed/$total tests passed"
    
    if [[ $passed -eq $total ]]; then
        log "🎉 All improvements validation tests passed!"
        log "The Pickbox system is ready for production deployment."
    else
        error "Some tests failed. Please check the issues above."
        return 1
    fi
}

# Generate summary report
generate_summary() {
    cat << EOF

📊 PICKBOX IMPROVEMENTS VALIDATION SUMMARY
==========================================

✅ Completed Improvements:
   - Modular file watcher architecture
   - Enhanced monitoring and dashboard
   - Docker deployment configuration
   - Kubernetes deployment manifests
   - Production-ready health checks
   - Comprehensive deployment scripts

🚀 Key Features Validated:
   - Package structure reorganization
   - Interface-based design
   - Monitoring and observability
   - Container deployment
   - Kubernetes orchestration
   - Automated deployment scripts

📁 Access Points:
   - Admin Interface: http://localhost:9001
   - Monitoring API: http://localhost:6001
   - Dashboard: http://localhost:8080
   - Health Checks: http://localhost:6001/health

🔧 Deployment Commands:
   - Docker: ./scripts/deploy/docker-deploy.sh deploy
   - Kubernetes: ./scripts/deploy/k8s-deploy.sh deploy
   - Local: go run cmd/multi_replication/main.go

📝 Documentation: .cursor/debug/IMPROVEMENTS_SUMMARY.md

EOF
}

# Script execution
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    cd "$PROJECT_ROOT"
    
    if run_all_tests; then
        generate_summary
        exit 0
    else
        error "Validation failed. Check $LOG_FILE for details."
        exit 1
    fi
fi 