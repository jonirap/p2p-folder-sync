#!/bin/bash
# Pre-commit hook for P2P Folder Sync
# Runs formatting, linting, and quick tests before commit

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Running pre-commit checks...${NC}\n"

# Function to print status
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
        exit 1
    fi
}

# 1. Check formatting
echo "Checking code formatting..."
if ! gofmt -l . | grep -q .; then
    print_status 0 "Code formatting"
else
    echo -e "${RED}The following files are not formatted:${NC}"
    gofmt -l .
    echo -e "${YELLOW}Run 'make fmt' to fix formatting${NC}"
    print_status 1 "Code formatting"
fi

# 2. Run linter
echo "Running golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint run --timeout=2m ./...; then
        print_status 0 "Linter checks"
    else
        print_status 1 "Linter checks"
    fi
else
    echo -e "${YELLOW}Warning: golangci-lint not installed, skipping linter${NC}"
    echo -e "${YELLOW}Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest${NC}"
fi

# 3. Run unit tests (fast subset)
echo "Running unit tests..."
if ./test/run_system_tests.sh --unit-only --fast &> /tmp/precommit-test.log; then
    print_status 0 "Unit tests"
else
    echo -e "${RED}Unit tests failed. See /tmp/precommit-test.log for details${NC}"
    tail -n 20 /tmp/precommit-test.log
    print_status 1 "Unit tests"
fi

# 4. Check for common issues
echo "Checking for common issues..."

# Check for TODO/FIXME in staged files
STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep "\.go$" || true)
if [ -n "$STAGED_GO_FILES" ]; then
    TODO_COUNT=$(echo "$STAGED_GO_FILES" | xargs grep -n "TODO\|FIXME" || true | wc -l)
    if [ "$TODO_COUNT" -gt 0 ]; then
        echo -e "${YELLOW}Warning: Found $TODO_COUNT TODO/FIXME comments in staged files${NC}"
        echo "$STAGED_GO_FILES" | xargs grep -n "TODO\|FIXME" || true
    fi
fi

# Check for debug print statements
DEBUG_COUNT=$(echo "$STAGED_GO_FILES" | xargs grep -n "fmt.Print\|log.Print" || true | grep -v "// " | wc -l)
if [ "$DEBUG_COUNT" -gt 0 ]; then
    echo -e "${YELLOW}Warning: Found $DEBUG_COUNT potential debug print statements${NC}"
    echo "$STAGED_GO_FILES" | xargs grep -n "fmt.Print\|log.Print" || true | grep -v "// "
fi

print_status 0 "Common issues check"

# 5. Build check
echo "Verifying build..."
if go build -o /tmp/p2p-sync-precommit ./cmd/p2p-sync > /dev/null 2>&1; then
    rm -f /tmp/p2p-sync-precommit
    print_status 0 "Build verification"
else
    print_status 1 "Build verification"
fi

echo -e "\n${GREEN}All pre-commit checks passed!${NC}\n"
exit 0
