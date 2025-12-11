#!/bin/bash

# Integration Test Infrastructure Verification Script
# This script tests that all components of the integration test infrastructure are working

set -e

echo "==================================="
echo "Integration Test Infrastructure Test"
echo "==================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    if [ "$status" = "OK" ]; then
        echo -e "${GREEN}[OK]${NC} $message"
    elif [ "$status" = "WARN" ]; then
        echo -e "${YELLOW}[WARN]${NC} $message"
    elif [ "$status" = "FAIL" ]; then
        echo -e "${RED}[FAIL]${NC} $message"
    else
        echo -e "${NC}[INFO]${NC} $message"
    fi
}

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INTEGRATION_DIR="$SCRIPT_DIR"

print_status "INFO" "Testing integration infrastructure in: $INTEGRATION_DIR"

# Test 1: Check directory structure
echo ""
echo "1. Checking directory structure..."

required_dirs=(
    "fixtures"
    "helpers"
    "db"
    "server"
    "delivery"
    "e2e"
)

all_dirs_exist=true
for dir in "${required_dirs[@]}"; do
    if [ -d "$INTEGRATION_DIR/$dir" ]; then
        print_status "OK" "Directory exists: $dir"
    else
        print_status "FAIL" "Directory missing: $dir"
        all_dirs_exist=false
    fi
done

# Test 2: Check fixture files
echo ""
echo "2. Checking fixture files..."

required_fixtures=(
    "fixtures/simple-email.eml"
    "fixtures/multipart-email.eml"
    "fixtures/email-with-attachment.eml"
    "fixtures/html-email.eml"
    "fixtures/multi-recipient-email.eml"
    "fixtures/unicode-email.eml"
    "fixtures/large-email.eml"
    "fixtures/test-users.json"
    "fixtures/test-config.yaml"
    "fixtures/mailbox-structures.json"
)

all_fixtures_exist=true
for fixture in "${required_fixtures[@]}"; do
    if [ -f "$INTEGRATION_DIR/$fixture" ]; then
        size=$(wc -c < "$INTEGRATION_DIR/$fixture")
        print_status "OK" "Fixture exists: $fixture (${size} bytes)"
    else
        print_status "FAIL" "Fixture missing: $fixture"
        all_fixtures_exist=false
    fi
done

# Test 3: Check helper files
echo ""
echo "3. Checking helper files..."

required_helpers=(
    "helpers/fixtures.go"
    "helpers/database.go"
    "helpers/server.go"
    "helpers/docker.go"
)

all_helpers_exist=true
for helper in "${required_helpers[@]}"; do
    if [ -f "$INTEGRATION_DIR/$helper" ]; then
        lines=$(wc -l < "$INTEGRATION_DIR/$helper")
        print_status "OK" "Helper exists: $helper (${lines} lines)"
    else
        print_status "FAIL" "Helper missing: $helper"
        all_helpers_exist=false
    fi
done

# Test 4: Check Docker Compose file
echo ""
echo "4. Checking Docker Compose configuration..."

if [ -f "$INTEGRATION_DIR/docker-compose.yml" ]; then
    print_status "OK" "Docker Compose file exists"

    # Validate Docker Compose syntax
    if command -v docker-compose >/dev/null 2>&1; then
        cd "$INTEGRATION_DIR"
        if docker-compose config >/dev/null 2>&1; then
            print_status "OK" "Docker Compose syntax is valid"
        else
            print_status "WARN" "Docker Compose syntax validation failed"
        fi
    else
        print_status "WARN" "docker-compose not available, skipping syntax check"
    fi
else
    print_status "FAIL" "Docker Compose file missing"
fi

# Test 5: Check Go module and build
echo ""
echo "5. Checking Go build..."

cd "$SCRIPT_DIR/../.."  # Go to project root

if [ -f "go.mod" ]; then
    print_status "OK" "go.mod exists"

    if go mod tidy; then
        print_status "OK" "go mod tidy successful"
    else
        print_status "WARN" "go mod tidy failed"
    fi

    if go build ./test/integration/... 2>/dev/null; then
        print_status "OK" "Integration test code compiles"
    else
        print_status "WARN" "Integration test code has compilation issues"
    fi
else
    print_status "FAIL" "go.mod missing from project root"
fi

# Test 6: Check if tests can run (syntax check)
echo ""
echo "6. Checking test syntax..."

cd "$INTEGRATION_DIR"

if go test -c ./helpers 2>/dev/null; then
    print_status "OK" "Helper test code compiles"
    rm -f helpers.test
else
    print_status "WARN" "Helper test code has issues"
fi

# Test existing test files
test_dirs=("db" "server" "delivery" "e2e")
for dir in "${test_dirs[@]}"; do
    if ls "$dir"/*_test.go >/dev/null 2>&1; then
        if go test -c "./$dir" 2>/dev/null; then
            print_status "OK" "Test code in $dir compiles"
            rm -f "${dir}.test"
        else
            print_status "WARN" "Test code in $dir has compilation issues"
        fi
    else
        print_status "INFO" "No test files found in $dir (expected for new infrastructure)"
    fi
done

# Test 7: Docker environment test (if Docker is available)
echo ""
echo "7. Checking Docker environment..."

if command -v docker >/dev/null 2>&1 && command -v docker-compose >/dev/null 2>&1; then
    print_status "OK" "Docker and Docker Compose are available"

    # Try to validate the compose file
    cd "$INTEGRATION_DIR"
    if docker-compose config >/dev/null 2>&1; then
        print_status "OK" "Docker Compose configuration is valid"

        # Quick test - try to pull/build images (but don't start)
        if docker-compose pull >/dev/null 2>&1 || docker-compose build >/dev/null 2>&1; then
            print_status "OK" "Docker images can be prepared"
        else
            print_status "WARN" "Docker image preparation may have issues"
        fi
    else
        print_status "WARN" "Docker Compose configuration has issues"
    fi
else
    print_status "WARN" "Docker not available - container tests will be skipped"
fi

# Summary
echo ""
echo "==================================="
echo "Summary"
echo "==================================="

if $all_dirs_exist && $all_fixtures_exist && $all_helpers_exist; then
    print_status "OK" "Integration test infrastructure is ready!"
    echo ""
    echo "You can now:"
    echo "  - Run Go integration tests: go test ./test/integration/..."
    echo "  - Start Docker environment: cd test/integration && docker-compose up"
    echo "  - Run specific test suites in each subdirectory"
    exit 0
else
    print_status "FAIL" "Integration test infrastructure has missing components"
    echo ""
    echo "Please fix the missing components before running integration tests."
    exit 1
fi
