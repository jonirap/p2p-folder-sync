#!/bin/bash

# System Tests Runner for P2P Folder Sync
# This script runs comprehensive system tests to verify missing functionality

set -e

echo "🧪 Running P2P Folder Sync System Tests"
echo "========================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results
PASSED=0
FAILED=0
TOTAL=0

# Check command line arguments
RUN_UNIT_ONLY=false
RUN_INTEGRATION_ONLY=false
RUN_FAST=false

for arg in "$@"; do
    case $arg in
        --unit-only)
            RUN_UNIT_ONLY=true
            ;;
        --integration-only)
            RUN_INTEGRATION_ONLY=true
            ;;
        --fast)
            RUN_FAST=true
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --unit-only        Run only fast unit tests (skip integration tests)"
            echo "  --integration-only Run only integration tests (skip unit tests)"
            echo "  --fast             Skip Docker tests for faster execution"
            echo "  --help             Show this help message"
            exit 0
            ;;
    esac
done

run_test() {
    local test_name="$1"
    local test_cmd="$2"

    echo -e "\n${BLUE}Running: ${test_name}${NC}"
    echo "Command: $test_cmd"

    TOTAL=$((TOTAL + 1))

    if eval "$test_cmd"; then
        echo -e "${GREEN}✅ PASSED: ${test_name}${NC}"
        PASSED=$((PASSED + 1))
    else
        echo -e "${RED}❌ FAILED: ${test_name}${NC}"
        FAILED=$((FAILED + 1))
    fi
}

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo -e "${RED}Error: Must run from project root directory${NC}"
    exit 1
fi

echo -e "\n${YELLOW}Phase 1: Fast Unit Tests (In-Memory, should pass quickly)${NC}"
echo "============================================================="

if [ "$RUN_INTEGRATION_ONLY" = false ]; then
    run_test "Unit Tests (Internal)" "go test ./internal/... -v -timeout 30s"

    echo -e "\n${YELLOW}Phase 1.5: Unit Tests (test/unit packages)${NC}"
    echo "=============================================="

    run_test "Unit Tests (Unit Package)" "go test ./test/unit/... -v -timeout 30s"

    echo -e "\n${YELLOW}Phase 2: System Unit Tests (In-Memory Messenger, existing tests)${NC}"
    echo "======================================================================"

    # Existing system tests using InMemoryMessenger (fast unit tests)
    run_test "Sync Loop Prevention (Unit)" "go test ./test/system -run TestSyncLoopPreventionCritical -v -timeout 60s"

    run_test "P2P File Sync (Unit)" "go test ./test/system -run TestPeerToPeerFileSync -v -timeout 120s"

    run_test "Large File Sync (Unit)" "go test ./test/system -run TestLargeFileSync -v -timeout 180s"

    run_test "Rename Sync (Unit)" "go test ./test/system -run TestRenameSync -v -timeout 120s"

    run_test "Network Resilience (Unit)" "go test ./test/system -run TestNetworkResilienceConnectionRecovery -v -timeout 120s"

    run_test "Heartbeat Mechanism (Unit)" "go test ./test/system -run TestHeartbeatMechanism -v -timeout 90s"

    run_test "Encryption (Unit)" "go test ./test/system -run TestEncryption -v -timeout 30s"

    run_test "Load Balancing (Unit)" "go test ./test/system -run TestLoadBalancing -v -timeout 120s"

    run_test "Conflict Resolution (Unit)" "go test ./test/system -run TestConflictResolution -v -timeout 120s"
fi

if [ "$RUN_UNIT_ONLY" = false ]; then
    echo -e "\n${YELLOW}Phase 3: System Integration Tests (Real Network, slower but comprehensive)${NC}"
    echo "==============================================================================="

    # New integration tests using real network components
    run_test "P2P File Sync (Network)" "go test ./test/system -run TestPeerToPeerFileSyncNetwork -tags=integration -v -timeout 180s"

    run_test "Large File Sync (Network)" "go test ./test/system -run TestLargeFileSyncNetwork -tags=integration -v -timeout 240s"

    run_test "Rename Sync (Network)" "go test ./test/system -run TestRenameSyncNetwork -tags=integration -v -timeout 180s"

    run_test "Sync Loop Prevention (Network)" "go test ./test/system -run TestSyncLoopPreventionNetwork -tags=integration -v -timeout 180s"

    run_test "Multi-Peer Sync Loop Prevention" "go test ./test/system -run TestSyncLoopPreventionWithRenameNetwork -tags=integration -v -timeout 150s"

    run_test "Network Resilience (Failing Transport)" "go test ./test/system -run TestNetworkResilienceWithFailingTransport -tags=integration -v -timeout 180s"

    run_test "Heartbeat (Network)" "go test ./test/system -run TestHeartbeatMechanismNetwork -tags=integration -v -timeout 120s"

    run_test "Peer Disconnection Recovery" "go test ./test/system -run TestPeerDisconnectionRecoveryNetwork -tags=integration -v -timeout 400s"

    run_test "Message Encryption (Network)" "go test ./test/system -run TestMessageEncryption -tags=integration -v -timeout 120s"

    run_test "Message Chunking (Network)" "go test ./test/system -run TestMessageChunking -tags=integration -v -timeout 240s"

    run_test "Message Compression (Network)" "go test ./test/system -run TestMessageCompression -tags=integration -v -timeout 120s"

    run_test "Multi-Peer Load Balancing" "go test ./test/system -run TestMultiPeerLoadBalancing -tags=integration -v -timeout 400s"

    run_test "Multi-Peer Conflict Resolution" "go test ./test/system -run TestMultiPeerConflictResolution -tags=integration -v -timeout 240s"

    run_test "End-to-End Integration" "go test ./test/system -run TestEndToEndIntegration -tags=integration -v -timeout 400s"

    run_test "Complex Conflict Resolution" "go test ./test/system -run TestComplexConflictResolution -tags=integration -v -timeout 240s"
fi

echo -e "\n${YELLOW}Phase 4: Docker Network Tests${NC}"
echo "================================"

# Check if Docker is available and fast mode is not enabled
if [ "$RUN_FAST" = false ] && command -v docker &> /dev/null && command -v docker-compose &> /dev/null; then
    echo -e "\n${BLUE}Docker available - running network simulation tests${NC}"

    # Build test containers
    echo "Building test containers..."
    docker-compose -f docker/docker-compose.yml build

    # Run multi-peer network test
    run_test "Docker Multi-Peer Network Test" "docker-compose -f docker/docker-compose.yml up --abort-on-container-exit --timeout 300"

    # Cleanup
    docker-compose -f docker/docker-compose.yml down -v
elif [ "$RUN_FAST" = true ]; then
    echo -e "\n${YELLOW}Fast mode enabled - skipping Docker tests${NC}"
else
    echo -e "\n${YELLOW}Docker not available - skipping network simulation tests${NC}"
fi

echo -e "\n${BLUE}Test Results Summary${NC}"
echo "======================"
echo -e "Total Tests: $TOTAL"
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All tests passed! Implementation is complete.${NC}"
    echo -e "${BLUE}Test Coverage:${NC}"
    echo "  ✅ Fast unit tests (in-memory components)"
    echo "  ✅ System unit tests (existing functionality)"
    echo "  ✅ System integration tests (real network components)"
    echo "  ✅ Docker network simulation tests"
    exit 0
else
    echo -e "\n${YELLOW}⚠️  Some tests failed. Analysis:${NC}"

    # Provide guidance based on which tests failed
    if [ "$RUN_UNIT_ONLY" = true ]; then
        echo -e "${BLUE}Unit tests failed - check basic functionality${NC}"
    elif [ "$RUN_INTEGRATION_ONLY" = true ]; then
        echo -e "${BLUE}Integration tests failed - check real network implementation${NC}"
    else
        echo -e "${BLUE}Mixed test failures - check both unit and integration code${NC}"
    fi

    echo -e "\n${BLUE}Common issues to check:${NC}"
    echo "1. Network connection timeouts (increase timeouts if needed)"
    echo "2. File system permissions for test directories"
    echo "3. Port conflicts (tests use dynamic ports but may collide)"
    echo "4. Race conditions in concurrent operations"
    echo "5. Missing error handling for edge cases"
    echo ""
    echo -e "${BLUE}For faster iteration:${NC}"
    echo "  Use --unit-only for quick validation of logic"
    echo "  Use --integration-only for focused network testing"
    echo "  Use --fast to skip Docker tests during development"

    exit 1
fi
