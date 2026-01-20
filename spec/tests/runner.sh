#!/bin/bash
# Spec compliance test runner for try (Go version)
# Usage: ./runner.sh /path/to/try

set +e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_DIR="$(dirname "$SCRIPT_DIR")"

# Check arguments
if [ $# -lt 1 ]; then
    echo "Usage: $0 /path/to/try"
    echo ""
    echo "Run spec compliance tests against the try binary."
    exit 1
fi

TRY_CMD="$1"

# Convert to absolute path if relative
if [[ "$TRY_CMD" != /* ]]; then
    TRY_CMD="$(cd "$(dirname "$TRY_CMD")" && pwd)/$(basename "$TRY_CMD")"
fi

# Verify binary exists and is executable
if [ ! -x "$TRY_CMD" ]; then
    echo -e "${RED}Error: '$TRY_CMD' is not executable or does not exist${NC}"
    exit 1
fi

# Export for test scripts
export TRY_CMD
export SPEC_DIR

# Helper function to run try
try_run() {
    $TRY_CMD "$@" 2>&1
}

# Create test environment
export TEST_ROOT=$(mktemp -d)
export TEST_TRIES="$TEST_ROOT/tries"
mkdir -p "$TEST_TRIES"

# Create test directories with different mtimes
mkdir -p "$TEST_TRIES/2025-11-01-alpha"
mkdir -p "$TEST_TRIES/2025-11-15-beta"
mkdir -p "$TEST_TRIES/2025-11-20-gamma"
mkdir -p "$TEST_TRIES/2025-11-25-project-with-long-name"
mkdir -p "$TEST_TRIES/no-date-prefix"

# Set mtimes (oldest first)
touch -t 202511010000 "$TEST_TRIES/2025-11-01-alpha"
touch -t 202511150000 "$TEST_TRIES/2025-11-15-beta"
touch -t 202511200000 "$TEST_TRIES/2025-11-20-gamma"
touch -t 202511250000 "$TEST_TRIES/2025-11-25-project-with-long-name"
touch "$TEST_TRIES/no-date-prefix"

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test utilities
pass() {
    echo -en "${GREEN}.${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    TESTS_RUN=$((TESTS_RUN + 1))
}

fail() {
    echo -e "\n${RED}FAIL${NC}: $1"
    if [ -n "$2" ]; then
        echo "  Expected: $2"
    fi
    if [ -n "$3" ]; then
        echo -e "\n  Command output:\n\n$3\n"
    fi
    if [ -n "$4" ]; then
        echo -e "  ${YELLOW}Spec: $4${NC}"
    fi
    TESTS_FAILED=$((TESTS_FAILED + 1))
    TESTS_RUN=$((TESTS_RUN + 1))
}

section() {
    echo -en "\n${YELLOW}$1${NC} "
}

export -f pass fail section try_run

# Cleanup on exit
cleanup() {
    rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

# Header
echo "Testing: $TRY_CMD"
echo "Test env: $TEST_TRIES"
echo

# Run all test_*.sh files in order
for test_file in "$SCRIPT_DIR"/test_*.sh; do
    if [ -f "$test_file" ]; then
        set +e
        source "$test_file"
    fi
done

# Summary
echo
echo
echo "═══════════════════════════════════"
echo "Results: $TESTS_PASSED/$TESTS_RUN passed"

EXIT_CODE=0

if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}$TESTS_FAILED tests failed${NC}"
    EXIT_CODE=1
fi

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}All tests passed${NC}"
fi

exit $EXIT_CODE
