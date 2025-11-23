# Makefile for Go IMAP Server Testing
#
# Quick Reference:
#   make test              - Run all tests
#   make test-db           - Run all database tests
#   make test-db-init      - Run database initialization tests
#   make test-db-domain    - Run domain management tests
#   make test-db-user      - Run user management tests
#   make test-db-mailbox   - Run mailbox operations tests
#   make test-db-message   - Run message management tests
#   make test-db-blob      - Run blob storage tests
#   make test-db-role      - Run role mailbox tests
#   make test-db-manager   - Run DB manager tests
#   make test-conf         - Run configuration tests
#   make test-utils        - Run server utilities tests
#   make test-response     - Run server response tests
#   make test-storage      - Run delivery storage tests
#   make test-noop         - Run NOOP command tests
#   make test-check        - Run CHECK command tests
#   make test-close        - Run CLOSE command tests
#   make test-expunge      - Run EXPUNGE command tests
#   make test-capability   - Run CAPABILITY command tests
#   make test-logout       - Run LOGOUT command tests
#   make test-append       - Run APPEND command tests
#   make test-authenticate - Run AUTHENTICATE command tests
#   make test-login        - Run LOGIN command tests
#   make test-starttls     - Run STARTTLS command tests
#   make test-select       - Run SELECT command tests
#   make test-examine      - Run EXAMINE command tests
#   make test-create       - Run CREATE command tests
#   make test-list         - Run LIST command tests
#   make test-list-extended - Run LIST extended tests (RFC3501, wildcards, etc.)
#   make test-delete       - Run DELETE command tests
#   make test-rename       - Run RENAME command tests
#   make test-subscribe    - Run SUBSCRIBE command tests
#   make test-unsubscribe  - Run UNSUBSCRIBE command tests
#   make test-lsub         - Run LSUB command tests
#   make test-status      - Run STATUS command tests
#   make test-rename       - Run RENAME command tests
#   make test-search       - Run SEARCH command tests
#   make test-fetch        - Run FETCH command tests
#   make test-store        - Run STORE command tests
#   make test-commands     - Run all command tests
#   make help              - Show all available targets

.PHONY: test test-db test-db-init test-db-domain test-db-user test-db-mailbox test-db-message test-db-blob test-db-role test-db-manager test-capability test-noop test-check test-close test-expunge test-authenticate test-login test-starttls test-select test-examine test-create test-list test-list-extended test-delete test-status test-search test-fetch test-store test-copy test-uid test-commands test-delivery test-sasl test-conf test-utils test-response test-storage test-verbose test-coverage test-race clean

# Build delivery service
build-delivery:
	go build -o bin/raven-delivery ./cmd/delivery

# Run delivery service
run-delivery:
	go run ./cmd/delivery

# Test delivery service
test-delivery:
	go test -tags=test -v ./internal/delivery/...

# Test SASL authentication service
test-sasl:
	go test -tags=test -v ./internal/sasl

# Test configuration
test-conf:
	go test -v ./internal/conf/...

# Test server utilities
test-utils:
	go test -v ./internal/server/utils/...

# Test server response formatting
test-response:
	go test -v ./internal/server/response/...

# Test delivery storage
test-storage:
	go test -v ./internal/delivery/storage/...

# ============================================================================
# Database Tests
# ============================================================================

# Run all database tests
test-db:
	go test -v ./internal/db/...

# Run database tests with coverage
test-db-coverage:
	go test -v -cover ./internal/db/...

# Run database initialization tests
test-db-init:
	go test -v ./internal/db -run "TestInitDB|TestNewDBManager"

# Run domain management tests
test-db-domain:
	go test -v ./internal/db -run "TestCreateDomain|TestGetDomainByName|TestGetOrCreateDomain"

# Run user management tests
test-db-user:
	go test -v ./internal/db -run "TestCreateUser|TestGetUser|TestUserExists"

# Run mailbox operations tests
test-db-mailbox:
	go test -v ./internal/db -run "TestCreateMailbox|TestGetMailbox|TestDeleteMailbox|TestRenameMailbox|TestMailboxExists"

# Run message management tests
test-db-message:
	go test -v ./internal/db -run "TestCreateMessage|TestAddMessageToMailbox|TestGetMessages|TestMessageFlags"

# Run blob storage tests
test-db-blob:
	go test -v ./internal/db -run "TestStoreBlob|TestGetBlob|TestDecrementBlobReference"

# Run role mailbox tests
test-db-role:
	go test -v ./internal/db -run "TestRoleMailbox|TestAssignUser|TestGetRoleMailboxAssigned"

# Run subscription tests
test-db-subscription:
	go test -v ./internal/db -run "TestSubscribe|TestUnsubscribe|TestIsMailboxSubscribed"

# Run delivery and outbound queue tests
test-db-delivery:
	go test -v ./internal/db -run "TestRecordDelivery|TestQueueOutbound|TestGetPendingOutbound|TestUpdateOutboundStatus|TestRetryOutbound"

# Run DB manager tests
test-db-manager:
	go test -v ./internal/db -run "TestGetSharedDB|TestGetUserDB|TestGetRoleMailboxDB|TestClose|TestCaching"

# Run all database tests with verbose output and coverage
test-db-all:
	@echo "Running all database tests with coverage..."
	@echo "\n=== Database Initialization Tests ==="
	@go test -v ./internal/db -run "TestInitDB|TestNewDBManager"
	@echo "\n=== Domain Management Tests ==="
	@go test -v ./internal/db -run "TestCreateDomain|TestGetDomainByName|TestGetOrCreateDomain"
	@echo "\n=== User Management Tests ==="
	@go test -v ./internal/db -run "TestCreateUser|TestGetUser|TestUserExists"
	@echo "\n=== Mailbox Operations Tests ==="
	@go test -v ./internal/db -run "TestCreateMailbox|TestGetMailbox|TestDeleteMailbox|TestRenameMailbox|TestMailboxExists"
	@echo "\n=== Message Management Tests ==="
	@go test -v ./internal/db -run "TestCreateMessage|TestAddMessageToMailbox|TestGetMessages|TestMessageFlags"
	@echo "\n=== Blob Storage Tests ==="
	@go test -v ./internal/db -run "TestStoreBlob|TestGetBlob|TestDecrementBlobReference"
	@echo "\n=== Role Mailbox Tests ==="
	@go test -v ./internal/db -run "TestRoleMailbox|TestAssignUser|TestGetRoleMailboxAssigned"
	@echo "\n=== Subscription Tests ==="
	@go test -v ./internal/db -run "TestSubscribe|TestUnsubscribe|TestIsMailboxSubscribed"
	@echo "\n=== Delivery & Outbound Queue Tests ==="
	@go test -v ./internal/db -run "TestRecordDelivery|TestQueueOutbound|TestGetPendingOutbound|TestUpdateOutboundStatus|TestRetryOutbound"
	@echo "\n=== DB Manager Tests ==="
	@go test -v ./internal/db -run "TestGetSharedDB|TestGetUserDB|TestGetRoleMailboxDB|TestClose|TestCaching"
	@echo "\n=== Full Database Test Suite with Coverage ==="
	@go test -v -cover ./internal/db/...

# Build SASL authentication service
build-sasl:
	go build -o bin/raven-sasl ./cmd/sasl

# Run SASL authentication service
run-sasl:
	go run ./cmd/sasl

# Build all services
build-all: build-delivery build-sasl

# Run all tests
test:
	go test -tags=test ./...

# Run only capability-related tests
test-capability:
	go test -tags=test -v ./internal/server -run "TestCapabilityCommand"

# Run only NOOP-related tests
test-noop:
	go test -tags=test -v ./internal/server -run "TestNoopCommand"

# Run only CHECK-related tests
test-check:
	go test -tags=test -v ./internal/server -run "TestCheckCommand"

# Run only CLOSE-related tests
test-close:
	go test -tags=test -v ./internal/server -run "TestCloseCommand"

# Run only EXPUNGE-related tests
test-expunge:
	go test -tags=test -v ./internal/server -run "TestExpungeCommand"

# Run only LOGOUT-related tests
test-logout:
	go test -tags=test -v ./internal/server -run "TestLogoutCommand"

# Run only APPEND-related tests
test-append:
	go test -tags=test -v ./internal/server -run "TestAppendCommand"

# Run only AUTHENTICATE-related tests
test-authenticate:
	go test -tags=test -v ./internal/server -run "TestAuthenticate"

# Run AUTHENTICATE benchmarks
bench-authenticate:
	go test -tags=test -bench=BenchmarkAuthenticate -benchmem ./internal/server

# Run only LOGIN-related tests
test-login:
	go test -tags=test -v ./internal/server -run "TestLoginCommand"

# Run only STARTTLS-related tests
test-starttls:
	go test -tags=test -v ./internal/server -run "TestStartTLS"

# Run only SELECT-related tests
test-select:
	go test -tags=test -v ./internal/server -run "TestSelectCommand"

# Run only EXAMINE-related tests
test-examine:
	go test -tags=test -v ./internal/server -run "TestExamineCommand"

# Run only CREATE-related tests
test-create:
	go test -tags=test -v ./internal/server -run "TestCreateCommand"

# Run only LIST-related tests
test-list:
	go test -tags=test -v ./internal/server -run "TestListCommand"

# Run LIST extended tests (RFC3501, wildcards, hierarchy, etc.)
test-list-extended:
	go test -tags=test -v ./internal/server -run "TestListCommand.*RFC3501|TestListCommand.*Wildcard|TestListCommand.*Hierarchy|TestListCommand.*Reference|TestListCommand.*Error|TestListCommand.*Special"

# Run only DELETE-related tests
test-delete:
	go test -tags=test -v ./internal/server -run "TestDeleteCommand"

# Run only SUBSCRIBE-related tests
test-subscribe:
	go test -tags=test -v ./internal/server -run "TestSubscribeCommand"

# Run only UNSUBSCRIBE-related tests
test-unsubscribe:
	go test -tags=test -v ./internal/server -run "TestUnsubscribeCommand"

# Run only LSUB-related tests
test-lsub:
	go test -tags=test -v ./internal/server -run "TestLsubCommand"

# Run only STATUS-related tests
test-status:
	go test -tags=test -v ./internal/server -run "TestStatusCommand"

# Run only RENAME-related tests
test-rename:
	go test -tags=test -v ./internal/server -run "TestRenameCommand"

# Run only SEARCH-related tests
test-search:
	go test -tags=test -v ./internal/server -run "TestSearchCommand"

# Run only FETCH-related tests
test-fetch:
	go test -tags=test -v ./internal/server -run "TestFetchCommand"

# Run only STORE-related tests
test-store:
	go test -tags=test -v ./internal/server -run "TestStoreCommand"

# Run only COPY-related tests
test-copy:
	go test -tags=test -v ./internal/server -run "TestCopyCommand"

# Run only UID-related tests
test-uid:
	go test -tags=test -v ./internal/server -run "TestUID"

# Run all command tests (CAPABILITY + NOOP + CHECK + CLOSE + EXPUNGE + LOGOUT + APPEND + AUTHENTICATE + LOGIN + STARTTLS + SELECT + EXAMINE + CREATE + LIST + LIST-EXTENDED + DELETE + SUBSCRIBE + UNSUBSCRIBE + LSUB + STATUS + RENAME + SEARCH + FETCH + STORE + COPY + UID)
test-commands:
	@echo "Running CAPABILITY tests..."
	@go test -tags=test -v ./internal/server -run "TestCapabilityCommand"
	@echo "\nRunning NOOP tests..."
	@go test -tags=test -v ./internal/server -run "TestNoopCommand"
	@echo "\nRunning CHECK tests..."
	@go test -tags=test -v ./internal/server -run "TestCheckCommand"
	@echo "\nRunning CLOSE tests..."
	@go test -tags=test -v ./internal/server -run "TestCloseCommand"
	@echo "\nRunning EXPUNGE tests..."
	@go test -tags=test -v ./internal/server -run "TestExpungeCommand"
	@echo "\nRunning LOGOUT tests..."
	@go test -tags=test -v ./internal/server -run "TestLogoutCommand"
	@echo "\nRunning APPEND tests..."
	@go test -tags=test -v ./internal/server -run "TestAppendCommand"
	@echo "\nRunning AUTHENTICATE tests..."
	@go test -tags=test -v ./internal/server -run "TestAuthenticate"
	@echo "\nRunning LOGIN tests..."
	@go test -tags=test -v ./internal/server -run "TestLoginCommand"
	@echo "\nRunning STARTTLS tests..."
	@go test -tags=test -v ./internal/server -run "TestStartTLS"
	@echo "\nRunning SELECT tests..."
	@go test -tags=test -v ./internal/server -run "TestSelectCommand"
	@echo "\nRunning EXAMINE tests..."
	@go test -tags=test -v ./internal/server -run "TestExamineCommand"
	@echo "\nRunning CREATE tests..."
	@go test -tags=test -v ./internal/server -run "TestCreateCommand"
	@echo "\nRunning LIST tests..."
	@go test -tags=test -v ./internal/server -run "TestListCommand"
	@echo "\nRunning LIST extended tests..."
	@go test -tags=test -v ./internal/server -run "TestListCommand.*RFC3501|TestListCommand.*Wildcard|TestListCommand.*Hierarchy|TestListCommand.*Reference|TestListCommand.*Error|TestListCommand.*Special"
	@echo "\nRunning DELETE tests..."
	@go test -tags=test -v ./internal/server -run "TestDeleteCommand"
	@echo "\nRunning SUBSCRIBE tests..."
	@go test -tags=test -v ./internal/server -run "TestSubscribeCommand"
	@echo "\nRunning UNSUBSCRIBE tests..."
	@go test -tags=test -v ./internal/server -run "TestUnsubscribeCommand"
	@echo "\nRunning LSUB tests..."
	@go test -tags=test -v ./internal/server -run "TestLsubCommand"
	@echo "\nRunning STATUS tests..."
	@go test -tags=test -v ./internal/server -run "TestStatusCommand"
	@echo "\nRunning RENAME tests..."
	@go test -tags=test -v ./internal/server -run "TestRenameCommand"
	@echo "\nRunning SEARCH tests..."
	@go test -tags=test -v ./internal/server -run "TestSearchCommand"
	@echo "\nRunning FETCH tests..."
	@go test -tags=test -v ./internal/server -run "TestFetchCommand"
	@echo "\nRunning STORE tests..."
	@go test -tags=test -v ./internal/server -run "TestStoreCommand"
	@echo "\nRunning COPY tests..."
	@go test -tags=test -v ./internal/server -run "TestCopyCommand"
	@echo "\nRunning UID tests..."
	@go test -tags=test -v ./internal/server -run "TestUID"

# Run tests with verbose output
test-verbose:
	go test -tags=test -v ./...

# Run tests with coverage
test-coverage:
	go test -tags=test -cover ./...
	go test -tags=test -coverprofile=coverage.out ./internal/server
	go tool cover -html=coverage.out -o coverage.html
	@echo "\nCoverage report generated: coverage.html"

# Run tests with race detection
test-race:
	go test -tags=test -race ./...

# Run capability tests with detailed output (deprecated, use test-capability)
test-capability-detailed:
	go test -tags=test -v -run "TestCapabilityCommand" ./internal/server

# Run benchmarks
bench:
	go test -tags=test -bench=. ./internal/server

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
	go test -tags=test -v -run "$(TEST)" ./internal/server

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
	@echo "Build & Run:"
	@echo "  build-delivery         - Build delivery service binary"
	@echo "  build-sasl             - Build SASL authentication service binary"
	@echo "  build-all              - Build all services"
	@echo "  run-delivery           - Run delivery service"
	@echo "  run-sasl               - Run SASL authentication service"
	@echo ""
	@echo "Testing:"
	@echo "  test                   - Run all tests"
	@echo "  test-db                - Run all database tests"
	@echo "  test-db-coverage       - Run database tests with coverage"
	@echo "  test-db-init           - Run database initialization tests"
	@echo "  test-db-domain         - Run domain management tests"
	@echo "  test-db-user           - Run user management tests"
	@echo "  test-db-mailbox        - Run mailbox operations tests"
	@echo "  test-db-message        - Run message management tests"
	@echo "  test-db-blob           - Run blob storage tests"
	@echo "  test-db-role           - Run role mailbox tests"
	@echo "  test-db-subscription   - Run subscription tests"
	@echo "  test-db-delivery       - Run delivery & outbound queue tests"
	@echo "  test-db-manager        - Run DB manager tests"
	@echo "  test-db-all            - Run all database tests with detailed output"
	@echo "  test-delivery          - Run delivery service tests"
	@echo "  test-sasl              - Run SASL authentication service tests"
	@echo "  test-conf              - Run configuration tests"
	@echo "  test-utils             - Run server utilities tests"
	@echo "  test-response          - Run server response tests"
	@echo "  test-storage           - Run delivery storage tests"
	@echo "  test-capability        - Run CAPABILITY command tests only"
	@echo "  test-noop              - Run NOOP command tests only"
	@echo "  test-check             - Run CHECK command tests only"
	@echo "  test-close             - Run CLOSE command tests only"
	@echo "  test-expunge           - Run EXPUNGE command tests only"
	@echo "  test-logout            - Run LOGOUT command tests only"
	@echo "  test-append            - Run APPEND command tests only"
	@echo "  test-authenticate      - Run AUTHENTICATE command tests only"
	@echo "  test-login             - Run LOGIN command tests only"
	@echo "  test-starttls          - Run STARTTLS command tests only"
	@echo "  test-select            - Run SELECT command tests only"
	@echo "  test-examine           - Run EXAMINE command tests only"
	@echo "  test-create            - Run CREATE command tests only"
	@echo "  test-list              - Run LIST command tests only"
	@echo "  test-list-extended     - Run LIST extended tests (RFC3501, wildcards, hierarchy, etc.)"
	@echo "  test-delete            - Run DELETE command tests only"
	@echo "  test-subscribe         - Run SUBSCRIBE command tests only"
	@echo "  test-unsubscribe       - Run UNSUBSCRIBE command tests only"
	@echo "  test-lsub              - Run LSUB command tests only"
	@echo "  test-status            - Run STATUS command tests only"
	@echo "  test-rename            - Run RENAME command tests only"
	@echo "  test-search            - Run SEARCH command tests only"
	@echo "  test-fetch             - Run FETCH command tests only"
	@echo "  test-store             - Run STORE command tests only"
	@echo "  test-copy              - Run COPY command tests only"
	@echo "  test-uid               - Run UID command tests only"
	@echo "  test-commands          - Run all command tests (CAPABILITY + NOOP + CHECK + CLOSE + EXPUNGE + LOGOUT + APPEND + AUTHENTICATE + LOGIN + STARTTLS + SELECT + EXAMINE + CREATE + LIST + LIST-EXTENDED + DELETE + SUBSCRIBE + UNSUBSCRIBE + LSUB + STATUS + RENAME + SEARCH + FETCH + STORE + COPY + UID)"
	@echo "  test-verbose           - Run tests with verbose output"
	@echo "  test-coverage          - Run tests with coverage report"
	@echo "  test-race              - Run tests with race detection"
	@echo "  bench                  - Run all benchmarks"
	@echo "  bench-authenticate     - Run AUTHENTICATE benchmarks"
	@echo "  test-single TEST=...   - Run a specific test"
	@echo ""
	@echo "Development:"
	@echo "  deps                   - Install dependencies"
	@echo "  fmt                    - Format code"
	@echo "  lint                   - Lint code"
	@echo "  clean                  - Clean test artifacts"
	@echo ""
	@echo "CI/CD:"
	@echo "  check                  - Run all quality checks"
	@echo "  ci                     - Run CI pipeline"
	@echo ""
	@echo "  help                   - Show this help"
