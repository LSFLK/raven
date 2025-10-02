# Makefile for Go IMAP Server Testing

.PHONY: test test-capability test-verbose test-coverage test-race clean

# Run all tests
test:
	go test ./...

# Run only capability-related tests
test-capability:
	go test -v ./test/server -run "TestCapability"

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -cover ./...
	go test -coverprofile=coverage.out ./test/server
	go tool cover -html=coverage.out -o coverage.html

# Run tests with race detection
test-race:
	go test -race ./...

# Run capability tests with detailed output
test-capability-detailed:
	go test -v -run "TestCapability" ./test/server

# Run benchmarks
bench:
	go test -bench=. ./test/server

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
	go test -v -run "$(TEST)" ./test/server

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
	@echo "  test                 - Run all tests"
	@echo "  test-capability      - Run capability tests only"
	@echo "  test-verbose         - Run tests with verbose output"
	@echo "  test-coverage        - Run tests with coverage report"
	@echo "  test-race            - Run tests with race detection"
	@echo "  bench                - Run benchmarks"
	@echo "  test-single TEST=... - Run a specific test"
	@echo "  clean                - Clean test artifacts"
	@echo "  deps                 - Install dependencies"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Lint code"
	@echo "  check                - Run all quality checks"
	@echo "  ci                   - Run CI pipeline"
	@echo "  help                 - Show this help"
