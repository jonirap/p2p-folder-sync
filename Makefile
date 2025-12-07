.PHONY: build test test-unit test-integration test-system test-benchmark test-all test-coverage test-no-bench test-functional clean docker-build docker-run lint fmt check

# Build the application
build:
	go build -o bin/p2p-sync ./cmd/p2p-sync

# Run unit tests only (fast, no external dependencies)
test-unit:
	@echo "Running unit tests..."
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test -short -timeout=2m ./test/unit/...

# Run integration tests (may require external dependencies)
test-integration:
	@echo "Running integration tests..."
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test -timeout=5m -v ./test/integration/...

# Run system/E2E tests (requires Docker, tests full system)
test-system:
	@echo "Running system tests..."
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test -timeout=10m -v ./test/system/...

# Run benchmark/performance tests (requires Docker, long-running)
test-benchmark:
	@echo "Running benchmark and performance tests (this may take 10-15 minutes)..."
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test -tags=benchmark -timeout=30m -v ./test/benchmark/...

# Default test target (runs unit tests for quick feedback)
test: test-unit

# Run all tests (unit + integration + system + benchmark)
test-all: test-unit test-integration test-system test-benchmark

# Run tests with coverage (unit tests only)
test-coverage:
	@echo "Running unit tests with coverage..."
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test -short -coverprofile=coverage.out -timeout=2m ./test/unit/...
	go tool cover -html=coverage.out

# Run non-benchmark tests (unit + integration + system, excludes long-running benchmarks)
test-no-bench: test-unit test-integration test-system

# Run just integration and system tests (useful for CI)
test-functional: test-integration test-system

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

# Build Docker image
docker-build:
	docker build -t p2p-sync:latest -f Dockerfile .

# Run multiple peers in Docker
docker-run:
	docker-compose -f docker-compose.yml up --abort-on-container-exit

# Run linters
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Run all checks (unit tests only, not benchmarks)
check: fmt lint test

