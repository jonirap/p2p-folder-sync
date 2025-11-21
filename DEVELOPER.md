# Developer Guide

This guide helps developers set up their environment, understand the codebase, and contribute effectively to the P2P Folder Sync project.

## Table of Contents

1. [Development Environment Setup](#development-environment-setup)
2. [Building the Project](#building-the-project)
3. [Running Tests](#running-tests)
4. [Code Organization](#code-organization)
5. [Development Workflow](#development-workflow)
6. [Testing Guidelines](#testing-guidelines)
7. [Code Style and Standards](#code-style-and-standards)
8. [Debugging](#debugging)
9. [Common Development Tasks](#common-development-tasks)
10. [Pull Request Process](#pull-request-process)

## Development Environment Setup

### Prerequisites

#### Required Tools

- **Go 1.21+**: [Download](https://golang.org/dl/)
  ```bash
  go version  # Should show 1.21 or higher
  ```

- **Git**: For version control
  ```bash
  git --version
  ```

- **Make**: Build automation (usually pre-installed on Linux/macOS)
  ```bash
  make --version
  ```

- **SQLite 3.x**: Database (usually pre-installed)
  ```bash
  sqlite3 --version
  ```

#### Recommended Tools

- **golangci-lint**: Code linting
  ```bash
  # Install via go install
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

  # Or via script
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
  ```

- **Docker & Docker Compose**: For container testing
  ```bash
  docker --version
  docker-compose --version
  ```

- **VS Code** (recommended IDE) with extensions:
  - Go extension (golang.go)
  - Go Test Explorer
  - EditorConfig
  - GitLens

### Initial Setup

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd p2p-folder-sync
   ```

2. **Install Go dependencies**
   ```bash
   go mod download
   go mod verify
   ```

3. **Verify your setup**
   ```bash
   make check  # Runs fmt, lint, and tests
   ```

4. **Build the project**
   ```bash
   make build  # Creates ./bin/p2p-sync
   ```

### IDE Configuration

#### VS Code (Recommended)

Create or update [.vscode/settings.json](/.vscode/settings.json):

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "go.formatTool": "gofmt",
  "go.formatOnSave": true,
  "go.testFlags": ["-v", "-race"],
  "go.testTimeout": "60s",
  "go.coverOnSave": true,
  "editor.formatOnSave": true,
  "files.eol": "\n"
}
```

Create [.vscode/launch.json](/.vscode/launch.json) for debugging:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch P2P Sync",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/p2p-sync",
      "env": {
        "P2P_SYNC_FOLDER": "/tmp/test-sync",
        "LOG_LEVEL": "debug"
      },
      "args": ["-config", "config/config.yaml"]
    },
    {
      "name": "Debug Current Test",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${fileDirname}"
    }
  ]
}
```

## Building the Project

### Make Targets

```bash
# Build the binary (output: ./bin/p2p-sync)
make build

# Build with verbose output
make build VERBOSE=1

# Clean build artifacts
make clean

# Build Docker image
make docker-build

# Full check (format, lint, test)
make check
```

### Manual Build

```bash
# Development build
go build -o bin/p2p-sync ./cmd/p2p-sync

# Production build (optimized)
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/p2p-sync ./cmd/p2p-sync

# With version information
VERSION=$(git describe --tags --always --dirty)
go build -ldflags="-X main.version=${VERSION}" -o bin/p2p-sync ./cmd/p2p-sync
```

### Cross-Compilation

```bash
# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o bin/p2p-sync-linux-amd64 ./cmd/p2p-sync

# macOS (arm64/M1)
GOOS=darwin GOARCH=arm64 go build -o bin/p2p-sync-darwin-arm64 ./cmd/p2p-sync

# Windows (amd64)
GOOS=windows GOARCH=amd64 go build -o bin/p2p-sync-windows-amd64.exe ./cmd/p2p-sync
```

## Running Tests

### Test Organization

```
test/
├── unit/           # Fast, isolated component tests (~5-10s)
├── integration/    # Multi-component interaction tests (~10-20s)
└── system/         # Full end-to-end tests (~30-60s)
```

### Running Tests

```bash
# All tests (recommended before commit)
make test

# With coverage report
make test-coverage
open coverage.html  # View coverage in browser

# Fast unit tests only
./test/run_system_tests.sh --unit-only

# Integration tests only
./test/run_system_tests.sh --integration-only

# Skip Docker tests (faster on WSL/macOS)
./test/run_system_tests.sh --fast

# Run specific test
go test -v ./internal/sync/... -run TestVectorClock

# Run with race detector
go test -race ./...

# Verbose output
go test -v ./...

# Run benchmarks
go test -bench=. ./internal/hashing/
```

### Test Environment Variables

```bash
# Enable test mode (disables actual networking)
P2P_TESTING_MODE=true go test ./...

# Custom ports for parallel test execution
P2P_PORT=8080 P2P_DISCOVERY_PORT=8081 go test ./test/integration/...

# Extended timeout for slow systems
go test -timeout 10m ./test/system/...
```

### Running Individual Test Files

```bash
# Test specific file
go test -v ./internal/sync/conflict/merge_test.go

# With package
go test -v ./internal/sync/conflict -run TestThreeWayMerge
```

## Code Organization

### Project Structure

```
p2p-folder-sync/
├── cmd/
│   └── p2p-sync/
│       └── main.go                 # Application entry point
│
├── internal/                       # Private application code
│   ├── sync/                       # Core synchronization logic
│   │   ├── engine.go               # Main sync orchestrator (800+ LOC)
│   │   ├── messenger.go            # Messaging interface
│   │   ├── operation.go            # Operation definitions
│   │   └── conflict/               # Conflict resolution
│   │       ├── resolver.go         # Conflict detection
│   │       └── merge.go            # 3-way merge implementation
│   │
│   ├── network/                    # Networking layer
│   │   ├── handler.go              # Message routing (500+ LOC)
│   │   ├── messenger.go            # Network messaging (560+ LOC)
│   │   ├── transport/              # QUIC/TCP transports
│   │   │   ├── quic.go
│   │   │   ├── tcp.go
│   │   │   └── fallback.go
│   │   ├── discovery/              # Peer discovery
│   │   │   └── mdns.go             # mDNS implementation
│   │   ├── messages/               # Protocol messages
│   │   │   └── types.go
│   │   ├── connection/             # Connection management
│   │   │   └── manager.go
│   │   └── flowcontrol/            # Rate limiting
│   │       └── ratelimiter.go
│   │
│   ├── database/                   # SQLite persistence
│   │   ├── db.go                   # Database init
│   │   ├── files.go                # File metadata
│   │   ├── chunks.go               # Chunk tracking
│   │   ├── operations.go           # Operation log
│   │   ├── peers.go                # Peer registry
│   │   └── migrations.go           # Schema management
│   │
│   ├── filesystem/                 # File operations
│   │   ├── watcher.go              # FS event monitoring
│   │   ├── operations.go           # File I/O
│   │   └── rename_detector.go      # Rename detection
│   │
│   ├── hashing/                    # Content hashing
│   │   ├── blake3.go               # BLAKE3 implementation
│   │   └── fileid.go               # File identification
│   │
│   ├── chunking/                   # File chunking
│   │   ├── chunker.go              # Chunk splitting
│   │   ├── manager.go              # Chunk lifecycle
│   │   ├── buffer.go               # Chunk buffering
│   │   └── assembler.go            # Chunk assembly
│   │
│   ├── crypto/                     # Encryption & auth
│   │   ├── encryption.go           # AES-256-GCM
│   │   ├── keyexchange.go          # ECDH
│   │   ├── handshake.go            # Protocol handshake
│   │   ├── auth.go                 # Authentication
│   │   └── keychain.go             # Key storage
│   │
│   ├── compression/                # Compression
│   │   ├── compressor.go           # Interface
│   │   ├── factory.go              # Algorithm selection
│   │   ├── zstd.go                 # Zstandard
│   │   ├── lz4.go                  # LZ4
│   │   └── gzip.go                 # Gzip
│   │
│   ├── config/                     # Configuration
│   │   ├── config.go               # Config structs
│   │   └── loader.go               # YAML loading
│   │
│   ├── monitoring/                 # Observability
│   │   ├── metrics.go              # OpenTelemetry
│   │   └── server.go               # Metrics endpoint
│   │
│   ├── observability/              # Logging & tracing
│   │   └── logger.go
│   │
│   └── state/                      # State management
│       ├── declaration.go          # Peer state
│       ├── reconciliation.go       # State sync
│       └── loadbalance.go          # Load distribution
│
├── test/                           # Test suites
│   ├── unit/                       # 24 test files
│   ├── integration/                # 8 test files
│   ├── system/                     # 17 test files
│   └── run_system_tests.sh         # Test runner
│
├── config/                         # Configuration examples
│   └── config.yaml
│
├── spec.md                         # Technical specification (1470 lines)
├── IMPLEMENTATION_REPORT.md        # Implementation status
├── VALIDATION_REPORT.md            # Test validation
├── README.md                       # Project overview
├── Makefile                        # Build automation
├── Dockerfile                      # Container build
├── docker-compose.yml              # Multi-peer setup
├── go.mod                          # Go dependencies
└── go.sum                          # Dependency checksums
```

### Key Packages

#### internal/sync
The core synchronization engine that orchestrates all operations.

**Key files:**
- `engine.go`: Main Engine struct, operation processing, peer coordination
- `messenger.go`: InMemoryMessenger for testing, Messenger interface
- `operation.go`: SyncOperation struct, operation types

**Responsibilities:**
- Coordinate file operations across peers
- Maintain vector clocks for causality
- Process incoming sync operations
- Handle conflict detection

#### internal/network
All networking functionality including transports and discovery.

**Key files:**
- `handler.go`: Route messages to appropriate handlers
- `messenger.go`: Network-level message sending/receiving
- `transport/fallback.go`: QUIC-to-TCP fallback logic

**Responsibilities:**
- QUIC/TCP transport management
- mDNS peer discovery
- Message serialization/deserialization
- Connection pooling and lifecycle

#### internal/database
SQLite-based persistence for state and operations.

**Schema:**
- `files`: File metadata with compression info
- `operations`: Operation log for recovery
- `peers`: Peer registry
- `chunks`: Chunk tracking for large files

**Responsibilities:**
- Persistent file metadata storage
- Operation logging for crash recovery
- Peer information tracking
- Chunk reassembly state

#### internal/filesystem
File system operations and change detection.

**Key components:**
- **Watcher**: Monitors file system changes via fsnotify
- **RenameDetector**: Distinguishes renames from edits using file IDs
- **Operations**: Atomic file writes, permission handling

## Development Workflow

### 1. Creating a New Feature

```bash
# Create feature branch
git checkout -b feature/your-feature-name

# Make changes
# ... edit code ...

# Run tests frequently
make test

# Format and lint
make fmt
make lint

# Commit changes
git add .
git commit -m "Add feature: description"

# Push and create PR
git push origin feature/your-feature-name
```

### 2. Fixing a Bug

```bash
# Create bug fix branch
git checkout -b fix/bug-description

# Write a failing test first
# ... add test in appropriate test/ directory ...

# Implement fix
# ... edit code ...

# Verify fix
make test

# Commit with reference to issue
git commit -m "Fix: description (closes #123)"
```

### 3. Code Review Checklist

Before submitting a PR:

- [ ] All tests pass (`make test`)
- [ ] Code is formatted (`make fmt`)
- [ ] Linter passes (`make lint`)
- [ ] New tests added for new functionality
- [ ] Documentation updated (inline and external)
- [ ] No TODOs left in code (or tracked in issues)
- [ ] Error handling is comprehensive
- [ ] Logging is appropriate (level and content)
- [ ] Performance implications considered

## Testing Guidelines

### Test Structure

Follow Go's standard testing patterns:

```go
package mypackage

import (
    "testing"
)

func TestFeatureName(t *testing.T) {
    // Arrange: Set up test data
    input := "test data"

    // Act: Execute the functionality
    result := MyFunction(input)

    // Assert: Verify expectations
    if result != expected {
        t.Errorf("MyFunction(%q) = %q, want %q", input, result, expected)
    }
}
```

### Test Categories

#### Unit Tests (test/unit/)

Test individual components in isolation.

**Characteristics:**
- Fast (<100ms per test)
- No external dependencies
- Use mocks/stubs for dependencies
- Focus on single function/method

**Example:**
```go
func TestChunkFile(t *testing.T) {
    data := []byte("test data")
    chunkSize := 4

    chunks := ChunkFile(data, chunkSize)

    if len(chunks) != 3 {
        t.Errorf("Expected 3 chunks, got %d", len(chunks))
    }
}
```

#### Integration Tests (test/integration/)

Test interaction between multiple components.

**Characteristics:**
- Medium speed (1-5s per test)
- May use real database (in-memory)
- Tests component boundaries
- Focuses on integration points

**Example:**
```go
func TestDatabaseFileOperations(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    file := &FileMetadata{...}
    err := db.InsertFile(file)
    if err != nil {
        t.Fatalf("Failed to insert: %v", err)
    }

    retrieved, err := db.GetFile(file.FileID)
    // ... assertions ...
}
```

#### System Tests (test/system/)

End-to-end testing with real networking.

**Characteristics:**
- Slower (5-30s per test)
- Tests full application flow
- Uses real or mock network
- Validates complete scenarios

**Example:**
```go
func TestMultiPeerSync(t *testing.T) {
    // Start 3 peers
    peer1 := startTestPeer(t, 8080)
    peer2 := startTestPeer(t, 8081)
    peer3 := startTestPeer(t, 8082)
    defer cleanupPeers(peer1, peer2, peer3)

    // Create file on peer1
    createTestFile(peer1.SyncFolder, "test.txt", "content")

    // Wait for sync
    waitForFileSync(t, peer2.SyncFolder, "test.txt", 30*time.Second)
    waitForFileSync(t, peer3.SyncFolder, "test.txt", 30*time.Second)

    // Verify content
    verifyFileContent(t, peer2.SyncFolder, "test.txt", "content")
}
```

### Test Utilities

#### EventDrivenWaiter

Replace sleep-based timing with event-driven waiting:

```go
waiter := &EventDrivenWaiter{
    Timeout: 30 * time.Second,
    PollInterval: 100 * time.Millisecond,
}

err := waiter.WaitForFileSync(targetPath, expectedContent)
if err != nil {
    t.Fatalf("File sync timeout: %v", err)
}
```

#### InMemoryMessenger

Fast testing without real network:

```go
messenger := sync.NewInMemoryMessenger()

engine1 := sync.NewEngineWithMessenger(config1, db1, messenger)
engine2 := sync.NewEngineWithMessenger(config2, db2, messenger)

messenger.RegisterPeer(engine1.PeerID, engine1)
messenger.RegisterPeer(engine2.PeerID, engine2)

// Now engines can communicate in-memory
```

### Writing Good Tests

**DO:**
- Use table-driven tests for multiple cases
- Test error paths, not just happy paths
- Use descriptive test names: `TestFeature_Condition_ExpectedBehavior`
- Clean up resources in defer statements
- Use sub-tests with `t.Run()` for organization

**DON'T:**
- Use sleep for timing (use waiters instead)
- Test implementation details (test behavior)
- Create interdependent tests
- Leave test databases or files behind
- Ignore test failures (fix or document skip reason)

**Example of good table-driven test:**

```go
func TestHashFile(t *testing.T) {
    tests := []struct {
        name    string
        input   []byte
        want    string
        wantErr bool
    }{
        {
            name:  "empty file",
            input: []byte{},
            want:  "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
        },
        {
            name:  "small file",
            input: []byte("hello"),
            want:  "ea8f163db38682925e4491c5e58d4bb3506ef8c14eb78a86e908c5624a67200f",
        },
        // ... more cases ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := HashFile(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("HashFile() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("HashFile() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Code Style and Standards

### Go Idioms

Follow [Effective Go](https://golang.org/doc/effective_go.html) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

### Formatting

```bash
# Format all code
gofmt -w .

# Or use make
make fmt
```

### Naming Conventions

- **Packages**: Short, lowercase, single-word names
- **Interfaces**: End with "-er" suffix (e.g., `Messenger`, `Compressor`)
- **Exported**: Start with capital letter
- **Unexported**: Start with lowercase letter
- **Constants**: CamelCase (not SCREAMING_SNAKE_CASE in Go)

### Error Handling

Always handle errors explicitly:

```go
// Good
result, err := SomeFunction()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad - ignoring error
result, _ := SomeFunction()
```

### Documentation

Document all exported functions, types, and constants:

```go
// Engine coordinates peer-to-peer file synchronization operations.
// It maintains state, processes incoming operations, and broadcasts
// local changes to connected peers.
type Engine struct {
    peerID      string
    db          *database.DB
    // ...
}

// ProcessOperation handles an incoming sync operation from a peer.
// It validates the operation, applies it to the local state, and
// broadcasts it to other peers if necessary.
//
// Returns an error if the operation is invalid or cannot be applied.
func (e *Engine) ProcessOperation(op *SyncOperation) error {
    // ...
}
```

### Concurrency

- Use channels for communication between goroutines
- Use mutexes to protect shared state
- Document locking requirements
- Avoid nested locks (deadlock risk)

```go
type SafeCounter struct {
    mu    sync.RWMutex
    count map[string]int
}

// Increment safely increments the counter for the given key.
// Safe for concurrent use.
func (sc *SafeCounter) Increment(key string) {
    sc.mu.Lock()
    defer sc.mu.Unlock()
    sc.count[key]++
}
```

## Debugging

### Enable Debug Logging

```bash
LOG_LEVEL=debug ./bin/p2p-sync
```

### Use Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug application
dlv debug ./cmd/p2p-sync -- -config config/config.yaml

# Debug test
dlv test ./internal/sync -- -test.run TestVectorClock
```

Common dlv commands:
- `break <location>` - Set breakpoint
- `continue` - Continue execution
- `next` - Step over
- `step` - Step into
- `print <var>` - Print variable
- `locals` - Show local variables

### VS Code Debugging

Use the launch configurations in `.vscode/launch.json` to debug directly in VS Code.

### Common Issues

#### Database Locked

```bash
# Check for stale lock
rm -f p2p_sync.db-wal p2p_sync.db-shm

# Or use WAL mode (should be default)
sqlite3 p2p_sync.db "PRAGMA journal_mode=WAL;"
```

#### Port Already in Use

```bash
# Find process using port
lsof -i :8080
netstat -tuln | grep 8080

# Kill process or change port
P2P_PORT=8090 ./bin/p2p-sync
```

#### Test Failures on WSL

Docker tests may fail due to credential helpers. Run with `--fast` flag:

```bash
./test/run_system_tests.sh --fast
```

## Common Development Tasks

### Adding a New Message Type

1. Define message in `internal/network/messages/types.go`:
   ```go
   type NewMessage struct {
       Type   string `json:"type"`
       Field1 string `json:"field1"`
   }
   ```

2. Add handler in `internal/network/handler.go`:
   ```go
   func (h *Handler) handleNewMessage(msg *messages.NewMessage, senderID string) error {
       // Implementation
   }
   ```

3. Register in message router

4. Add tests in `test/unit/network/`

### Adding a Compression Algorithm

1. Implement `Compressor` interface in `internal/compression/`:
   ```go
   type MyCompressor struct {}

   func (c *MyCompressor) Compress(data []byte, level int) ([]byte, error) {...}
   func (c *MyCompressor) Decompress(data []byte) ([]byte, error) {...}
   func (c *MyCompressor) Name() string { return "myalgo" }
   ```

2. Register in `factory.go`:
   ```go
   case "myalgo":
       return &MyCompressor{}, nil
   ```

3. Add tests in `test/unit/compression/`

4. Update config validation

### Adding Metrics

```go
import "internal/monitoring"

// Define metric
var myCounter = monitoring.NewCounter(
    "my_metric_total",
    "Description of metric",
)

// Use in code
myCounter.Inc()
```

## Pull Request Process

### Before Submitting

1. **Rebase on main**:
   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. **Run full checks**:
   ```bash
   make check
   ```

3. **Update documentation** if needed

4. **Write clear commit message**:
   ```
   Short summary (50 chars max)

   Detailed explanation of changes, why they were needed,
   and any implications. Reference issues with #123.
   ```

### PR Template

When creating a PR, include:

- **Description**: What changes and why
- **Testing**: How you tested the changes
- **Screenshots**: If UI/output changes
- **Checklist**: Verification steps completed
- **Related Issues**: Link to GitHub issues

### Review Process

1. Automated CI checks must pass
2. At least one reviewer approval required
3. Address all review comments
4. Maintainer will merge when ready

## Additional Resources

- [Specification](spec.md): Detailed technical specification
- [Implementation Report](IMPLEMENTATION_REPORT.md): Current implementation status
- [Validation Report](VALIDATION_REPORT.md): Test validation results
- [Go Documentation](https://golang.org/doc/): Official Go docs
- [Effective Go](https://golang.org/doc/effective_go.html): Go best practices

## Getting Help

- **Questions**: Open a GitHub discussion
- **Bugs**: File a GitHub issue with reproduction steps
- **Features**: Propose in GitHub issues with use case

---

Welcome to the project! If you have questions not covered here, please ask in GitHub issues or discussions.
