.PHONY: build test clean docker-build docker-test lint

# Build the application
build:
	go build -o bin/p2p-sync ./cmd/p2p-sync

# Run tests
test:
	P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

# Build Docker image
docker-build:
	docker build -t p2p-sync:latest -f Dockerfile .

# Run integration tests in Docker
docker-test:
	docker-compose -f docker-compose.yml up --abort-on-container-exit

# Run linters
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Run all checks
check: fmt lint test

