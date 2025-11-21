# API Reference

Complete reference for P2P Folder Sync configuration, protocols, and interfaces.

## Table of Contents

1. [Configuration Reference](#configuration-reference)
2. [Environment Variables](#environment-variables)
3. [Command-Line Flags](#command-line-flags)
4. [Message Protocol](#message-protocol)
5. [Error Codes](#error-codes)
6. [Metrics Reference](#metrics-reference)
7. [File System Extended Attributes](#file-system-extended-attributes)

---

## Configuration Reference

### Configuration File Format

Configuration file must be in YAML format (typically `config.yaml`).

```yaml
sync:
  folder_path: string              # Required: Path to synchronized folder
  chunk_size_min: int64            # Minimum chunk size in bytes (64KB-1MB)
  chunk_size_max: int64            # Maximum chunk size in bytes (1MB-10MB)
  chunk_size_default: int64        # Default chunk size in bytes
  max_concurrent_transfers: int    # Max parallel file transfers (1-20)
  operation_log_size: int          # Max log entries before compaction

network:
  port: int                        # Main sync port (1024-65535)
  discovery_port: int              # UDP discovery port (1024-65535)
  heartbeat_interval: int          # Heartbeat interval in seconds
  connection_timeout: int          # Connection timeout in seconds
  peers: []string                  # Manual peer list (optional)

security:
  key_rotation_interval: int64     # Key rotation interval in seconds
  encryption_algorithm: string     # Encryption algorithm (aes-256-gcm)

compression:
  enabled: bool                    # Enable/disable compression
  file_size_threshold: int64       # Min file size to compress (bytes)
  algorithm: string                # zstd|lz4|gzip|none
  level: int                       # Compression level (algorithm-specific)
  chunk_compression: bool          # Enable per-chunk compression

observability:
  otel_endpoint: string            # OpenTelemetry collector endpoint
  log_level: string                # debug|info|warn|error
  metrics_enabled: bool            # Enable metrics collection
  tracing_enabled: bool            # Enable distributed tracing
```

### Sync Configuration

#### `sync.folder_path` (required)

- **Type**: `string`
- **Description**: Absolute path to the folder to be synchronized
- **Example**: `"/home/user/sync"` or `"/var/data/shared"`
- **Validation**: Must be an existing directory with read/write permissions

#### `sync.chunk_size_min`

- **Type**: `int64`
- **Default**: `65536` (64 KB)
- **Range**: `4096` to `1048576` (4 KB to 1 MB)
- **Description**: Minimum size for file chunks in bytes
- **Usage**: Smaller chunks increase overhead but improve granularity

#### `sync.chunk_size_max`

- **Type**: `int64`
- **Default**: `2097152` (2 MB)
- **Range**: `1048576` to `10485760` (1 MB to 10 MB)
- **Description**: Maximum size for file chunks in bytes
- **Usage**: Larger chunks reduce overhead but increase memory usage

#### `sync.chunk_size_default`

- **Type**: `int64`
- **Default**: `524288` (512 KB)
- **Range**: Must be between `chunk_size_min` and `chunk_size_max`
- **Description**: Default chunk size for splitting large files

#### `sync.max_concurrent_transfers`

- **Type**: `int`
- **Default**: `5`
- **Range**: `1` to `20`
- **Description**: Maximum number of concurrent file transfers
- **Performance Impact**: Higher values increase throughput but also memory/CPU usage

#### `sync.operation_log_size`

- **Type**: `int`
- **Default**: `10000`
- **Description**: Maximum number of log entries before automatic compaction
- **Usage**: Prevents unbounded log growth; compacts acknowledged operations

### Network Configuration

#### `network.port`

- **Type**: `int`
- **Default**: `8080`
- **Range**: `1024` to `65535`
- **Description**: TCP/QUIC port for peer-to-peer sync connections
- **Note**: Must not be in use by another service

#### `network.discovery_port`

- **Type**: `int`
- **Default**: `8081`
- **Range**: `1024` to `65535`
- **Description**: UDP port for peer discovery broadcasts
- **Firewall**: Must allow incoming UDP on this port

#### `network.heartbeat_interval`

- **Type**: `int`
- **Default**: `30`
- **Unit**: seconds
- **Description**: Interval between heartbeat messages to detect peer availability
- **Tuning**: Lower values detect failures faster but increase network traffic

#### `network.connection_timeout`

- **Type**: `int`
- **Default**: `60`
- **Unit**: seconds
- **Description**: Timeout for establishing new peer connections

#### `network.peers`

- **Type**: `[]string`
- **Default**: `[]` (empty, rely on auto-discovery)
- **Format**: `["hostname:port", "ip:port"]`
- **Example**:
  ```yaml
  peers:
    - "192.168.1.10:8080"
    - "192.168.1.11:8080"
    - "peer1.local:8080"
  ```
- **Description**: Manual peer list for cross-subnet or explicit connections

### Security Configuration

#### `security.key_rotation_interval`

- **Type**: `int64`
- **Default**: `86400` (24 hours)
- **Range**: `3600` to `604800` (1 hour to 7 days)
- **Unit**: seconds
- **Description**: Interval for rotating session encryption keys

#### `security.encryption_algorithm`

- **Type**: `string`
- **Default**: `"aes-256-gcm"`
- **Allowed Values**: `aes-256-gcm`
- **Description**: Encryption algorithm for data in transit
- **Note**: Currently only AES-256-GCM is supported

### Compression Configuration

#### `compression.enabled`

- **Type**: `bool`
- **Default**: `true`
- **Description**: Enable/disable automatic file compression

#### `compression.file_size_threshold`

- **Type**: `int64`
- **Default**: `1048576` (1 MB)
- **Range**: `1024` to `1073741824` (1 KB to 1 GB)
- **Unit**: bytes
- **Description**: Minimum file size to trigger compression
- **Tuning**: Lower threshold compresses more files; higher threshold reduces CPU usage

#### `compression.algorithm`

- **Type**: `string`
- **Default**: `"zstd"`
- **Allowed Values**: `zstd`, `lz4`, `gzip`, `none`
- **Description**: Compression algorithm to use
- **Characteristics**:
  - `zstd`: Best compression ratio, moderate speed (recommended)
  - `lz4`: Very fast, lower compression ratio
  - `gzip`: Compatible, moderate compression/speed
  - `none`: Disable compression

#### `compression.level`

- **Type**: `int`
- **Default**: `3`
- **Range**: Algorithm-dependent:
  - `zstd`: 1-22 (default: 3)
  - `lz4`: 1-16 (default: 1)
  - `gzip`: 1-9 (default: 6)
  - `none`: must be 1
- **Description**: Compression level (higher = better compression, slower)

#### `compression.chunk_compression`

- **Type**: `bool`
- **Default**: `true`
- **Description**: Enable compression for individual chunks in addition to file-level compression

### Observability Configuration

#### `observability.otel_endpoint`

- **Type**: `string`
- **Default**: `""` (disabled)
- **Format**: `"http://host:port"` or `"grpc://host:port"`
- **Example**: `"http://otel-collector:4317"`
- **Description**: OpenTelemetry collector endpoint for metrics and traces

#### `observability.log_level`

- **Type**: `string`
- **Default**: `"info"`
- **Allowed Values**: `debug`, `info`, `warn`, `error`
- **Description**: Logging verbosity level
- **Usage**:
  - `debug`: Verbose logging, includes internal state
  - `info`: Standard operational messages
  - `warn`: Warning messages only
  - `error`: Error messages only

#### `observability.metrics_enabled`

- **Type**: `bool`
- **Default**: `true`
- **Description**: Enable Prometheus metrics collection on port 9090

#### `observability.tracing_enabled`

- **Type**: `bool`
- **Default**: `true`
- **Description**: Enable OpenTelemetry distributed tracing
- **Requires**: `otel_endpoint` must be configured

---

## Environment Variables

Environment variables override configuration file values.

| Variable | Type | Description | Example |
|----------|------|-------------|---------|
| `P2P_SYNC_FOLDER` | string | Synchronized folder path | `/home/user/sync` |
| `P2P_CONFIG_PATH` | string | Configuration file path | `/etc/p2p-sync/config.yaml` |
| `P2P_PORT` | int | Main sync port | `8080` |
| `P2P_DISCOVERY_PORT` | int | Discovery port | `8081` |
| `P2P_TESTING_MODE` | bool | Enable testing mode | `true` or `false` |
| `PEERS` | string | Comma-separated peer list | `192.168.1.10:8080,peer.local:8080` |
| `OTEL_ENDPOINT` | string | OpenTelemetry endpoint | `http://localhost:4317` |
| `LOG_LEVEL` | string | Logging level | `debug` or `info` or `warn` or `error` |

### Environment Variable Precedence

Priority (highest to lowest):
1. Environment variables
2. Configuration file values
3. Default values

### Example Usage

```bash
# Override sync folder and log level
P2P_SYNC_FOLDER=/data/sync LOG_LEVEL=debug ./bin/p2p-sync

# Override network ports and enable testing mode
P2P_PORT=9090 P2P_DISCOVERY_PORT=9091 P2P_TESTING_MODE=true ./bin/p2p-sync

# Manual peer list
PEERS="192.168.1.10:8080,192.168.1.11:8080" ./bin/p2p-sync -config config.yaml
```

---

## Command-Line Flags

Command-line flags for the `p2p-sync` binary.

### Flags

#### `-config <path>`

- **Type**: `string`
- **Default**: `config/config.yaml`
- **Description**: Path to configuration file
- **Example**: `./p2p-sync -config /etc/p2p-sync/config.yaml`

#### `-version`

- **Type**: `bool`
- **Description**: Print version information and exit
- **Example**: `./p2p-sync -version`

#### `-help` or `-h`

- **Type**: `bool`
- **Description**: Display help message and exit
- **Example**: `./p2p-sync -help`

### Example Usage

```bash
# Start with custom config
./p2p-sync -config /path/to/config.yaml

# Print version
./p2p-sync -version

# Display help
./p2p-sync -help
```

---

## Message Protocol

### Message Types

All P2P communication uses structured messages with the following base format:

```go
type Message struct {
    ID            string      // Unique message identifier
    Type          string      // Message type (see below)
    Timestamp     int64       // Unix timestamp (milliseconds)
    SenderID      string      // Sender peer ID
    Payload       interface{} // Message-specific payload
    CorrelationID *string     // Optional correlation ID for request/response
}
```

### Message Type Reference

| Type | Direction | Purpose | Requires ACK | Payload Type |
|------|-----------|---------|--------------|--------------|
| `discovery` | Broadcast | Find peers on network | No | `DiscoveryMessage` |
| `discovery_response` | Unicast | Response to discovery | No | `DiscoveryResponseMessage` |
| `handshake` | Unicast | Initiate key exchange | Yes | `HandshakeMessage` |
| `handshake_ack` | Unicast | Acknowledge handshake | No | `HandshakeAckMessage` |
| `handshake_complete` | Unicast | Complete handshake | No | `HandshakeCompleteMessage` |
| `state_declaration` | Unicast | Declare peer state | Yes | `StateDeclarationMessage` |
| `file_request` | Unicast | Request specific files | Yes | `FileRequestMessage` |
| `sync_operation` | Unicast/Broadcast | File operation (create/update/delete/rename) | Yes | `LogEntryPayload` |
| `chunk` | Unicast | File chunk data | Yes | `ChunkMessage` |
| `chunk_request` | Unicast | Request missing chunks | Yes | `ChunkRequestMessage` |
| `operation_ack` | Unicast | Acknowledge operation | No | `OperationAckMessage` |
| `chunk_ack` | Unicast | Acknowledge chunk | No | `ChunkAckMessage` |
| `heartbeat` | Unicast | Keep-alive signal | No | `HeartbeatMessage` |

### Discovery Messages

#### DiscoveryMessage

```go
type DiscoveryMessage struct {
    PeerID       string            // Peer identifier
    Port         int               // Listening port
    Capabilities PeerCapabilities  // Peer capabilities
    Version      string            // Protocol version
}

type PeerCapabilities struct {
    Encryption  bool  // Supports encryption
    Compression bool  // Supports compression
    Chunking    bool  // Supports chunking
}
```

### State Sync Messages

#### StateDeclarationMessage

```go
type StateDeclarationMessage struct {
    PeerID            string                 // Peer identifier
    VectorClock       map[string]int64       // Vector clock state
    FileManifest      []FileManifestEntry    // List of files
    PendingOperations []LogEntry             // Unacknowledged operations
}

type FileManifestEntry struct {
    FileID         string  // Stable file identifier
    Path           string  // File path
    Hash           string  // Content hash (BLAKE3)
    Size           int64   // File size in bytes
    Mtime          int64   // Modification time (Unix timestamp)
    LastModifiedBy string  // Peer ID that last modified
}
```

#### FileRequestMessage

```go
type FileRequestMessage struct {
    RequestedFiles   []RequestedFile   // Files to request
    PeerCapabilities PeerCapabilities  // Requesting peer capabilities
}

type RequestedFile struct {
    FileID   string  // File identifier
    Priority string  // "high" | "normal" | "low"
}
```

### Sync Operation Messages

#### LogEntryPayload (SyncOperation)

```go
type LogEntryPayload struct {
    OperationID          string           // Unique operation ID
    Timestamp            int64            // Operation timestamp
    Type                 string           // "create" | "update" | "delete" | "rename" | "mkdir" | "rmdir"
    PeerID               string           // Origin peer ID
    Path                 string           // Current file path
    FromPath             *string          // Original path (for rename)
    FileID               *string          // Stable file identifier
    Checksum             *string          // File content hash
    Size                 *int64           // File size
    Mtime                *int64           // Modification time
    Mode                 *uint32          // POSIX permissions
    Data                 []byte           // File content (small files only)
    VectorClock          map[string]int64 // Vector clock
    Compressed           *bool            // Is data compressed?
    OriginalSize         *int64           // Original uncompressed size
    CompressionAlgorithm *string          // "zstd" | "lz4" | "gzip"
}
```

### Chunk Messages

#### ChunkMessage

```go
type ChunkMessage struct {
    FileID      string  // File identifier
    FileHash    string  // Full file hash for verification
    ChunkID     int     // Sequential chunk number (0, 1, 2, ...)
    TotalChunks int     // Total number of chunks
    Offset      int64   // Byte offset in original file
    Length      int64   // Chunk size in bytes
    ChunkHash   string  // BLAKE3 hash of chunk data
    Data        []byte  // Encrypted chunk data
    IsLast      bool    // Is this the last chunk?
}
```

#### ChunkRequestMessage

```go
type ChunkRequestMessage struct {
    FileID         string  // File identifier
    RequestedChunks []int  // Chunk IDs to request
}
```

### Acknowledgment Messages

#### OperationAckMessage

```go
type OperationAckMessage struct {
    OperationID string  // Operation being acknowledged
    Success     bool    // Operation succeeded?
    Error       string  // Error message (if !Success)
}
```

#### ChunkAckMessage

```go
type ChunkAckMessage struct {
    FileID   string  // File identifier
    ChunkID  int     // Chunk being acknowledged
    Success  bool    // Chunk received successfully?
    Error    string  // Error message (if !Success)
}
```

---

## Error Codes

### Application Error Codes

| Code | Name | Description | Recovery Action |
|------|------|-------------|-----------------|
| `ERR_INVALID_HASH` | Invalid Hash | Chunk/file hash mismatch | Request retransmission |
| `ERR_MISSING_CHUNK` | Missing Chunk | Expected chunk not received | Request specific chunk |
| `ERR_CONNECTION_LOST` | Connection Lost | Peer disconnected | Reconnect and resume |
| `ERR_INVALID_OPERATION` | Invalid Operation | Malformed operation message | Log and skip |
| `ERR_ENCRYPTION_FAILED` | Encryption Failed | Encryption/decryption error | Re-establish session keys |
| `ERR_DECOMPRESSION_FAILED` | Decompression Failed | Unable to decompress data | Request retransmission |
| `ERR_FILE_NOT_FOUND` | File Not Found | Requested file doesn't exist | Update manifest |
| `ERR_PERMISSION_DENIED` | Permission Denied | Insufficient file system permissions | Check permissions |
| `ERR_DISK_FULL` | Disk Full | No space available | Free disk space |
| `ERR_DATABASE_CORRUPT` | Database Corrupt | SQLite database corrupted | Restore from backup |

### System Exit Codes

| Code | Name | Description |
|------|------|-------------|
| `0` | Success | Normal exit |
| `1` | General Error | Unspecified error |
| `2` | Config Error | Configuration validation failed |
| `3` | Database Error | Database initialization failed |
| `4` | Network Error | Network binding failed |
| `5` | Permission Error | Insufficient permissions |

---

## Metrics Reference

All metrics are exposed in Prometheus format on `http://localhost:9090/metrics`.

### Sync Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `sync_operations_total` | Counter | `type`, `peer_id` | Total sync operations processed |
| `sync_operation_duration_seconds` | Histogram | `type` | Time to process operations |
| `sync_file_transfer_bytes` | Counter | `direction`, `peer_id` | Bytes transferred per peer |
| `sync_active_transfers` | Gauge | - | Currently active file transfers |
| `sync_operation_errors_total` | Counter | `type`, `error` | Failed operations by type |

### Compression Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `compression_files_compressed_total` | Counter | `algorithm` | Files compressed |
| `compression_bytes_saved_total` | Counter | `algorithm` | Bytes saved through compression |
| `compression_ratio` | Histogram | `algorithm` | Compression ratio (compressed/original) |
| `compression_duration_seconds` | Histogram | `operation`, `algorithm` | Compression/decompression time |

### Network Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `network_connections_active` | Gauge | - | Active peer connections |
| `network_message_latency_seconds` | Histogram | `type` | Message round-trip time |
| `network_chunk_retransmissions_total` | Counter | `peer_id` | Chunk retransmission count |
| `network_messages_sent_total` | Counter | `type`, `peer_id` | Messages sent |
| `network_messages_received_total` | Counter | `type`, `peer_id` | Messages received |

### Resource Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `resource_memory_bytes` | Gauge | `type` | Memory usage |
| `resource_cpu_usage_ratio` | Gauge | - | CPU usage (0-1) |
| `resource_disk_usage_bytes` | Gauge | `path` | Disk space used |
| `resource_bandwidth_bytes_per_second` | Gauge | `direction` | Network bandwidth |

### Error Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `error_operation_failures_total` | Counter | `type`, `error_code` | Failed operations |
| `error_network_timeouts_total` | Counter | `peer_id` | Network timeouts |
| `error_corruption_detected_total` | Counter | `type` | Data corruption events |

---

## File System Extended Attributes

Extended attributes store file metadata for rename detection.

### Attribute Keys

#### `user.p2p_sync.file_id`

- **Type**: Base64-encoded string
- **Size**: 44 bytes (32-byte BLAKE3 hash encoded)
- **Description**: Stable file identifier that persists across renames
- **Format**: `BLAKE3(first_64KB_of_content + initial_size + creation_time)`

### Platform Support

| Platform | Extended Attributes | Command |
|----------|---------------------|---------|
| Linux | Yes (xattr) | `getfattr`, `setfattr` |
| macOS | Yes (xattr) | `xattr` |
| Windows | No (uses metadata DB) | N/A |
| BSD | Yes (extattr) | `getextattr`, `setextattr` |

### Example Usage

```bash
# View file ID on Linux
getfattr -n user.p2p_sync.file_id /path/to/file

# View file ID on macOS
xattr -p user.p2p_sync.file_id /path/to/file
```

---

## Version Compatibility

### Protocol Version

Current protocol version: `1.0`

### Backwards Compatibility

- Minor version changes (1.x) maintain backwards compatibility
- Major version changes (x.0) may break compatibility
- Peers check protocol version during handshake

### Feature Negotiation

Peers negotiate capabilities during connection:
- Encryption support
- Compression algorithms
- Maximum chunk size
- Concurrent transfer limits

---

For detailed implementation examples, see [DEVELOPER.md](DEVELOPER.md).
For troubleshooting, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
