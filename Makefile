# Makefile for Go IMAP Server Testing
#
# Quick Reference:
#   make test            - Run all tests
#   make test-noop       - Run NOOP command tests
#   make test-capability - Run CAPABILITY command tests
#   make test-logout     - Run LOGOUT command tests
#   make test-append     - Run APPEND command tests
#   make test-commands   - Run all command tests
#   make help            - Show all available targets

.PHONY: test test-capability test-noop test-commands test-verbose test-coverage test-race clean

# Run all tests
test:
	go test -tags=test ./...

# Run only capability-related tests
test-capability:
	go test -tags=test -v ./test/server -run "TestCapabilityCommand"

# Run only NOOP-related tests
test-noop:
	go test -tags=test -v ./test/server -run "TestNoopCommand"

# Run only LOGOUT-related tests
test-logout:
	go test -tags=test -v ./test/server -run "TestLogoutCommand"

# Run only APPEND-related tests
test-append:
	go test -tags=test -v ./test/server -run "TestAppendCommand"

# Run all command tests (CAPABILITY + NOOP + LOGOUT + APPEND)
test-commands:
	@echo "Running CAPABILITY tests..."
	@go test -tags=test -v ./test/server -run "TestCapabilityCommand"
	@echo "\nRunning NOOP tests..."
	@go test -tags=test -v ./test/server -run "TestNoopCommand"
	@echo "\nRunning LOGOUT tests..."
	@go test -tags=test -v ./test/server -run "TestLogoutCommand"
	@echo "\nRunning APPEND tests..."
	@go test -tags=test -v ./test/server -run "TestAppendCommand"

# Run tests with verbose output
test-verbose:
	go test -tags=test -v ./...

# Run tests with coverage
test-coverage:
	go test -tags=test -cover ./...
	go test -tags=test -coverprofile=coverage.out ./test/server
	go tool cover -html=coverage.out -o coverage.html
	@echo "\nCoverage report generated: coverage.html"

# Run tests with race detection
test-race:
	go test -tags=test -race ./...

# Run capability tests with detailed output (deprecated, use test-capability)
test-capability-detailed:
	go test -tags=test -v -run "TestCapabilityCommand" ./test/server

# Run benchmarks
bench:
	go test -tags=test -bench=. ./test/server

# Clean test artifacts
clean:
	rm -f coverage.out coverage.html

# Run specific test
test-single:
	@echo "Usage: make test-single TEST=TestCapabilityCommand_NonTLSConnection"
	@if [ -z "$(TEST)" ]; then \
		echo "Please specify TEST variable"; \
		exit 1; \
	fi
	go test -tags=test -v -run "$(TEST)" ./test/server

# Install test dependencies
deps:
	go mod tidy
	go mod download

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run ./...

# All quality checks
check: fmt lint test-race test-coverage

# Run tests in CI environment
ci: deps check

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Testing:"
	@echo "  test                 - Run all tests"
	@echo "  test-capability      - Run CAPABILITY command tests only"
	@echo "  test-noop            - Run NOOP command tests only"
	@echo "  test-logout          - Run LOGOUT command tests only"
	@echo "  test-append          - Run APPEND command tests only"
	@echo "  test-commands        - Run all command tests (CAPABILITY + NOOP + LOGOUT + APPEND)"
	@echo "  test-verbose         - Run tests with verbose output"
	@echo "  test-coverage        - Run tests with coverage report"
	@echo "  test-race            - Run tests with race detection"
	@echo "  bench                - Run benchmarks"
	@echo "  test-single TEST=... - Run a specific test"
	@echo ""
	@echo "Development:"
	@echo "  deps                 - Install dependencies"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Lint code"
	@echo "  clean                - Clean test artifacts"
	@echo ""
	@echo "CI/CD:"
	@echo "  check                - Run all quality checks"
	@echo "  ci                   - Run CI pipeline"
	@echo ""
	@echo "  help                 - Show this help"
