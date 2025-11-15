# Peer-to-Peer Folder Synchronization System Specification

## 1. Overview

This specification describes a distributed peer-to-peer folder synchronization system that enables multiple peers to maintain consistent copies of a shared folder across a local network. The system is designed to handle unreliable network conditions, support large files through chunking, distinguish between file renames and edits, and provide secure encrypted communication.

### 1.1 Design Goals

- **Reliability**: No data loss even with unstable network connections
- **Efficiency**: Support for large files through intelligent chunking
- **Security**: End-to-end encryption for all data transfers
- **Autonomy**: Automatic peer discovery within local network
- **Scalability**: Support for asynchronous updates of multiple files
- **Robustness**: Handle out-of-order chunk delivery
- **Intelligence**: Distinguish between file renames and content edits

### 1.2 System Architecture

The system consists of:
- **Peer Nodes**: Each peer maintains a local copy of the synchronized folder
- **Discovery Service**: Automatic peer discovery via Layer 3 network protocols
- **Sync Engine**: Handles file operations, chunking, and conflict resolution
- **Encryption Layer**: Manages key exchange and encrypted communication
- **Logging System**: Persistent operation log for recovery and state reconstruction

## 2. Core Components

### 2.1 File Identification and Change Detection

#### 2.1.1 File Identifiers

Each file is assigned a stable **File ID** (FID) that persists across renames:
- Generated using content-based hashing: `BLAKE3(first_64KB_of_content + initial_size + creation_time)`
- For empty files: `BLAKE3(creation_time + initial_size + peer_id)`
- Stored in extended attributes (xattrs) or a metadata database
- Used to track file identity even when path changes
- Independent of file path to support rename detection

#### 2.1.2 Distinguishing Renames from Edits

The system distinguishes between renames and edits using:

1. **File ID Matching**: If a deleted file's FID matches a newly created file's FID, it's a rename
2. **Content Hashing**: Compare file checksums:
   - **Rename**: Old file checksum == New file checksum
   - **Edit**: Old file checksum != New file checksum
3. **Metadata Comparison**: Compare file size, modification time patterns
4. **Temporal Analysis**: Renames typically occur within a short time window

**Algorithm**:
```
When file deleted:
  - Store FID, checksum, size, mtime in "recent_deletes" (TTL: 5 seconds)

When file created:
  - Check if FID matches any recent delete
  - If match found AND checksum matches: RENAME operation
  - If match found BUT checksum differs: DELETE + CREATE (edit)
  - If no match: CREATE operation
```

### 2.2 Hashing Strategy

#### 2.2.1 Hash Algorithm

- **Primary Algorithm**: BLAKE3
  - Fast, secure, parallelizable
  - Supports incremental hashing for large files
  - 256-bit output (32 bytes), encoded as hex string

#### 2.2.2 Hash Usage

1. **File Content Hash**: Full file checksum for integrity verification
2. **Chunk Hash**: Individual chunk checksums for out-of-order assembly
3. **Metadata Hash**: Hash of file metadata (path, size, mtime) for change detection
4. **Merkle Tree**: For efficient bulk synchronization (optional optimization)

**Hash Computation**:
```
file_hash = BLAKE3(file_content)
chunk_hash[i] = BLAKE3(chunk_data[i])
metadata_hash = BLAKE3(path + size + mtime + mode)
```

### 2.3 Chunking System

#### 2.3.1 Chunking Strategy

Files larger than a threshold (default: 1MB) are split into chunks:

- **Adaptive Chunking**: Chunk size adapts based on network conditions
  - Minimum: 64 KB
  - Maximum: 2 MB
  - Default: 512 KB
- **Chunk Identification**: Each chunk has:
  - `chunk_id`: Sequential number (0, 1, 2, ...)
  - `file_id`: Reference to parent file
  - `chunk_hash`: BLAKE3 hash of chunk data
  - `offset`: Byte offset in original file
  - `length`: Chunk size in bytes
  - `is_last`: Boolean flag for final chunk

#### 2.3.2 Out-of-Order Chunk Handling

The system supports receiving chunks in any order:

1. **Chunk Buffer**: Maintain a buffer per file for incoming chunks
2. **Chunk Map**: `Map<chunk_id, chunk_data>` for received chunks
3. **Assembly Logic**:
   ```
   When chunk received:
     - Store in buffer[file_id][chunk_id] = chunk_data
     - Check if all chunks received (track expected total)
     - If complete: Assemble in order, verify file hash
     - If incomplete: Wait for remaining chunks
   ```

4. **Timeout and Retransmission**:
   - Track last chunk received timestamp
   - If timeout (30s) and chunks missing: request retransmission
   - Use chunk request message to ask for specific chunks

#### 2.3.3 Chunk Transfer Protocol

```
Chunk Message Format:
{
  type: "chunk",
  file_id: string,
  file_hash: string,      // Full file hash for verification
  chunk_id: number,
  total_chunks: number,
  offset: number,
  length: number,
  chunk_hash: string,
  data: Buffer,           // Encrypted chunk data
  is_last: boolean
}
```

### 2.4 File Compression System

#### 2.4.1 Compression Strategy

Files larger than a configurable threshold are compressed before chunking to reduce transfer size and improve efficiency:

- **Threshold-Based Compression**: Only compress files above `file_size_threshold` (default: 1MB)
- **Algorithm Selection**: Support for zstd (default), LZ4, gzip, or none
- **Compression Level**: Configurable compression level per algorithm
- **Chunk-Level Compression**: Optional additional compression of individual chunks

#### 2.4.2 Compression Workflow

**File Preparation**:
```go
if file.Size > config.Compression.FileSizeThreshold && config.Compression.Enabled {
    compressedData, err := compress(file.Data, config.Compression.Algorithm, config.Compression.Level)
    if err != nil {
        return fmt.Errorf("compression failed: %w", err)
    }
    originalSize := file.Size
    file.Size = int64(len(compressedData))
    file.Compressed = true
    file.OriginalSize = &originalSize
    file.CompressionAlgorithm = &config.Compression.Algorithm
} else {
    file.Compressed = false
}
```

**Decompression on Receipt**:
```go
if receivedFile.Compressed {
    decompressedData, err := decompress(receivedFile.Data, *receivedFile.CompressionAlgorithm)
    if err != nil {
        return fmt.Errorf("decompression failed: %w", err)
    }
    if int64(len(decompressedData)) != *receivedFile.OriginalSize {
        return fmt.Errorf("decompressed size %d does not match expected %d",
            len(decompressedData), *receivedFile.OriginalSize)
    }
    receivedFile.Data = decompressedData
    receivedFile.Size = *receivedFile.OriginalSize
}
```

#### 2.4.3 Compression Metadata

**Extended File Metadata**:
- `compressed`: Boolean flag indicating compression status
- `original_size`: Uncompressed file size (when compressed)
- `compression_algorithm`: Algorithm used ("zstd", "lz4", "gzip")
- `compression_ratio`: Calculated as `compressed_size / original_size`

#### 2.4.4 Compression Algorithms

**Zstandard (zstd)**:
- Default algorithm with excellent compression ratio and speed
- Levels: 1-22 (default: 3)
- Streaming compression support for large files

**LZ4**:
- Fast compression with good speed/compression tradeoff
- Levels: 1-16 (default: 1)
- Very fast decompression

**Gzip**:
- Traditional algorithm with wide compatibility
- Levels: 1-9 (default: 6)
- Good for text files and network transfer

### 2.5 Asynchronous Multi-File Updates

The system supports concurrent synchronization of multiple files:

1. **Operation Queue**: Per-file operation queues prevent conflicts
2. **Parallel Transfers**: Multiple files can transfer simultaneously
3. **Flow Control**: Per-file flow control prevents network congestion
4. **Priority System**: Critical files (small, recently modified) prioritized

**Concurrency Model**:
```
For each file operation:
  - Add to file-specific queue
  - Process queue sequentially per file
  - Multiple files processed in parallel
  - Maximum concurrent transfers: 5 (configurable)
```

### 2.6 Sync Loop Prevention

**Critical Requirement**: The system must distinguish between local file changes (that should be synchronized) and remote file changes (that should not trigger further synchronization).

#### 2.6.1 Change Origin Tracking

**Remote Change Marker**:
- All incoming file writes must be marked as "remote" operations
- File system watchers must ignore changes flagged as remote
- Database updates for remote changes must not trigger sync operations

**Implementation Strategy**:
```go
type FileOperation struct {
    Source string `json:"source"` // "local" or "remote" - Critical: prevents sync loops
    FileID string `json:"file_id"`
    // ... other fields
}

// When receiving file from peer:
func handleIncomingFile(fileData []byte, metadata FileMetadata) error {
    // Mark as remote operation
    operation := FileOperation{
        Source: "remote",
        FileID: metadata.FileID,
        // ... other metadata
    }

    // Temporarily disable file system watcher for this path
    fileWatcher.IgnorePath(metadata.Path)
    defer fileWatcher.WatchPath(metadata.Path) // Ensure re-enabling even on error

    // Write file to disk atomically
    if err := atomicWriteFile(metadata.Path, fileData); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    // Update database (marked as remote)
    if err := db.InsertFile(metadata, OperationContext{Source: "remote"}); err != nil {
        return fmt.Errorf("failed to update database: %w", err)
    }

    // Log operation but do not broadcast
    if err := logOperation(operation); err != nil {
        return fmt.Errorf("failed to log operation: %w", err)
    }

    return nil
}
```

#### 2.6.2 File System Watcher Integration

**Watcher Configuration**:
- File system watchers must support temporary ignore patterns
- Remote file writes must be excluded from change detection
- Atomic write operations must complete before re-enabling watchers

**Race Condition Prevention**:
- Use file system events with operation IDs to correlate changes
- Implement operation deduplication based on file_id and timestamp
- Maintain operation log to detect and prevent duplicate syncs

## 3. State Management and Logging

### 3.1 Operation Log

A persistent log ensures no data loss during network instability:

#### 3.1.1 Log Structure

**Log Entry Format**:
```go
type LogEntry struct {
    Sequence     int64         `json:"sequence"`      // Monotonically increasing sequence number
    Timestamp    int64         `json:"timestamp"`     // Unix timestamp (milliseconds)
    Operation    SyncOperation `json:"operation"`     // The operation data
    PeerID       string        `json:"peer_id"`       // Origin peer ID
    VectorClock  VectorClock   `json:"vector_clock"`  // Vector clock state
    Acknowledged bool          `json:"acknowledged"`  // Whether all peers acknowledged
    Persisted    bool          `json:"persisted"`     // Whether written to disk
}
```

#### 3.1.2 Log Persistence

- **Storage**: SQLite database with Write-Ahead Logging (WAL)
- **Durability**: `fsync()` after each critical operation
- **Compaction**: Periodic log compaction (remove old acknowledged entries)
- **Recovery**: On startup, replay unacknowledged operations

#### 3.1.3 Log Operations

1. **Append**: Add operation to log before broadcasting
2. **Acknowledge**: Mark operation as acknowledged when peer confirms
3. **Replay**: On reconnection, replay unacknowledged operations
4. **Checkpoint**: Periodically checkpoint acknowledged operations

### 3.2 State Declaration Protocol

Peers declare their current state to enable efficient synchronization:

#### 3.2.1 State Declaration Message

```
State Declaration:
{
  type: "state_declaration",
  peer_id: string,
  vector_clock: VectorClock,
  file_manifest: {
    file_id: string,
    path: string,
    hash: string,
    size: number,
    mtime: number,
    last_modified_by: string
  }[],
  pending_operations: LogEntry[]  // Unacknowledged operations
}
```

#### 3.2.2 State Reconciliation

When peers connect:

1. **Exchange State Declarations**: Both peers send their current state declarations
2. **Identify Synchronization Direction**: Determine which peer needs to catch up:
   - **New Peer Responsibility**: The peer with fewer/missing files requests synchronization
   - **Load Balancing**: When multiple peers are available, distribute file requests across them
3. **Request Missing Files**: New peer requests missing files from appropriate source peers
4. **Handle Conflicts**: Apply conflict resolution for divergent files
5. **Sync Pending Operations**: Exchange and apply unacknowledged operations

**File Request Load Balancing**:
- **Peer Selection**: Choose peers based on network proximity and current load
- **File Distribution**: Assign files to peers using consistent hashing or round-robin
- **Duplicate Prevention**: Each file is requested from exactly one source peer
- **Fallback Strategy**: If primary peer unavailable, request from alternative peer

**File Request Message Format**:
```go
type FileRequestMessage struct {
    Type             string           `json:"type"` // "file_request"
    RequestedFiles   []RequestedFile  `json:"requested_files"`
    PeerCapabilities PeerCapabilities `json:"peer_capabilities"`
}

type RequestedFile struct {
    FileID   string `json:"file_id"`
    Priority string `json:"priority"` // "high", "normal", "low" - For progressive sync
}

type PeerCapabilities struct {
    SupportsCompression     bool `json:"supports_compression"`
    MaxConcurrentTransfers  int  `json:"max_concurrent_transfers"`
}
```

### 3.3 Vector Clocks

Vector clocks track causal relationships between operations:

```go
type VectorClock map[string]int64  // peer_id -> sequence number per peer
```

**Operations**:
- **Increment**: On local operation, increment own counter
- **Merge**: On receiving operation, take max of each peer's counter
- **Compare**: Determine if operations are concurrent or causally ordered

## 4. Conflict Resolution

### 4.1 Conflict Detection

Conflicts occur when:
- Two peers modify the same file concurrently
- Vector clocks indicate concurrent operations (neither causally before the other)

### 4.2 Resolution Strategy: Intelligent Merge with LWW Fallback

#### 4.2.1 Primary Strategy: 3-Way Merge
For text files, use 3-way merge algorithm to preserve concurrent edits:

**Algorithm**:
```go
func resolveConflict(base, branchA, branchB File) (File, error) {
    if isTextFile(base) {
        return threeWayMerge(base, branchA, branchB)
    } else {
        return lastWriteWins(branchA, branchB), nil
    }
}
```

**3-Way Merge Process**:
1. Identify common ancestor (base version)
2. Apply diff3 algorithm to merge concurrent changes
3. For conflicts within lines, use conflict markers: `<<<<<<< branchA`, `=======`, `>>>>>>> branchB`
4. Preserve non-conflicting changes from both branches

#### 4.2.2 Fallback Strategy: Last Write Wins (LWW)
For binary files and failed merges, use timestamp-based resolution:

**Algorithm**:
```
On conflict detection:
  if (op1.timestamp > op2.timestamp):
    apply(op1)
    discard(op2)
  elif (op1.timestamp < op2.timestamp):
    apply(op2)
    discard(op1)
  else:
    // Same timestamp - use peer_id as tiebreaker
    apply(op1.peer_id < op2.peer_id ? op1 : op2)
```

**Advantages**:
- Preserves user data in most scenarios
- Automatic resolution for text files
- Predictable fallback for binary files

## 5. Encryption and Security

### 5.1 Key Exchange Protocol

#### 5.1.1 Initial Key Exchange with Authentication

1. **Discovery Phase**: Peers discover each other (unencrypted, public info only)
2. **Authentication**: Challenge-response authentication using pre-shared keys or certificates
3. **Handshake**: Establish encrypted channel with mutual authentication
4. **Key Exchange**: Use Elliptic Curve Diffie-Hellman (ECDH) with Curve25519
5. **Session Keys**: Derive symmetric session keys from shared secret

**Authentication Methods**:
- **Pre-shared Keys (PSK)**: Shared secret distributed out-of-band
- **Certificate-based**: X.509 certificates with CA validation
- **Trust-on-first-use (TOFU)**: Accept first connection, pin certificate

**Secure Handshake Protocol**:
```
Peer A → Peer B: { type: "handshake", public_key: A_pub, nonce: A_nonce, auth_challenge: challenge_A }
Peer B → Peer A: { type: "handshake_challenge", auth_response: response_B, public_key: B_pub, nonce: B_nonce, auth_challenge: challenge_B }
Peer A → Peer B: { type: "handshake_complete", auth_response: response_A }
Both peers: Derive shared_secret = ECDH(A_priv, B_pub) = ECDH(B_priv, A_pub)
Both peers: Derive session_key = HKDF(shared_secret, "p2p-sync-session", A_nonce + B_nonce)
```

#### 5.1.2 Key Rotation

- Session keys rotated periodically (every 24 hours)
- New handshake initiated before expiration
- Old keys retained briefly for in-flight messages

### 5.2 Encryption Scheme

#### 5.2.1 Symmetric Encryption

- **Algorithm**: AES-256-GCM (Galois/Counter Mode)
- **Key Size**: 256 bits
- **IV/Nonce**: 96-bit random IV per message
- **Authentication**: GCM authentication tag (128 bits) for integrity

#### 5.2.2 Encrypted Message Format

```
Encrypted Message:
{
  iv: Buffer,              // 12 bytes random IV (initialization vector)
  ciphertext: Buffer,      // Encrypted payload
  tag: Buffer              // 16 bytes GCM authentication tag
}

Plaintext Payload:
{
  type: string,
  data: any,
  timestamp: number,
  peer_id: string
}
```

#### 5.2.3 Key Management

- **Key Storage**: Encrypted keychain (OS keychain or encrypted file)
- **Key Derivation**: HKDF-SHA256 for key derivation
- **Key Exchange**: ECDH for initial key establishment

## 6. Peer Discovery

### 6.1 Layer 3 Auto-Discovery

Automatic peer discovery within local network (Layer 3 - Network Layer):

#### 6.1.1 Discovery Methods

1. **UDP Broadcast**:
   - Broadcast to subnet (e.g., 255.255.255.255)
   - Port: 8081 (discovery port)
   - Message: `{ type: "discovery", peer_id, port, capabilities }`
   - Response: `{ type: "discovery_response", peer_id, port }`
   - Periodic broadcasts every 30 seconds to discover new peers
   - **Limitation**: May not work across subnets or through firewalls

2. **mDNS/DNS-SD Discovery**:
   - Service name: `_p2p-sync._tcp.local`
   - TXT records contain: `peer_id`, `port`, `capabilities`, `version`
   - Automatic discovery in local network segments
   - Works with Apple Bonjour, Avahi (Linux), and Windows mDNS
   - More reliable than UDP broadcast in modern networks

3. **Manual Peer List**:
   - Configuration file or environment variable
   - Format: Comma-separated list of `hostname:port` or `ip:port`
   - Example: `192.168.1.10:8080,192.168.1.11:8080,peer1.local:8080`
   - Peers from manual list are always attempted for connection

#### 6.1.2 Discovery Protocol

**UDP Broadcast Protocol**:
```
Discovery Message:
{
  type: "discovery",
  peer_id: string,
  port: number,
  capabilities: {
    encryption: boolean,
    compression: boolean,
    chunking: boolean
  },
  version: string  // Protocol version for compatibility
}

Discovery Response:
{
  type: "discovery_response",
  peer_id: string,
  port: number,
  version: string
}
```

**mDNS/DNS-SD Service Record**:
```
Service: _p2p-sync._tcp.local
Port: 8080 (or configured port)
TXT Records:
  - peer_id=<unique-peer-identifier>
  - port=<service-port>
  - encryption=true
  - compression=true
  - chunking=true
  - version=1.0
```

#### 6.1.3 Peer Registry

Maintain a registry of discovered peers:
- **Active Peers**: Currently connected
- **Known Peers**: Discovered but not connected
- **Peer Info**: IP, port, last seen, capabilities

## 7. Network Protocol

### 7.1 Transport Layer

#### 7.1.1 Primary Protocol: QUIC
- **Protocol**: QUIC (Quick UDP Internet Connections) for modern transport
- **Benefits**: Built-in multiplexing, security (TLS 1.3), faster connection establishment
- **Port**: Configurable (default: 8080 for QUIC/UDP)
- **Connection Migration**: Supports connection migration across IP changes
- **0-RTT Handshake**: Faster reconnections for known peers

#### 7.1.2 Fallback Protocol: TCP
- **Protocol**: TCP for reliable delivery (fallback for networks without QUIC support)
- **Port**: Configurable (default: 8081 for TCP)
- **Keep-Alive**: TCP keep-alive enabled (60s interval)
- **Compatibility**: Works in restrictive network environments

**Protocol Negotiation**:
1. Attempt QUIC connection first
2. Fall back to TCP if QUIC fails or is not supported
3. Advertise supported protocols in discovery messages

### 7.2 Message Types

#### 7.2.1 Control Messages

```
handshake:          Initial key exchange
handshake_ack:      Key exchange acknowledgment
state_declaration:  Peer state information
file_request:       Request specific files (new peer sync)
chunk_request:      Request specific chunks
chunk_ack:          Chunk received acknowledgment
operation_ack:      Operation received acknowledgment
heartbeat:          Keep-alive message
```

#### 7.2.2 Data Messages

```
sync_operation:     File operation (create/update/delete/rename)
chunk:              File chunk data
state_sync:         Bulk state synchronization
```

### 7.3 Message Format

```go
type Message struct {
    ID            string      `json:"id"`             // Unique message ID
    Type          string      `json:"type"`           // Message type
    Timestamp     int64       `json:"timestamp"`      // Sender timestamp
    SenderID      string      `json:"sender_id"`      // Sender peer ID
    Payload       interface{} `json:"payload"`        // Message-specific data
    CorrelationID *string     `json:"correlation_id,omitempty"` // For request/response matching
}
```

### 7.4 Reliability Mechanisms

1. **Acknowledgments**: All critical messages require ACK
2. **Retransmission**: Retry unacknowledged messages with exponential backoff
3. **Sequence Numbers**: Detect duplicate and out-of-order messages
4. **Connection Recovery**: Reconnect and resume on disconnection

## 8. File Operations

### 8.1 Supported Operations

1. **CREATE**: New file created
2. **UPDATE**: File content modified
3. **DELETE**: File removed
4. **RENAME**: File renamed/moved (distinguished from delete+create)
5. **MKDIR**: Directory created
6. **RMDIR**: Directory removed

### 8.2 Operation Format

```go
type SyncOperation struct {
    ID                   string      `json:"id"`                    // Unique operation ID
    Type                 OperationType `json:"type"`                // create|update|delete|rename|mkdir|rmdir
    Path                 string      `json:"path"`                  // Current file path
    FromPath             *string     `json:"from_path,omitempty"`   // Original path (for rename)
    FileID               string      `json:"file_id"`               // Stable file identifier
    Checksum             string      `json:"checksum"`              // BLAKE3 hash of file content
    Size                 int64       `json:"size"`                  // File size in bytes
    Timestamp            int64       `json:"timestamp"`             // Operation timestamp
    VectorClock          VectorClock `json:"vector_clock"`          // Vector clock state
    PeerID               string      `json:"peer_id"`               // Origin peer ID
    Source               string      `json:"source"`                // "local" or "remote" - Critical: prevents sync loops
    Mtime                int64       `json:"mtime"`                 // File modification time
    Mode                 *uint32     `json:"mode,omitempty"`         // POSIX file permissions
    Data                 []byte      `json:"data,omitempty"`         // File content (small files only)
    ChunkID              *int        `json:"chunk_id,omitempty"`    // Chunk identifier (if chunked)
    IsLast               *bool       `json:"is_last,omitempty"`     // Last chunk flag
    Compressed           *bool       `json:"compressed,omitempty"`  // Whether file content is compressed
    OriginalSize         *int64      `json:"original_size,omitempty"` // Original uncompressed size (when compressed)
    CompressionAlgorithm *string     `json:"compression_algorithm,omitempty"` // Compression algorithm used ("zstd", "lz4", "gzip")
}
```

## 9. Recovery and Resilience

### 9.1 Network Instability Handling

1. **Operation Logging**: All operations logged before transmission
2. **Acknowledgment Tracking**: Track which peers acknowledged each operation
3. **Replay on Reconnect**: Replay unacknowledged operations when peer reconnects
4. **Chunk Resume**: Resume interrupted chunk transfers from last received chunk

### 9.2 Failure Scenarios

#### 9.2.1 Peer Disconnection

- Detect via heartbeat timeout (30 seconds)
- Mark peer as disconnected
- Continue operations, queue for reconnection
- On reconnect: exchange state declarations and sync

#### 9.2.2 Partial Chunk Transfer

- Track received chunks per file
- On resume: request missing chunks
- Verify file integrity after all chunks received

#### 9.2.3 Corrupted Data

- Verify chunk hashes on receipt
- Request retransmission if hash mismatch
- Verify file hash after assembly

#### 9.2.4 Disk Space Exhaustion

- Monitor available disk space before operations
- Pause synchronization when space < 10% available
- Implement space reclamation (delete temp files, compact logs)
- Alert user and prioritize critical file synchronization

#### 9.2.5 File Permission Errors

- Handle EACCES/ENOPERM errors gracefully
- Skip inaccessible files with warning logs
- Maintain permission consistency across peers

#### 9.2.6 Database Corruption

- Use WAL mode for atomic transactions
- Implement database integrity checks on startup
- Maintain backup of recent database state
- Automatic recovery from corruption with data preservation

#### 9.2.7 Memory Exhaustion

- Implement memory limits for chunk buffers
- Use streaming for large file operations
- Graceful degradation under memory pressure
- Monitor and log memory usage patterns

#### 9.2.8 Network Partition

- Detect partition via heartbeat failures
- Continue local operations during partition
- Implement conflict-free replicated data types (CRDTs) for key operations
- Merge states when partition heals using vector clocks

## 10. Implementation Considerations

### 10.1 Performance Optimizations

#### 10.1.1 Data Transfer Optimizations

1. **Batch Operations**: Group multiple file operations into single messages
2. **File Compression**: Threshold-based compression for large files (>1MB default)
3. **Chunk Compression**: Optional additional compression per chunk (default: enabled)
4. **Delta Sync**: For updates, send only changed chunks using rolling hash (rsync-style)
5. **Parallel Transfers**: Multiple chunks from one or more files simultaneously (configurable limit)
6. **Adaptive Chunking**: Adjust chunk size based on network conditions and file type

#### 10.1.2 Resource Management

**Memory Management**:
- Chunk buffer size limits (default: 64MB per file)
- Streaming processing for large files to avoid memory exhaustion
- Memory-mapped I/O for efficient large file handling
- Garbage collection hints for long-running processes

**CPU Management**:
- Configurable thread pools for hashing and compression
- CPU usage throttling for background operations
- Priority-based scheduling (critical files first)
- Parallel chunk hashing using SIMD instructions

**Network Management**:
- Bandwidth throttling (configurable limits per peer)
- Connection pooling with keep-alive optimization
- TCP window size optimization for high-latency networks
- Quality of Service (QoS) marking for sync traffic

**Disk I/O Management**:
- Asynchronous I/O operations to avoid blocking
- I/O priority scheduling for sync operations
- Database connection pooling
- WAL mode for concurrent database access

**Flow Control**:
- Per-peer transfer rate limiting
- Global bandwidth ceiling across all peers
- Adaptive backoff on network congestion
- Priority queues for different operation types

#### 10.1.3 Graceful Degradation

**Network Congestion Response**:
- Reduce chunk size when packet loss >5%
- Decrease concurrent transfers when latency >100ms
- Switch to TCP fallback when QUIC performance degrades
- Implement exponential backoff for retransmissions

**Resource Pressure Response**:
- Reduce memory buffers when RAM usage >80%
- Lower thread pool sizes when CPU usage >70%
- Pause non-critical operations when disk space <5%
- Switch to streaming mode when memory pressure detected

**Peer Availability Degradation**:
- Continue local operations when all peers disconnected
- Queue operations for later replay when peers reconnect
- Reduce operation frequency during network partitions
- Implement operation batching to reduce network overhead

**Performance Monitoring**:
- Continuous monitoring of key metrics (latency, throughput, error rates)
- Automatic performance tuning based on observed conditions
- Alert generation for performance degradation
- Historical performance data for trend analysis

### 10.2 Storage

#### 10.2.1 Database Schema

**SQLite Database**: `p2p_sync.db` with WAL mode enabled for concurrent access

```sql
-- File metadata and identifiers
CREATE TABLE files (
  file_id TEXT PRIMARY KEY,           -- BLAKE3 hash of content prefix
  path TEXT NOT NULL,                 -- Current file path
  checksum TEXT NOT NULL,             -- Full file BLAKE3 hash
  size INTEGER NOT NULL,              -- File size in bytes (compressed size if compressed)
  mtime REAL NOT NULL,                -- Modification time (Unix timestamp)
  mode INTEGER,                       -- POSIX file permissions
  peer_id TEXT NOT NULL,              -- Last modifying peer
  vector_clock TEXT NOT NULL,         -- JSON-encoded vector clock
  compressed INTEGER DEFAULT 0,       -- Boolean: whether file is compressed
  original_size INTEGER,              -- Original uncompressed size (when compressed)
  compression_algorithm TEXT,         -- Compression algorithm ("zstd", "lz4", "gzip")
  created_at REAL DEFAULT (unixepoch()), -- Creation timestamp
  updated_at REAL DEFAULT (unixepoch())  -- Last update timestamp
);

-- Operation log for recovery and state reconstruction
CREATE TABLE operations (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  operation_id TEXT UNIQUE NOT NULL,
  timestamp REAL NOT NULL,
  operation_type TEXT NOT NULL,       -- create|update|delete|rename|mkdir|rmdir
  peer_id TEXT NOT NULL,
  vector_clock TEXT NOT NULL,         -- JSON-encoded vector clock
  acknowledged INTEGER DEFAULT 0,     -- Boolean: all peers acknowledged
  persisted INTEGER DEFAULT 0,        -- Boolean: written to disk
  file_id TEXT,                       -- Reference to files table
  path TEXT NOT NULL,                 -- Current file path
  from_path TEXT,                     -- Original path (for renames)
  checksum TEXT,                      -- File content hash
  size INTEGER,                       -- File size (compressed size if compressed)
  mtime REAL,                         -- Modification time
  mode INTEGER,                       -- File permissions
  chunk_count INTEGER DEFAULT 0,      -- Number of chunks (for large files)
  data BLOB,                          -- File content (small files only)
  compressed INTEGER DEFAULT 0,       -- Boolean: whether file is compressed
  original_size INTEGER,              -- Original uncompressed size (when compressed)
  compression_algorithm TEXT          -- Compression algorithm ("zstd", "lz4", "gzip")
);

-- Peer registry and connection state
CREATE TABLE peers (
  peer_id TEXT PRIMARY KEY,
  address TEXT,                       -- IP address or hostname
  port INTEGER,
  public_key TEXT,                    -- ECDH public key (base64)
  certificate TEXT,                   -- X.509 certificate (optional)
  capabilities TEXT,                  -- JSON: encryption, compression, etc.
  last_seen REAL,                     -- Last contact timestamp
  connection_state TEXT DEFAULT 'disconnected', -- connected|disconnected|connecting
  trust_level TEXT DEFAULT 'unknown', -- trusted|unknown|blocked
  created_at REAL DEFAULT (unixepoch())
);

-- Chunk tracking for resumable transfers
CREATE TABLE chunks (
  file_id TEXT NOT NULL,
  chunk_id INTEGER NOT NULL,
  chunk_hash TEXT NOT NULL,
  offset INTEGER NOT NULL,            -- Byte offset in file
  length INTEGER NOT NULL,            -- Chunk size in bytes
  received INTEGER DEFAULT 0,         -- Boolean: chunk received
  received_at REAL,                   -- When chunk was received
  PRIMARY KEY (file_id, chunk_id),
  FOREIGN KEY (file_id) REFERENCES files(file_id)
);

-- Configuration overrides
CREATE TABLE config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at REAL DEFAULT (unixepoch())
);

-- Indexes for performance
CREATE INDEX idx_operations_timestamp ON operations(timestamp);
CREATE INDEX idx_operations_file_id ON operations(file_id);
CREATE INDEX idx_operations_acknowledged ON operations(acknowledged);
CREATE INDEX idx_files_path ON files(path);
CREATE INDEX idx_peers_last_seen ON peers(last_seen);
CREATE INDEX idx_chunks_file_id ON chunks(file_id);
```

#### 10.2.2 File System Storage

- **Synchronized Folder**: Direct file storage in configured folder
- **Temporary Files**: `.p2p-sync-tmp-` prefix for atomic writes
- **Extended Attributes**: Store file IDs using `xattr` when supported:
  - Key: `user.p2p_sync.file_id`
  - Value: Base64-encoded file identifier
- **Atomic Operations**: Use temporary files and `rename()` for atomic updates

### 10.3 Configuration

```yaml
sync:
  folder_path: "/path/to/sync"
  chunk_size_min: 65536      # 64 KB
  chunk_size_max: 2097152    # 2 MB
  chunk_size_default: 524288 # 512 KB
  max_concurrent_transfers: 5
  operation_log_size: 10000  # Max log entries before compaction

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30     # seconds
  connection_timeout: 60      # seconds
  peers:                     # Manual peer list (optional)
    - "192.168.1.10:8080"
    - "192.168.1.11:8080"
    - "peer1.local:8080"
  # Alternative: comma-separated string via environment variable PEERS

security:
  key_rotation_interval: 86400  # 24 hours
  encryption_algorithm: "aes-256-gcm"

conflict:
  resolution_strategy: "intelligent_merge"  # Options: intelligent_merge, last_write_wins

compression:
  enabled: true
  file_size_threshold: 1048576  # 1 MB - compress files larger than this
  algorithm: "zstd"             # Options: zstd, lz4, gzip, none
  level: 3                      # Compression level (1-22 for zstd, 1-16 for lz4, 1-9 for gzip)
  chunk_compression: true       # Enable compression for individual chunks

observability:
  otel_endpoint: ""  # Optional OpenTelemetry collector endpoint
  log_level: "info"  # Options: debug, info, warn, error
  metrics_enabled: true
  tracing_enabled: true
```

#### 10.3.1 Configuration Validation Schema

```go
type Config struct {
    Sync        SyncConfig        `yaml:"sync"`
    Network     NetworkConfig     `yaml:"network"`
    Security    SecurityConfig    `yaml:"security"`
    Compression CompressionConfig `yaml:"compression"`
    Observability ObservabilityConfig `yaml:"observability"`
}

type SyncConfig struct {
    FolderPath            string `yaml:"folder_path" validate:"required"`
    ChunkSizeMin          int64  `yaml:"chunk_size_min" validate:"min=4096,max=1048576"`
    ChunkSizeMax          int64  `yaml:"chunk_size_max" validate:"min=1048576,max=10485760"`
    ChunkSizeDefault      int64  `yaml:"chunk_size_default"`
    MaxConcurrentTransfers int    `yaml:"max_concurrent_transfers" validate:"min=1,max=20"`
}

type NetworkConfig struct {
    Port          int `yaml:"port" validate:"min=1024,max=65535"`
    DiscoveryPort int `yaml:"discovery_port" validate:"min=1024,max=65535"`
}

type SecurityConfig struct {
    KeyRotationInterval int64 `yaml:"key_rotation_interval" validate:"min=3600,max=604800"`
}

type CompressionConfig struct {
    Enabled             bool   `yaml:"enabled"`
    FileSizeThreshold   int64  `yaml:"file_size_threshold" validate:"min=1024,max=1073741824"`
    Algorithm           string `yaml:"algorithm" validate:"oneof=zstd lz4 gzip none"`
    Level               int    `yaml:"level"`
    ChunkCompression    bool   `yaml:"chunk_compression"`
}

type ObservabilityConfig struct {
    OTELendpoint   string `yaml:"otel_endpoint"`
    LogLevel       string `yaml:"log_level" validate:"oneof=debug info warn error"`
    MetricsEnabled bool   `yaml:"metrics_enabled"`
    TracingEnabled bool   `yaml:"tracing_enabled"`
}

// Validation methods
func (c *Config) Validate() error {
    // Validate folder path exists and is writable
    if err := validateFolderPath(c.Sync.FolderPath); err != nil {
        return err
    }

    // Validate compression level based on algorithm
    if err := c.Compression.validateLevel(); err != nil {
        return err
    }

    // Validate chunk size relationships
    if c.Sync.ChunkSizeDefault < c.Sync.ChunkSizeMin ||
       c.Sync.ChunkSizeDefault > c.Sync.ChunkSizeMax {
        return errors.New("chunk_size_default must be between chunk_size_min and chunk_size_max")
    }

    return nil
}

func (cc *CompressionConfig) validateLevel() error {
    switch cc.Algorithm {
    case "zstd":
        if cc.Level < 1 || cc.Level > 22 {
            return errors.New("zstd level must be between 1 and 22")
        }
    case "lz4":
        if cc.Level < 1 || cc.Level > 16 {
            return errors.New("lz4 level must be between 1 and 16")
        }
    case "gzip":
        if cc.Level < 1 || cc.Level > 9 {
            return errors.New("gzip level must be between 1 and 9")
        }
    case "none":
        if cc.Level != 1 {
            return errors.New("none algorithm level must be 1")
        }
    default:
        return errors.New("invalid compression algorithm")
    }
    return nil
}
```

**Validation Rules**:
- Configuration loaded and validated on startup
- Invalid values fall back to defaults with warnings
- Critical validation failures prevent startup
- Configuration hot-reload for non-critical changes

## 10.4 Monitoring and Observability

### 10.4.1 OpenTelemetry Integration

**Metrics Collection**:
```typescript
// Core sync metrics
sync_operations_total: Counter  // Total operations processed
sync_operation_duration: Histogram  // Operation processing time
sync_file_transfer_bytes: Counter  // Bytes transferred per file
sync_active_transfers: Gauge  // Currently active file transfers

// Compression metrics
compression_files_compressed: Counter  // Total files compressed
compression_bytes_saved: Counter  // Total bytes saved through compression
compression_ratio: Histogram  // Compression ratio (compressed/original)
compression_operation_duration: Histogram  // Time spent compressing/decompressing

// Sync load balancing metrics
sync_file_requests_total: Counter  // Total file requests made by new peers
sync_files_per_peer: Histogram  // Number of files requested from each peer
sync_load_balance_efficiency: Gauge  // Load distribution efficiency (0-1)

// Network metrics
network_connections_active: Gauge  // Active peer connections
network_message_latency: Histogram  // End-to-end message latency
network_chunk_retransmissions: Counter  // Chunk retransmission count

// Resource metrics
resource_memory_usage: Gauge  // Memory usage (bytes)
resource_cpu_usage: Gauge  // CPU usage percentage
resource_disk_usage: Gauge  // Disk space usage
resource_bandwidth_usage: Gauge  // Network bandwidth usage

// Error metrics
error_operation_failures: Counter  // Failed operations by type
error_network_timeouts: Counter  // Network timeout errors
error_corruption_detected: Counter  // Data corruption incidents
```

**Distributed Tracing**:
- **Trace Operations**: Full request lifecycle from file change to sync completion
- **Span Hierarchy**: Discovery → Handshake → State Exchange → File Transfer
- **Context Propagation**: Correlation IDs across peer communications

**Logging Structure**:
```json
{
  "timestamp": "2025-01-01T12:00:00Z",
  "level": "info",
  "service": "p2p-sync",
  "peer_id": "peer-123",
  "operation_id": "op-456",
  "trace_id": "trace-789",
  "span_id": "span-101",
  "message": "File synchronization completed",
  "metadata": {
    "file_path": "/docs/readme.md",
    "file_size": 1024,
    "transfer_duration_ms": 150,
    "chunks_transferred": 1
  }
}
```

### 10.4.2 Alerting and Monitoring

**Health Checks**:
- **Peer Connectivity**: Monitor active peer connections
- **Database Health**: SQLite integrity and performance checks
- **File System Access**: Verify read/write permissions
- **Network Reachability**: Test peer communication

**Alert Conditions**:
- No peer connections for >5 minutes
- Database corruption detected
- File synchronization failures >10% of operations
- Memory usage >90% for >1 minute
- Disk space <1GB available

**Dashboards**:
- Real-time sync status across all peers
- Historical transfer rates and success rates
- Error rate trends and top failure modes
- Resource utilization graphs

## 11. Protocol Flow Examples

### 11.1 Initial Peer Connection

```
1. New Peer (A): Broadcast discovery message
2. Existing Peer (B): Respond with discovery_response
3. New Peer (A) → Existing Peer (B): handshake (A's public key)
4. Existing Peer (B) → New Peer (A): handshake_ack (B's public key)
5. Both: Derive session keys
6. New Peer (A) → Existing Peer (B): state_declaration (request sync)
7. Existing Peer (B) → New Peer (A): state_declaration (current state)
8. New Peer (A): Analyze state differences, identify missing files
9. New Peer (A): Request missing files from appropriate peers (load balanced)
10. Begin synchronized operations
```

**Load Balancing for New Peer Sync**:
- **Multi-Peer Scenario**: If multiple existing peers available:
  - New peer requests file manifests from all existing peers
  - Uses consistent hashing to assign files to source peers
  - Example: Files starting with A-M from Peer B, N-Z from Peer C
- **Single Source**: If only one peer available, request all missing files from it
- **Progressive Sync**: Prioritize small/critical files, sync large files in background

### 11.2 File Update with Chunking

```
1. Peer A: File modified, compute hash, split into chunks
2. Peer A → Peer B: sync_operation (create/update, metadata, chunk 0)
3. Peer A → Peer B: chunk (chunk 0 data)
4. Peer A → Peer B: chunk (chunk 1 data)
5. Peer A → Peer B: chunk (chunk 2 data, is_last=true)
6. Peer B: Receive chunks (may be out of order)
7. Peer B: Assemble chunks, verify file hash
8. Peer B → Peer A: operation_ack
9. Peer B: Write file atomically
```

### 11.3 Rename Detection

```
1. Peer A: File "doc.txt" deleted
   - Store: { file_id: "abc123", checksum: "def456", path: "doc.txt" }
   
2. Peer A: File "document.txt" created
   - Compute file_id: "abc123" (matches deleted file)
   - Compare checksum: "def456" == "def456" (matches)
   - Conclusion: RENAME operation
   
3. Peer A → Peer B: sync_operation (type: "rename", from_path: "doc.txt", path: "document.txt")
```

## 12. Testing and Validation

### 12.1 Test Scenarios

#### 12.1.1 Functional Tests

1. **Basic Sync**: Create/update/delete files, verify sync across all peers
2. **Large Files**: Transfer files >100MB with chunking and resume capability
3. **Concurrent Edits**: Two peers edit same file, verify 3-way merge resolution
4. **Rename Detection**: Rename files, verify detected as rename not delete+create
5. **Out-of-Order Chunks**: Simulate out-of-order delivery, verify correct assembly
6. **Key Exchange**: Verify secure ECDH key establishment with authentication
7. **Peer Discovery**: Verify UDP broadcast and mDNS discovery mechanisms
8. **Protocol Negotiation**: Test QUIC primary with TCP fallback
9. **File Compression**: Test compression of files >1MB threshold with various algorithms
10. **Compression Integrity**: Verify compressed files decompress correctly and match original
11. **New Peer Sync Load Balancing**: Test that new peer requests files from multiple peers without duplicates
12. **Sync Loop Prevention**: Verify that incoming file writes don't trigger outbound sync messages

#### 12.1.2 Failure Mode Tests

13. **Network Interruption**: Disconnect during transfer, verify resume from last chunk
14. **Peer Crash Recovery**: Kill process, restart, verify state reconstruction from log
15. **Database Corruption**: Simulate DB corruption, verify automatic recovery
16. **Disk Space Exhaustion**: Fill disk during transfer, verify graceful pause/resume
17. **Memory Pressure**: Simulate memory exhaustion, verify graceful degradation
18. **Network Partition**: Isolate peers, verify conflict-free operation and healing
19. **Permission Errors**: Test file access denied scenarios
20. **Corrupted Data**: Inject chunk corruption, verify retransmission and validation
21. **Clock Skew**: Test timestamp-based conflict resolution with unsynchronized clocks
22. **Byzantine Failure**: Simulate malicious peer behavior (if security model allows)
23. **Resource Exhaustion**: Test bandwidth throttling and CPU limiting
24. **Configuration Errors**: Invalid config files, missing permissions, etc.
25. **Compression Corruption**: Test decompression of corrupted compressed data
26. **Load Balancing Failure**: Test fallback when primary source peer becomes unavailable
27. **Sync Loop Prevention**: Verify remote writes don't trigger local change detection

### 12.2 Validation Criteria

- All file operations synchronized across peers
- No data loss during network interruptions
- Chunks assembled correctly regardless of order
- Renames correctly distinguished from edits
- Encryption prevents unauthorized access
- Discovery finds peers automatically
- Conflict resolution applies correctly
- Incoming file writes do not trigger outbound sync messages (no sync loops)

## 13. Future Enhancements

Potential improvements (out of scope for initial implementation):

1. **Merkle Trees**: Efficient bulk synchronization for large file sets
2. **Advanced Delta Sync**: Block-level delta encoding beyond rolling hash
3. **Selective Sync**: Sync only specific subdirectories or file types
4. **Compression**: Additional compression algorithms (LZ4, Brotli) beyond zstd
5. **Conflict UI**: Graphical user interface for manual conflict resolution
6. **Mobile Support**: iOS/Android peer implementations
7. **Cloud Integration**: Hybrid cloud-local synchronization
8. **Backup Integration**: Automatic backup to cloud storage providers
9. **Audit Logging**: Detailed operation audit trails for compliance
10. **Multi-Site Replication**: WAN-optimized synchronization across geographic sites

---

## 14. Docker Deployment

### 14.1 Container Architecture

**Base Image**: Ubuntu 22.04 LTS with Go 1.21+ runtime

**Multi-stage Build**:
```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o p2p-sync ./cmd/p2p-sync

# Runtime stage
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/p2p-sync /usr/local/bin/p2p-sync

EXPOSE 8080 8081
CMD ["p2p-sync"]
```

### 14.2 Volume Configuration

**Bidirectional Host Path Volume**:
```yaml
version: '3.8'
services:
  p2p-sync:
    build: .
    volumes:
      # Bidirectional sync folder - host changes propagate to container
      - ./sync-data:/app/sync:delegated
      # Persistent database storage
      - p2p-sync-db:/app/data
      # Configuration (read-only)
      - ./config:/app/config:ro
    ports:
      - "8080:8080"    # QUIC/TCP sync port
      - "8081:8081"    # UDP discovery port
    environment:
      - P2P_SYNC_FOLDER=/app/sync
      - P2P_CONFIG_PATH=/app/config/config.yaml
      - OTEL_ENDPOINT=http://otel-collector:4317
    networks:
      - p2p-network
    restart: unless-stopped

volumes:
  p2p-sync-db:

networks:
  p2p-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
```

### 14.3 Deployment Strategies

**Single Host Multi-Container**:
```yaml
# Multiple peer instances on same host
services:
  peer-alpha:
    # ... volume config
    environment:
      - PEER_ID=alpha
      - PEERS=peer-beta:8080,peer-gamma:8080

  peer-beta:
    # ... volume config
    environment:
      - PEER_ID=beta
      - PEERS=peer-alpha:8080,peer-gamma:8080

  peer-gamma:
    # ... volume config
    environment:
      - PEER_ID=gamma
      - PEERS=peer-alpha:8080,peer-beta:8080
```

**Docker Swarm Multi-Host**:
```yaml
version: '3.8'
services:
  p2p-sync:
    image: p2p-sync:latest
    volumes:
      - /host/sync/data:/app/sync:delegated
    ports:
      - "8080:8080"
      - "8081:8081/udp"
    deploy:
      mode: replicated
      replicas: 3
      placement:
        constraints:
          - node.role == worker
    networks:
      - p2p-overlay

networks:
  p2p-overlay:
    driver: overlay
```

### 14.4 Observability Integration

**OpenTelemetry Collector Sidecar**:
```yaml
services:
  p2p-sync:
    # ... main service config

  otel-collector:
    image: otel/opentelemetry-collector:latest
    volumes:
      - ./otel-config.yaml:/etc/otel/config.yaml
    ports:
      - "4317:4317"    # gRPC receiver
      - "4318:4318"    # HTTP receiver
    depends_on:
      - p2p-sync
```

**Docker Logging Drivers**:
```yaml
services:
  p2p-sync:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    # Or use fluentd for centralized logging
    # logging:
    #   driver: fluentd
    #   options:
    #     fluentd-address: "localhost:24224"
```

### 14.5 Security Considerations

**Container Security**:
- Non-root user execution
- Minimal base image (distroless possible)
- Read-only root filesystem where possible
- Seccomp and AppArmor profiles

**Network Security**:
- Internal Docker networks for peer communication
- TLS termination at reverse proxy for external access
- Network segmentation between sync and management traffic

## Appendix A: Message Type Reference

| Type | Direction | Purpose | Acknowledgment Required |
|------|-----------|---------|------------------------|
| `discovery` | Broadcast | Find peers | No |
| `discovery_response` | Unicast | Respond to discovery | No |
| `handshake` | Unicast | Key exchange | Yes |
| `handshake_ack` | Unicast | Key exchange ACK | No |
| `state_declaration` | Unicast | Declare peer state | Yes |
| `file_request` | Unicast | Request files for sync | Yes |
| `sync_operation` | Unicast/Broadcast | File operation | Yes |
| `chunk` | Unicast | File chunk data | Yes |
| `chunk_request` | Unicast | Request missing chunks | Yes |
| `operation_ack` | Unicast | Operation received | No |
| `chunk_ack` | Unicast | Chunk received | No |
| `heartbeat` | Unicast | Keep-alive | No |

## Appendix B: Error Codes

| Code | Description | Recovery Action |
|------|-------------|-----------------|
| `ERR_INVALID_HASH` | Chunk/file hash mismatch | Request retransmission |
| `ERR_MISSING_CHUNK` | Chunk not received | Request specific chunk |
| `ERR_CONNECTION_LOST` | Peer disconnected | Reconnect and resume |
| `ERR_INVALID_OPERATION` | Malformed operation | Log and skip |
| `ERR_ENCRYPTION_FAILED` | Encryption/decryption error | Re-establish keys |

---

**Document Version**: 2.5
**Last Updated**: November 2025
**Status**: Complete Specification with Go Implementation, Docker Deployment, Observability, Security, Compression, Pull-based Sync, and Sync Loop Prevention


