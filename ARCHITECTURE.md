# Architecture Documentation

This document describes the system architecture, design decisions, and component interactions of the P2P Folder Sync system.

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture Diagrams](#architecture-diagrams)
3. [Core Components](#core-components)
4. [Data Flow](#data-flow)
5. [Network Protocol](#network-protocol)
6. [Storage Architecture](#storage-architecture)
7. [Security Architecture](#security-architecture)
8. [Design Decisions](#design-decisions)
9. [Scalability Considerations](#scalability-considerations)
10. [Future Enhancements](#future-enhancements)

---

## System Overview

P2P Folder Sync is a distributed peer-to-peer file synchronization system that maintains consistent copies of a shared folder across multiple peers in a local network.

### Key Characteristics

- **Architecture**: Decentralized peer-to-peer
- **Consistency Model**: Eventual consistency with vector clocks
- **Communication**: Direct peer-to-peer with QUIC/TCP
- **Discovery**: Automatic via mDNS/DNS-SD
- **Encryption**: End-to-end with AES-256-GCM
- **Conflict Resolution**: 3-way merge for text, LWW for binary

### Design Philosophy

1. **No Central Server**: Fully decentralized architecture
2. **Automatic Recovery**: Resilient to network failures
3. **Data Integrity**: Cryptographic hashes verify all data
4. **Performance**: Chunking, compression, and parallel transfers
5. **Security First**: End-to-end encryption by default

---

## Architecture Diagrams

### High-Level System Architecture

```mermaid
graph TB
    subgraph Node["P2P Sync Node"]
        subgraph AppLayer["Application Layer"]
            FileWatcher["File System<br/>Watcher"]
            SyncEngine["Sync Engine"]
            StateManager["State Manager"]

            FileWatcher <--> SyncEngine
            SyncEngine <--> StateManager
        end

        subgraph SyncLayer["Synchronization Layer"]
            Chunking["Chunking"]
            Compress["Compression"]
            Hashing["Hashing<br/>(BLAKE3)"]
        end

        subgraph CryptoLayer["Encryption Layer"]
            Encryption["AES-256-GCM + ECDH Key Exchange"]
        end

        subgraph NetworkLayer["Network Transport Layer"]
            QUIC["QUIC<br/>(Primary)"]
            TCP["TCP<br/>(Fallback)"]
            mDNS["mDNS/UDP<br/>(Discovery)"]
            FlowControl["Flow Control"]
            ConnMgr["Connection<br/>Manager"]
        end

        subgraph PersistenceLayer["Persistence Layer"]
            DB["SQLite + WAL<br/>• File Metadata<br/>• Operation Log<br/>• Peer Registry<br/>• Chunk Tracking"]
        end

        AppLayer --> SyncLayer
        SyncLayer --> CryptoLayer
        CryptoLayer --> NetworkLayer
        AppLayer <--> PersistenceLayer
        SyncLayer <--> PersistenceLayer
    end

    style Node fill:#f9f9f9,stroke:#333,stroke-width:2px
    style AppLayer fill:#e1f5ff,stroke:#0288d1
    style SyncLayer fill:#fff3e0,stroke:#f57c00
    style CryptoLayer fill:#fce4ec,stroke:#c2185b
    style NetworkLayer fill:#f3e5f5,stroke:#7b1fa2
    style PersistenceLayer fill:#e8f5e9,stroke:#388e3c
```

### Component Interaction Diagram

```mermaid
flowchart TD
    FS[File System Events]
    Watcher[File Watcher<br/>fsnotify]
    RenameDetector[Rename Detector]
    DB[(Database)]
    SyncEngine[Sync Engine<br/>Vector Clock<br/>Conflict Detection]
    Chunker[Chunker]
    Compressor[Compressor]
    Encryptor[Encryptor]
    Transport[Transport<br/>QUIC/TCP]
    Messenger[Network Messenger]
    ConnMgr[Connection Manager]
    mDNS[mDNS/UDP Service]
    FlowControl[Flow Control]
    MsgHandler[Message Handler]

    FS -->|fsnotify events| Watcher
    Watcher --> RenameDetector
    RenameDetector --> DB
    Watcher -->|File Change Event| SyncEngine

    SyncEngine <--> DB
    SyncEngine -->|Sync Operation| Chunker

    Chunker --> Compressor
    Compressor --> Encryptor

    Encryptor -->|Encrypted Chunks| Messenger
    Messenger --> FlowControl
    FlowControl --> Transport

    Transport -.->|Incoming| MsgHandler
    MsgHandler --> SyncEngine

    Transport <--> ConnMgr
    ConnMgr -->|Peer Discovery| mDNS

    style FS fill:#e1f5ff
    style Watcher fill:#e1f5ff
    style SyncEngine fill:#fff3e0
    style DB fill:#e8f5e9
    style Encryptor fill:#fce4ec
    style Transport fill:#f3e5f5
    style mDNS fill:#f3e5f5
```

### Multi-Peer Network Topology

```mermaid
graph TD
    A[Peer A<br/>Origin]
    B[Peer B]
    C[Peer C]
    D[Peer D]

    A <--> B
    A <--> C
    A <--> D
    B <--> C
    B <--> D
    C <--> D

    note[Full Mesh Network<br/>Each peer connects to all other peers]

    style A fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    style B fill:#e1f5ff,stroke:#0288d1
    style C fill:#e1f5ff,stroke:#0288d1
    style D fill:#e1f5ff,stroke:#0288d1
    style note fill:#f5f5f5,stroke:#999,stroke-dasharray: 5 5
```

---

## Core Components

### 1. Sync Engine (`internal/sync/`)

**Purpose**: Orchestrates all synchronization operations.

**Responsibilities**:
- Maintain vector clocks for causality tracking
- Process incoming file operations
- Detect and resolve conflicts
- Coordinate with other components

**Key Interfaces**:
```go
type Engine struct {
    peerID      string
    db          *database.DB
    messenger   Messenger
    vectorClock VectorClock
    // ...
}

func (e *Engine) ProcessLocalChange(path string, eventType EventType) error
func (e *Engine) HandleIncomingFile(data []byte, op *SyncOperation) error
func (e *Engine) HandleIncomingRename(op *SyncOperation) error
```

**Design Patterns**:
- **Observer Pattern**: Watches file system events
- **Command Pattern**: Sync operations as commands
- **Strategy Pattern**: Different conflict resolution strategies

### 2. Network Layer (`internal/network/`)

**Purpose**: Handles all peer-to-peer communication.

**Components**:

#### NetworkMessenger
- Sends/receives messages
- Manages encryption
- Handles retries and acknowledgments

#### NetworkMessageHandler
- Routes incoming messages to handlers
- Processes different message types
- Manages chunk assembly

#### Transport Layer
- **QUIC Transport**: Primary, fast, multiplexed
- **TCP Transport**: Fallback for compatibility
- **Automatic Failover**: Switches on QUIC failure

#### Connection Manager
- Tracks active peer connections
- Manages session keys
- Updates connection states

**Key Methods**:
```go
func (nm *NetworkMessenger) SendFile(peerID string, fileData []byte, metadata *SyncOperation) error
func (nm *NetworkMessenger) BroadcastOperation(op *SyncOperation) error
func (h *NetworkMessageHandler) HandleMessage(msg *Message) error
```

### 3. File System Layer (`internal/filesystem/`)

**Purpose**: Monitors and manipulates file system.

**Components**:

#### File Watcher
- Uses `fsnotify` for file system events
- Filters out remote changes (sync loop prevention)
- Debounces rapid changes

#### Rename Detector
- Distinguishes renames from delete+create
- Uses stable file IDs (BLAKE3 hash of content prefix)
- Temporal analysis (5-second window)

#### File Operations
- Atomic file writes (temp + rename)
- Permission preservation
- Extended attributes (xattr) management

**Sync Loop Prevention**:
```go
// Mark all incoming writes as remote
operation := FileOperation{
    Source: "remote",  // Critical: prevents re-broadcast
    // ...
}

// Temporarily disable watcher for this path
fileWatcher.IgnorePath(metadata.Path)
defer fileWatcher.WatchPath(metadata.Path)
```

### 4. Storage Layer (`internal/database/`)

**Purpose**: Persistent state management with SQLite.

**Database Schema**:
- **files**: File metadata with compression info
- **operations**: Append-only operation log
- **peers**: Peer registry
- **chunks**: Chunk tracking for resumable transfers

**Configuration**:
- Write-Ahead Logging (WAL) for concurrency
- Automatic checkpointing
- Periodic log compaction

**Design Benefits**:
- ACID transactions
- Crash recovery via operation log
- Efficient queries with proper indexes

### 5. Chunking System (`internal/chunking/`)

**Purpose**: Split large files into manageable chunks.

**Components**:

#### Chunker
- Adaptive chunk sizing (64KB - 2MB)
- Generates chunk metadata
- Computes chunk hashes (BLAKE3)

#### Assembler
- Handles out-of-order chunk delivery
- Verifies chunk integrity
- Assembles complete files

**Chunk Structure**:
```go
type Chunk struct {
    FileID      string
    ChunkID     int      // Sequential number
    Offset      int64    // Byte offset in file
    Length      int64    // Chunk size
    Data        []byte   // Chunk content
    Hash        string   // BLAKE3(Data)
    IsLast      bool
    TotalChunks int
}
```

### 6. Compression Layer (`internal/compression/`)

**Purpose**: Reduce transfer size and improve efficiency.

**Supported Algorithms**:
- **Zstandard (zstd)**: Best compression ratio, default
- **LZ4**: Very fast, lower compression
- **Gzip**: Compatible, moderate performance

**Factory Pattern**:
```go
func NewCompressor(algorithm string, level int) (Compressor, error)
```

### 7. Crypto Layer (`internal/crypto/`)

**Purpose**: End-to-end encryption and key exchange.

**Components**:

#### Key Exchange
- ECDH with Curve25519
- Session key derivation (HKDF-SHA256)
- 24-hour key rotation

#### Encryption
- AES-256-GCM (authenticated encryption)
- 96-bit random IV per message
- 128-bit authentication tag

**Message Flow**:
```
Plaintext → JSON Encoding → AES-256-GCM Encryption → Network
Network → GCM Decryption → JSON Decoding → Plaintext
```

### 8. Observability (`internal/monitoring/`, `internal/observability/`)

**Purpose**: Metrics, tracing, and logging.

**Components**:
- OpenTelemetry metrics collection
- Distributed tracing
- Structured JSON logging
- Prometheus metrics endpoint

---

## Data Flow

### File Creation/Update Flow

```mermaid
flowchart TD
    User[User creates file.txt]
    FSNotify[fsnotify event<br/>CREATE]
    FileWatcher[File Watcher<br/>- Check if remote<br/>- Generate file ID]
    SyncEngine[Sync Engine<br/>- Increment vector clock<br/>- Create SyncOperation<br/>- Hash file content]
    Database[Database<br/>- Insert file metadata<br/>- Log operation]
    Messenger[Network Messenger<br/>- Read file data<br/>- Compress if needed<br/>- Chunk if >512KB]
    Encryption[Encryption Layer<br/>- Encrypt chunks<br/>- Add auth tags]
    Transport[Transport Layer<br/>- Send via QUIC/TCP<br/>- Wait for ACK<br/>- Retry on failure]
    Peer[Peer receives & stores]

    User --> FSNotify
    FSNotify --> FileWatcher
    FileWatcher --> SyncEngine
    SyncEngine --> Database
    Database --> Messenger
    Messenger --> Encryption
    Encryption --> Transport
    Transport --> Peer

    style User fill:#e1f5ff
    style FileWatcher fill:#e1f5ff
    style SyncEngine fill:#fff3e0
    style Database fill:#e8f5e9
    style Messenger fill:#f3e5f5
    style Encryption fill:#fce4ec
    style Transport fill:#f3e5f5
    style Peer fill:#e8f5e9
```

### File Receive Flow

```mermaid
flowchart TD
    Network[Network receives<br/>encrypted msg]
    Transport[Transport Layer<br/>- Verify message ID<br/>- Check sender]
    Decryption[Decryption<br/>- Get session key<br/>- Decrypt payload<br/>- Verify auth tag]
    Handler[Message Handler<br/>- Route by type<br/>- Decompress if needed]
    ChunkAssembly[Chunk Assembly<br/>- Store chunk<br/>- Check if complete<br/>- Verify file hash]
    SyncEngine[Sync Engine<br/>- Mark as remote source<br/>- Check for conflicts]
    FileSystem[File System<br/>- Disable watcher<br/>- Write atomically<br/>- Re-enable watcher]
    DBUpdate[Database Update<br/>- Update metadata<br/>- Log operation]
    ACK[Send ACK to sender]

    Network --> Transport
    Transport --> Decryption
    Decryption --> Handler
    Handler --> ChunkAssembly
    ChunkAssembly --> SyncEngine
    SyncEngine --> FileSystem
    FileSystem --> DBUpdate
    DBUpdate --> ACK

    style Network fill:#f3e5f5
    style Transport fill:#f3e5f5
    style Decryption fill:#fce4ec
    style Handler fill:#f3e5f5
    style ChunkAssembly fill:#fff3e0
    style SyncEngine fill:#fff3e0
    style FileSystem fill:#e1f5ff
    style DBUpdate fill:#e8f5e9
    style ACK fill:#f3e5f5
```

### Conflict Resolution Flow

```mermaid
flowchart TD
    Start[Two peers edit<br/>same file]
    VectorClock[Vector Clock Comparison<br/>Peer A: &#123;A:5, B:3&#125;<br/>Peer B: &#123;A:4, B:6&#125;<br/>→ Concurrent!]
    Detection[Conflict Detection<br/>- Same file ID<br/>- Different checksums<br/>- Concurrent clocks]
    IsText{Is Text File?}
    ThreeWay[3-Way Merge]
    LWW[Last Write Wins<br/>LWW]
    Apply[Apply Resolution<br/>- Update file<br/>- Merge vector clocks<br/>- Broadcast result]

    Start --> VectorClock
    VectorClock --> Detection
    Detection --> IsText
    IsText -->|Yes| ThreeWay
    IsText -->|No| LWW
    ThreeWay --> Apply
    LWW --> Apply

    style Start fill:#e1f5ff
    style VectorClock fill:#fff3e0
    style Detection fill:#fff3e0
    style IsText fill:#fff9c4
    style ThreeWay fill:#f3e5f5
    style LWW fill:#f3e5f5
    style Apply fill:#e8f5e9
```

---

## Network Protocol

### Message Types and Flow

#### Discovery and Connection

```mermaid
sequenceDiagram
    participant A as Peer A
    participant B as Peer B

    A->>B: discovery
    B->>A: discovery_response

    A->>B: handshake (ECDH public key)
    B->>A: handshake_ack (ECDH public key)
    A->>B: handshake_complete

    Note over A,B: Session keys established

    A->>B: state_declaration (file manifest)
    B->>A: state_declaration (file manifest)

    Note over A,B: Synchronization begins
```

#### File Synchronization

```mermaid
sequenceDiagram
    participant A as Peer A (Sender)
    participant B as Peer B (Receiver)

    A->>B: sync_operation (metadata, small file data)
    B->>A: operation_ack (success)

    Note over A,B: For large files:

    A->>B: chunk (0)
    A->>B: chunk (1)
    A->>B: chunk (2)
    A->>B: chunk (n)

    B->>A: chunk_ack
    B->>A: chunk_ack
    B->>A: chunk_ack
    B->>A: chunk_ack

    Note over A,B: File assembled and verified
```

### Protocol Layers

```mermaid
flowchart TD
    App[Application Layer<br/>SyncOperation, FileRequest, etc]
    Msg[Message Layer<br/>Message serialization, routing]
    Enc[Encryption Layer<br/>AES-256-GCM]
    Trans[Transport Layer<br/>QUIC primary, TCP fallback]
    Net[Network Layer<br/>UDP/TCP, IP]

    App --> Msg
    Msg --> Enc
    Enc --> Trans
    Trans --> Net

    style App fill:#fff3e0,stroke:#f57c00
    style Msg fill:#e1f5ff,stroke:#0288d1
    style Enc fill:#fce4ec,stroke:#c2185b
    style Trans fill:#f3e5f5,stroke:#7b1fa2
    style Net fill:#e8f5e9,stroke:#388e3c
```

---

## Storage Architecture

### Database Design

**SQLite Database (WAL Mode Enabled)**

```mermaid
erDiagram
    files {
        string file_id PK
        string path
        string checksum
        int size
        datetime mtime
        string peer_id
        bool compressed
        int original_size
    }

    operations {
        int sequence PK
        string operation_id
        datetime timestamp
        string type
        string peer_id
        string file_id FK
        bool acknowledged
        bool persisted
    }

    peers {
        string peer_id PK
        string address
        int port
        string session_key
        datetime last_seen
        string state
    }

    chunks {
        string file_id PK
        int chunk_id PK
        string chunk_hash
        int offset
        int length
        bool received
    }

    files ||--o{ operations : "tracks"
    files ||--o{ chunks : "contains"
```

### File System Layout

```
/var/lib/p2p-sync/
├── sync/                    # Synchronized folder
│   ├── file1.txt
│   ├── file2.jpg
│   └── subdir/
│       └── file3.pdf
├── data/                    # Application data
│   ├── p2p_sync.db          # SQLite database
│   ├── p2p_sync.db-wal      # Write-ahead log
│   └── p2p_sync.db-shm      # Shared memory
└── logs/                    # Application logs
    └── p2p-sync.log
```

---

## Security Architecture

### Threat Model

**Threats Considered**:
1. Eavesdropping on network traffic
2. Man-in-the-middle attacks
3. Data tampering in transit
4. Unauthorized peer access
5. File corruption (accidental or malicious)

**Threats NOT Addressed**:
1. Compromised peer nodes (trusted environment assumed)
2. Physical access to storage
3. DoS attacks
4. Supply chain attacks

### Security Layers

```mermaid
flowchart TD
    Auth[Authentication Layer<br/>- Pre-shared keys<br/>- Certificate-based optional<br/>- Trust-on-first-use TOFU]
    KeyExchange[Key Exchange Layer<br/>- ECDH with Curve25519<br/>- HKDF-SHA256 key derivation<br/>- 24-hour key rotation]
    Encryption[Encryption Layer<br/>- AES-256-GCM<br/>- Per-message random IV<br/>- Authenticated encryption]
    Integrity[Integrity Layer<br/>- BLAKE3 content hashing<br/>- Chunk-level verification<br/>- File-level verification]

    Auth --> KeyExchange
    KeyExchange --> Encryption
    Encryption --> Integrity

    style Auth fill:#e1f5ff,stroke:#0288d1,stroke-width:2px
    style KeyExchange fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    style Encryption fill:#fce4ec,stroke:#c2185b,stroke-width:2px
    style Integrity fill:#e8f5e9,stroke:#388e3c,stroke-width:2px
```

---

## Design Decisions

### 1. Why P2P Instead of Client-Server?

**Decision**: Fully decentralized peer-to-peer architecture

**Rationale**:
- ✅ No single point of failure
- ✅ Scales horizontally without server upgrades
- ✅ Works in air-gapped environments
- ✅ Lower latency (direct peer communication)
- ❌ More complex conflict resolution
- ❌ Harder to maintain global consistency

**Trade-offs Accepted**: Eventual consistency model instead of strong consistency

### 2. Why SQLite Instead of Key-Value Store?

**Decision**: SQLite with WAL mode

**Rationale**:
- ✅ ACID transactions for consistency
- ✅ Powerful query capabilities (complex filters, joins)
- ✅ Built-in crash recovery
- ✅ No external dependencies
- ✅ Excellent performance for <100GB datasets
- ❌ Not suitable for massive scale (millions of files)

### 3. Why QUIC with TCP Fallback?

**Decision**: QUIC as primary, TCP as fallback

**Rationale**:
- ✅ QUIC: Multiplexing without head-of-line blocking
- ✅ QUIC: Faster connection establishment (0-RTT)
- ✅ TCP: Universal compatibility
- ✅ TCP: Works through restrictive firewalls
- ❌ Added complexity of dual stack

### 4. Why Vector Clocks for Causality?

**Decision**: Vector clocks instead of Lamport timestamps

**Rationale**:
- ✅ Detects concurrent operations accurately
- ✅ Enables better conflict detection
- ✅ Works in distributed environment
- ❌ O(n) space per peer (n = number of peers)
- ❌ Requires periodic compaction

**Alternatives Considered**: Lamport timestamps (insufficient), hybrid logical clocks (too complex)

### 5. Why BLAKE3 Instead of SHA-256?

**Decision**: BLAKE3 for all hashing

**Rationale**:
- ✅ 10x faster than SHA-256
- ✅ Parallelizable (SIMD support)
- ✅ Cryptographically secure
- ✅ Incremental hashing support
- ❌ Less widely known (but well-vetted)

### 6. Why In-Memory Testing (InMemoryMessenger)?

**Decision**: Mock messenger for fast unit tests

**Rationale**:
- ✅ Tests run in milliseconds instead of seconds
- ✅ No network flakiness in CI/CD
- ✅ Easier to simulate failure scenarios
- ✅ Deterministic test execution
- ❌ Doesn't test real network behavior (covered by integration tests)

---

## Scalability Considerations

### Current Limits

| Aspect | Current Limit | Bottleneck |
|--------|---------------|------------|
| Peers | ~50 peers | Vector clock size, broadcast overhead |
| Files | ~1 million | SQLite performance, memory for manifest |
| File Size | ~10 GB | Memory for chunk buffers |
| Throughput | ~500 MB/s | Network bandwidth, CPU for encryption |
| Sync Latency | <1 second | Network RTT, processing time |

### Scaling Strategies

#### Horizontal Scaling
- Add more peers to distribute load
- Geographic distribution reduces latency
- Each peer handles full dataset (redundancy)

#### Vertical Scaling
- More CPU: Faster hashing and compression
- More RAM: Larger chunk buffers, more concurrent transfers
- Faster storage: SSD improves database performance

#### Optimization Opportunities
1. **Delta Sync**: Only send changed blocks (rsync-style)
2. **Merkle Trees**: Efficient bulk synchronization
3. **Selective Sync**: Only sync subset of files per peer
4. **Hierarchical P2P**: Introduce super-peers for large networks

---

## Future Enhancements

### Planned Features

1. **NAT Traversal** (High Priority)
   - STUN/TURN server support
   - Hole punching for cross-NAT communication
   - Relay servers as fallback

2. **Content Deduplication** (Medium Priority)
   - Block-level dedup across files
   - Reduces storage and transfer overhead
   - Content-addressable storage

3. **File Versioning** (Medium Priority)
   - Keep N versions of each file
   - Point-in-time recovery
   - Version pruning policies

4. **Web UI** (Low Priority)
   - Status dashboard
   - Configuration management
   - Log viewer

5. **Mobile Support** (Future)
   - iOS/Android sync clients
   - Battery-efficient sync
   - Cellular-aware transfers

### Research Areas

- **CRDTs** for conflict-free operations
- **Byzantine fault tolerance** for untrusted networks
- **Blockchain** for tamper-proof operation logs
- **AI-based** conflict resolution

---

## References

- [Specification Document](spec.md)
- [API Reference](API_REFERENCE.md)
- [Developer Guide](DEVELOPER.md)
- [Deployment Guide](DEPLOYMENT.md)

---

**Document Version**: 1.0
**Last Updated**: January 2025
**Status**: Complete System Architecture
