# Test Inventory and Rationale

This document provides a comprehensive inventory of all tests in the P2P Folder Sync project, organized by type and module, with explanations of why each test is necessary.

## Table of Contents
- [Unit Tests](#unit-tests)
- [Integration Tests](#integration-tests)
- [System Tests](#system-tests)

---

## Unit Tests

Unit tests verify individual components in isolation, ensuring each module functions correctly independently.

### Cryptography Tests

#### [test/unit/crypto/crypto_test.go](test/unit/crypto/crypto_test.go)

1. **TestEncryptDecrypt**
   - **Why**: Verifies end-to-end encryption/decryption cycle with AES-256-GCM
   - **Necessity**: Core security requirement - data must be encrypted correctly and decrypt to original plaintext
   - **What it validates**: IV size, tag size, ciphertext generation, decryption accuracy

2. **TestDeriveSessionKey**
   - **Why**: Tests HKDF key derivation for secure session keys
   - **Necessity**: Session keys must be cryptographically secure and deterministic for same inputs
   - **What it validates**: Key size (32 bytes), deterministic output, HKDF correctness

3. **TestGenerateKeyPair**
   - **Why**: Validates X25519 key pair generation
   - **Necessity**: Public/private keys are fundamental to peer authentication
   - **What it validates**: Key sizes (32 bytes each), uniqueness of public vs private key

#### [test/unit/crypto/handshake_test.go](test/unit/crypto/handshake_test.go)

1. **TestHandshakeManager_Creation**
   - **Why**: Ensures handshake manager can be initialized with PSK
   - **Necessity**: Handshake manager is required for secure peer connections
   - **What it validates**: Non-nil manager creation

2. **TestHandshakeManager_FullHandshake**
   - **Why**: Tests complete 3-step handshake protocol between two peers
   - **Necessity**: Peers must establish authenticated, encrypted sessions
   - **What it validates**: Public key exchange, nonce/challenge generation, auth response verification, shared session key derivation

3. **TestHandshakeManager_InvalidAuthentication**
   - **Why**: Verifies handshake fails with mismatched PSKs
   - **Necessity**: Security requirement - unauthorized peers must be rejected
   - **What it validates**: Authentication failure detection, error message accuracy

4. **TestHandshakeManager_SessionRetrieval**
   - **Why**: Tests session key storage and retrieval
   - **Necessity**: Sessions must be retrievable for encrypted communication
   - **What it validates**: Error for non-existent sessions, session validation

5. **TestHandshakeManager_ChallengeResponse**
   - **Why**: Validates challenge-response authentication mechanism
   - **Necessity**: Prevents replay attacks and ensures peer authenticity
   - **What it validates**: HMAC-SHA256 response generation (32 bytes)

6. **TestHandshakeManager_RemoveSession**
   - **Why**: Tests session cleanup functionality
   - **Necessity**: Memory management and security (remove stale sessions)
   - **What it validates**: Safe removal, no panics for non-existent sessions

### Database Tests

#### [test/unit/database/database_test.go](test/unit/database/database_test.go)

1. **TestNewDB**
   - **Why**: Validates database initialization and schema creation
   - **Necessity**: Database is critical for file metadata persistence
   - **What it validates**: DB file creation, schema tables (≥5 tables expected)

2. **TestNewDB_InvalidPath**
   - **Why**: Tests error handling for invalid database paths
   - **Necessity**: Graceful failure prevents silent data loss
   - **What it validates**: Error returned for inaccessible paths

3. **TestFileOperations**
   - **Why**: Comprehensive test of CRUD operations on file metadata
   - **Necessity**: File tracking is core to sync functionality
   - **What it validates**: Insert, GetFile, GetFileByPath, GetAllFiles, vector clock persistence, error handling for non-existent files

### Filesystem Tests

#### [test/unit/filesystem/filesystem_test.go](test/unit/filesystem/filesystem_test.go)

1. **TestAtomicWriteFile**
   - **Why**: Validates atomic file write operations
   - **Necessity**: Prevents partial writes and file corruption
   - **What it validates**: Content integrity, file mode preservation

2. **TestFileExists**
   - **Why**: Tests file existence checking utility
   - **Necessity**: Basic filesystem operation used throughout codebase
   - **What it validates**: True for existing files, false for non-existent

#### [test/unit/filesystem/watcher_test.go](test/unit/filesystem/watcher_test.go)

1. **TestWatcher_IgnorePath**
   - **Why**: Verifies filesystem watcher ignores specified paths
   - **Necessity**: Prevents infinite sync loops when writing remote files
   - **What it validates**: No events generated for ignored paths

2. **TestWatcher_WatchPath_Reenable**
   - **Why**: Tests re-enabling watching after ignoring
   - **Necessity**: Path must generate events again after remote write completes
   - **What it validates**: Ignore → write → re-enable → events resume

3. **TestWatcher_RemoteWriteIgnored**
   - **Why**: Simulates remote file write scenario
   - **Necessity**: Remote operations must not trigger local sync events
   - **What it validates**: IgnorePath before write prevents event

4. **TestWatcher_IgnoreThenWatch**
   - **Why**: Tests full ignore → write → watch → modify sequence
   - **Necessity**: Common pattern in HandleIncomingFile
   - **What it validates**: Multi-step ignore/watch cycle

5. **TestWatcher_ConcurrentIgnoreWatch**
   - **Why**: Validates thread-safety of ignore map operations
   - **Necessity**: Multiple goroutines access watcher concurrently
   - **What it validates**: No race conditions, no panics

6. **TestWatcher_IgnorePathDuringRemoteOperation**
   - **Why**: Simulates HandleIncomingFile flow
   - **Necessity**: Ensures production code pattern works correctly
   - **What it validates**: Full remote operation sequence

7. **TestWatcher_MultipleIgnorePatterns**
   - **Why**: Tests independent ignoring of multiple paths
   - **Necessity**: System handles multiple concurrent remote writes
   - **What it validates**: Selective ignoring, correct event filtering

8. **TestWatcher_IgnoreNonExistentPath**
   - **Why**: Verifies pre-ignore before file creation works
   - **Necessity**: HandleIncomingFile ignores before creating file
   - **What it validates**: Pre-emptive ignoring

#### [test/unit/filesystem/rename_detector_test.go](test/unit/filesystem/rename_detector_test.go)

1. **TestRenameDetector_RecordDelete**
   - **Why**: Tests deletion recording for rename detection
   - **Necessity**: Delete events must be tracked for pattern matching
   - **What it validates**: FID, checksum, size, mtime, path storage

2. **TestRenameDetector_CheckRename_MatchingFIDAndChecksum**
   - **Why**: Validates rename detection (FID match + checksum match)
   - **Necessity**: Core rename detection logic per specification
   - **What it validates**: Returns true for rename, provides old path, removes entry

3. **TestRenameDetector_CheckRename_MatchingFIDDifferentChecksum**
   - **Why**: Tests edit detection (FID match + checksum mismatch)
   - **Necessity**: Distinguishes rename from delete+create of modified file
   - **What it validates**: Returns false for rename (edit scenario)

4. **TestRenameDetector_CheckRename_NoMatch**
   - **Why**: Tests create detection (no FID match)
   - **Necessity**: New files should not be detected as renames
   - **What it validates**: Returns false when no matching deletion

5. **TestRenameDetector_TTLExpiration**
   - **Why**: Verifies 5-second TTL for deletion records
   - **Necessity**: Prevents memory leaks, ensures freshness
   - **What it validates**: Entries expire after 5+ seconds

6. **TestRenameDetector_SizeMismatch**
   - **Why**: Tests size validation in rename detection
   - **Necessity**: File size must match for valid rename
   - **What it validates**: Rejects rename when size differs

7. **TestRenameDetector_Cleanup**
   - **Why**: Validates cleanup goroutine removes expired entries
   - **Necessity**: Automatic cleanup prevents unbounded growth
   - **What it validates**: Background cleanup works correctly

8. **TestRenameDetector_ConcurrentAccess**
   - **Why**: Tests thread-safety of rename detector
   - **Necessity**: Concurrent file operations require safe access
   - **What it validates**: No race conditions

9. **TestRenameDetector_MultipleEntries**
   - **Why**: Tests handling multiple pending deletions
   - **Necessity**: System tracks multiple potential renames
   - **What it validates**: Correct matching, selective removal

### Hashing Tests

#### [test/unit/hashing/hash_test.go](test/unit/hashing/hash_test.go)

1. **TestHash**
   - **Why**: Validates BLAKE3 hash generation
   - **Necessity**: File integrity verification depends on hashing
   - **What it validates**: 32-byte hash output

2. **TestHashString**
   - **Why**: Tests hex string encoding of hash
   - **Necessity**: Hashes stored as strings in database
   - **What it validates**: 64-character hex string

3. **TestHashConsistency**
   - **Why**: Verifies deterministic hashing
   - **Necessity**: Same input must produce same hash
   - **What it validates**: Consistency across multiple calls

4. **TestHash_KnownTestVectors**
   - **Why**: Validates against official BLAKE3 test vectors
   - **Necessity**: Ensures correct BLAKE3 implementation
   - **What it validates**: Known outputs for "", "hello world", "a", "abc"

5. **TestHash_IncrementalHashing**
   - **Why**: Tests streaming hash computation
   - **Necessity**: Large files hashed incrementally for memory efficiency
   - **What it validates**: HashReader matches direct Hash, chunk-by-chunk hashing

6. **TestHash_ErrorHandling**
   - **Why**: Tests error conditions
   - **Necessity**: Graceful handling of nil readers, error readers
   - **What it validates**: Appropriate errors returned

7. **TestHash_ConcurrentHashing**
   - **Why**: Validates thread-safety
   - **Necessity**: Multiple goroutines hash concurrently
   - **What it validates**: Consistent results across concurrent hashers

8. **TestHashString_Format**
   - **Why**: Verifies hex encoding format
   - **Necessity**: Consistent string format required
   - **What it validates**: 64 chars, lowercase, valid hex

#### [test/unit/hashing/fileid_test.go](test/unit/hashing/fileid_test.go)

1. **TestGenerateFileID_StandardFile**
   - **Why**: Tests FID generation for files <64KB
   - **Necessity**: FID must be stable and deterministic
   - **What it validates**: 64-char FID, deterministic, valid format

2. **TestGenerateFileID_LargeFile**
   - **Why**: Validates FID uses only first 64KB for large files
   - **Necessity**: Performance optimization for large files
   - **What it validates**: Same FID for same first 64KB+size

3. **TestGenerateFileID_EmptyFile**
   - **Why**: Tests empty file FID generation
   - **Necessity**: Empty files need unique, stable FIDs
   - **What it validates**: Uses creation_time + size + peer_id

4. **TestGenerateFileID_Consistency**
   - **Why**: Verifies FID consistency across multiple calls
   - **Necessity**: Critical for rename detection
   - **What it validates**: Identical FIDs across 5 calls

5. **TestGenerateFileID_CollisionResistance**
   - **Why**: Tests FID uniqueness for different files
   - **Necessity**: Prevent false rename detection
   - **What it validates**: Different files produce different FIDs

6. **TestGenerateFileIDFromData**
   - **Why**: Validates data-based FID generation
   - **Necessity**: Alternative to file-based when data already in memory
   - **What it validates**: Matches file-based FID

7. **TestValidateFileID**
   - **Why**: Tests FID validation function
   - **Necessity**: Input validation for FIDs
   - **What it validates**: Valid/invalid FID detection

8. **TestGenerateFileID_PersistenceAcrossRenames**
   - **Why**: Documents FID behavior during renames
   - **Necessity**: Understanding basic algorithm limitations
   - **What it validates**: FID from renamed file is valid

9. **TestGenerateFileID_ErrorHandling**
   - **Why**: Tests error conditions
   - **Necessity**: Graceful failure for invalid inputs
   - **What it validates**: Errors for non-existent, empty path, directories

### Chunking Tests

#### [test/unit/chunking/chunker_test.go](test/unit/chunking/chunker_test.go)

1. **TestChunkFile**
   - **Why**: Validates file chunking logic
   - **Necessity**: Large files must be split for network transfer
   - **What it validates**: Correct number of chunks, chunk metadata, data integrity

2. **TestChunkEmptyFile**
   - **Why**: Tests empty file handling
   - **Necessity**: Edge case that must be handled
   - **What it validates**: Single empty chunk, marked as last

3. **TestChunkFileReconstruction**
   - **Why**: Verifies file can be reconstructed from chunks
   - **Necessity**: Core file transfer functionality
   - **What it validates**: Reassembly produces original data

#### [test/unit/chunking/buffer_test.go](test/unit/chunking/buffer_test.go)

1. **TestChunkBuffer**
   - **Why**: Tests out-of-order chunk buffering
   - **Necessity**: Network may deliver chunks out of order
   - **What it validates**: Correct reordering, completion detection, data integrity

2. **TestChunkBufferMissingChunks**
   - **Why**: Validates missing chunk identification
   - **Necessity**: Retransmission requires knowing which chunks are missing
   - **What it validates**: Correct missing chunk list

#### [test/unit/chunking/manager_test.go](test/unit/chunking/manager_test.go)

1. **TestChunkManager_StartTransfer**
   - **Why**: Tests transfer initialization
   - **Necessity**: Transfers must be tracked
   - **What it validates**: Duplicate transfer prevention

2. **TestChunkManager_ReceiveChunksInOrder**
   - **Why**: Tests ordered chunk reception
   - **Necessity**: Happy path validation
   - **What it validates**: Assembly, completion detection, file reconstruction

3. **TestChunkManager_ReceiveChunksOutOfOrder**
   - **Why**: Tests out-of-order chunk handling
   - **Necessity**: Network delivers chunks unreliably
   - **What it validates**: Correct assembly regardless of order

4. **TestChunkManager_GetMissingChunks**
   - **Why**: Validates missing chunk tracking
   - **Necessity**: Retransmission logic depends on this
   - **What it validates**: Accurate missing chunk identification

5. **TestChunkManager_TransferStatus**
   - **Why**: Tests transfer progress tracking
   - **Necessity**: UI/logging needs transfer status
   - **What it validates**: Accurate received/total/complete counts

6. **TestChunkManager_RetransmissionCount**
   - **Why**: Validates retransmission tracking
   - **Necessity**: Monitoring and debugging
   - **What it validates**: Counter increment logic

7. **TestChunkManager_Cleanup**
   - **Why**: Tests transfer cleanup
   - **Necessity**: Memory management
   - **What it validates**: Transfer removal, status inaccessible after cleanup

### Compression Tests

#### [test/unit/compression/compressor_test.go](test/unit/compression/compressor_test.go)

1. **TestZstdCompression**
   - **Why**: Validates Zstandard compression/decompression
   - **Necessity**: Reduces network bandwidth usage
   - **What it validates**: Compression reduces size, decompression restores original

2. **TestGzipCompression**
   - **Why**: Tests gzip compression support
   - **Necessity**: Alternative compression algorithm
   - **What it validates**: Compression/decompression cycle

### State Management Tests

#### [test/unit/state/state_test.go](test/unit/state/state_test.go)

1. **TestReconciler**
   - **Why**: Tests file assignment to peers
   - **Necessity**: Load distribution across peers
   - **What it validates**: Round-robin distribution, all files assigned

2. **TestLoadBalancer**
   - **Why**: Validates consistent hashing for file distribution
   - **Necessity**: Deterministic file-to-peer mapping
   - **What it validates**: Consistent peer selection, distribution across peers

### Sync Logic Tests

#### [test/unit/sync/vectorclock_test.go](test/unit/sync/vectorclock_test.go)

1. **TestVectorClockIncrement**
   - **Why**: Tests vector clock increment logic
   - **Necessity**: Causality tracking for distributed sync
   - **What it validates**: Counter increments correctly

2. **TestVectorClockMerge**
   - **Why**: Validates vector clock merging
   - **Necessity**: Combining state from multiple peers
   - **What it validates**: Max value selection

3. **TestVectorClockCompare**
   - **Why**: Tests vector clock comparison
   - **Necessity**: Conflict detection requires comparison
   - **What it validates**: Correct ordering determination (-1, 0, 1)

#### [test/unit/sync/conflict/conflict_test.go](test/unit/sync/conflict/conflict_test.go)

1. **TestNewResolver**
   - **Why**: Tests conflict resolver creation
   - **Necessity**: Multiple strategies must be supported
   - **What it validates**: Non-nil resolver for each strategy

2. **TestResolver_ResolveLWW**
   - **Why**: Validates Last-Write-Wins resolution
   - **Necessity**: Simple conflict resolution strategy
   - **What it validates**: Newer timestamp wins

3. **TestResolver_Resolve3Way**
   - **Why**: Tests 3-way merge algorithm
   - **Necessity**: Intelligent text file merging
   - **What it validates**: Conflict markers in result

4. **TestResolver_ResolveLWWFallback**
   - **Why**: Tests timestamp-based fallback
   - **Necessity**: Non-text files need simple resolution
   - **What it validates**: Timestamp comparison logic

5. **TestResolver_SelectStrategy**
   - **Why**: Validates strategy selection
   - **Necessity**: Different file types need different strategies
   - **What it validates**: Correct strategy for file type

#### [internal/sync/conflict/merge_test.go](internal/sync/conflict/merge_test.go)

1. **TestMergeLines**
   - **Why**: Tests line-based text merging
   - **Necessity**: 3-way merge implementation
   - **What it validates**: Additions merge, conflicts marked

### Network Tests

#### [test/unit/network/messages/messages_test.go](test/unit/network/messages/messages_test.go)

1. **TestNewMessage**
   - **Why**: Tests message creation
   - **Necessity**: Messages are core communication primitive
   - **What it validates**: Type, sender, payload preservation

2. **TestMessageEncodingDecoding**
   - **Why**: Validates JSON serialization
   - **Necessity**: Network transmission requires encoding
   - **What it validates**: Round-trip encode/decode preserves data

3. **TestMessageTypes**
   - **Why**: Tests all message type constants
   - **Necessity**: Type safety
   - **What it validates**: All types work correctly

4. **TestDiscoveryMessage**
   - **Why**: Validates discovery payload
   - **Necessity**: Peer discovery requires specific fields
   - **What it validates**: Payload structure

5. **TestMessagePayloadEncodingDecoding**
   - **Why**: Tests specific payload types
   - **Necessity**: Different message types have different payloads
   - **What it validates**: DiscoveryMessage, ChunkMessage encoding

#### [test/unit/network/transport/transport_test.go](test/unit/network/transport/transport_test.go)

1. **TestNewQUICTransport**
   - **Why**: Tests QUIC transport creation
   - **Necessity**: QUIC is primary protocol
   - **What it validates**: Non-nil transport

2. **TestNewTCPTransport**
   - **Why**: Tests TCP transport creation
   - **Necessity**: TCP fallback required
   - **What it validates**: Non-nil transport

3. **TestTransportFactory**
   - **Why**: Validates factory pattern
   - **Necessity**: Dynamic protocol selection
   - **What it validates**: Creates correct transport type

4. **TestTransportInterface**
   - **Why**: Tests interface compliance
   - **Necessity**: Type safety
   - **What it validates**: Both transports implement Transport interface

#### [test/unit/network/connection/connection_test.go](test/unit/network/connection/connection_test.go)

1. **TestNewConnectionManager**
   - **Why**: Tests connection manager creation
   - **Necessity**: Connection tracking required
   - **What it validates**: Non-nil manager

2. **TestConnectionManager_AddGetRemove**
   - **Why**: Validates CRUD operations on connections
   - **Necessity**: Connection lifecycle management
   - **What it validates**: Add, retrieve, remove connections

3. **TestConnectionManager_GetAllConnections**
   - **Why**: Tests batch retrieval
   - **Necessity**: Listing all connections
   - **What it validates**: Correct count

4. **TestConnectionManager_UpdateConnectionState**
   - **Why**: Validates state transitions
   - **Necessity**: Connection states track health
   - **What it validates**: State updates correctly

5. **TestConnectionManager_GetConnectedPeers**
   - **Why**: Tests filtering by state
   - **Necessity**: Only send to connected peers
   - **What it validates**: Filters correctly

6. **TestNewHeartbeatManager**
   - **Why**: Tests heartbeat manager creation
   - **Necessity**: Keep-alive functionality
   - **What it validates**: Non-nil manager

#### [test/unit/network/handler_test.go](test/unit/network/handler_test.go)

1. **TestNewNetworkMessageHandler**
   - **Why**: Tests handler creation
   - **Necessity**: Message routing
   - **What it validates**: Non-nil handler

2. **TestSetSyncEngine**
   - **Why**: Validates dependency injection
   - **Necessity**: Handler needs sync engine reference
   - **What it validates**: Engine can be set

3. **TestSetHeartbeatManager**
   - **Why**: Tests heartbeat manager injection
   - **Necessity**: Handler processes heartbeats
   - **What it validates**: Manager integration

4. **TestHandleMessage_Heartbeat**
   - **Why**: Validates heartbeat processing
   - **Necessity**: Connection liveness
   - **What it validates**: Heartbeat handler called

5. **TestHandleMessage_UnknownType**
   - **Why**: Tests error handling
   - **Necessity**: Robustness
   - **What it validates**: Error for unknown types

6. **TestHandleMessage_Chunk**
   - **Why**: Validates chunk message handling
   - **Necessity**: File transfer
   - **What it validates**: Chunk processing

7. **TestHandleMessage_Chunk_MultipleChunks**
   - **Why**: Tests multi-chunk file handling
   - **Necessity**: Large file transfers
   - **What it validates**: All chunks processed

8. **TestCompression**
   - **Why**: Tests compression integration
   - **Necessity**: Bandwidth optimization
   - **What it validates**: Zstd and gzip work in handler

#### [test/unit/network/messenger_test.go](test/unit/network/messenger_test.go)

1. **TestNewNetworkMessenger**
   - **Why**: Tests messenger creation
   - **Necessity**: Core messaging component
   - **What it validates**: Valid config creates messenger, invalid config fails

2. **TestSendFile_SmallFile**
   - **Why**: Validates small file sending
   - **Necessity**: Common case
   - **What it validates**: File sent successfully

3. **TestSendFile_PeerNotConnected**
   - **Why**: Tests error handling
   - **Necessity**: Robustness
   - **What it validates**: Error for disconnected peer

4. **TestSendFile_NoSessionKey**
   - **Why**: Tests security requirement
   - **Necessity**: Encryption requires session key
   - **What it validates**: Error when session key missing

5. **TestSendFile_WithCompression**
   - **Why**: Validates compression integration
   - **Necessity**: Large files should be compressed
   - **What it validates**: Compression metadata set

6. **TestBroadcastOperation_NoPeers**
   - **Why**: Tests edge case
   - **Necessity**: Should not fail with no peers
   - **What it validates**: No error, no messages sent

7. **TestBroadcastOperation_SkipsSelfPeer**
   - **Why**: Validates self-exclusion
   - **Necessity**: Don't send to self
   - **What it validates**: Self not in recipient list

8. **TestHandleMessage_Acknowledgment**
   - **Why**: Tests ACK handling
   - **Necessity**: Reliable delivery
   - **What it validates**: ACK processed correctly

9. **TestHandleMessage_UnknownSender**
   - **Why**: Tests security
   - **Necessity**: Reject unknown senders
   - **What it validates**: Error for unknown sender

10. **TestSetMessageHandler**
    - **Why**: Tests handler injection
    - **Necessity**: Delegation pattern
    - **What it validates**: Handler can be set

11. **TestConnectToPeer**
    - **Why**: Validates peer connection
    - **Necessity**: Establish connections
    - **What it validates**: Connection created, session established

12. **TestRequestStateSync**
    - **Why**: Tests state sync request
    - **Necessity**: Initial synchronization
    - **What it validates**: Sync request sent

13. **TestMessageRetry**
    - **Why**: Validates retry logic
    - **Necessity**: Network reliability
    - **What it validates**: Retries on failure

14. **TestAcknowledgmentTimeout**
    - **Why**: Tests timeout handling
    - **Necessity**: Detect failed deliveries
    - **What it validates**: Skipped (too long for unit test)

### Observability Tests

#### [test/unit/observability/observability_test.go](test/unit/observability/observability_test.go)

1. **TestLogger**
   - **Why**: Tests logger creation
   - **Necessity**: Logging infrastructure
   - **What it validates**: All log levels create loggers

2. **TestLoggerOutput**
   - **Why**: Validates log output format
   - **Necessity**: Structured logging
   - **What it validates**: Message and level in output

3. **TestLoggerLevelFiltering**
   - **Why**: Tests log level filtering
   - **Necessity**: Control verbosity
   - **What it validates**: Only logs at or above level appear

4. **TestLoggerContext**
   - **Why**: Validates context fields
   - **Necessity**: Distributed tracing
   - **What it validates**: WithPeerID, WithOperationID, WithTraceID

### Configuration Tests

#### [test/unit/config/config_test.go](test/unit/config/config_test.go)

1. **TestLoadConfig**
   - **Why**: Tests config file loading
   - **Necessity**: Configuration management
   - **What it validates**: Default config, YAML parsing, field population

2. **TestConfigValidate**
   - **Why**: Comprehensive validation testing
   - **Necessity**: Prevent invalid configurations
   - **What it validates**: Valid configs pass, invalid configs fail with appropriate errors (13 test cases)

### Transport Fallback Tests

#### [test/unit/transport/fallback_test.go](test/unit/transport/fallback_test.go)

1. **TestFallbackTransport_Creation**
   - **Why**: Tests fallback transport initialization
   - **Necessity**: QUIC/TCP fallback mechanism
   - **What it validates**: Non-nil transport

2. **TestFallbackTransport_GetActiveProtocol**
   - **Why**: Validates protocol tracking
   - **Necessity**: Know which protocol is active
   - **What it validates**: Default is QUIC

3. **TestFallbackTransport_GetPeerProtocol**
   - **Why**: Tests per-peer protocol tracking
   - **Necessity**: Different peers may use different protocols
   - **What it validates**: Unknown peers use default

4. **TestTransportFactory_Default**
   - **Why**: Tests factory default behavior
   - **Necessity**: Sensible defaults
   - **What it validates**: Empty string creates fallback

5. **TestTransportFactory_ExplicitQUIC** & **TestTransportFactory_ExplicitTCP**
   - **Why**: Validates explicit protocol selection
   - **Necessity**: Override default behavior
   - **What it validates**: Factory creates requested type

6. **TestFallbackTransport_SetMessageHandler**
   - **Why**: Tests handler integration
   - **Necessity**: Message routing
   - **What it validates**: Handler can be set

### Flow Control Tests

#### [test/unit/flowcontrol/rate_limiter_test.go](test/unit/flowcontrol/rate_limiter_test.go)

1. **TestRateLimiter_Creation**
   - **Why**: Tests rate limiter initialization
   - **Necessity**: Bandwidth management
   - **What it validates**: Non-nil limiter

2. **TestRateLimiter_BasicLimit**
   - **Why**: Validates burst consumption
   - **Necessity**: Token bucket algorithm
   - **What it validates**: Burst consumed quickly

3. **TestRateLimiter_RateEnforcement**
   - **Why**: Tests rate limiting
   - **Necessity**: Bandwidth throttling
   - **What it validates**: Waits for token refill

4. **TestRateLimiter_ContextCancellation**
   - **Why**: Validates cancellation support
   - **Necessity**: Graceful shutdown
   - **What it validates**: Respects context cancellation

5. **TestRateLimiter_DynamicRateChange**
   - **Why**: Tests runtime rate adjustment
   - **Necessity**: Adaptive bandwidth control
   - **What it validates**: SetRate/GetRate work

6. **TestFlowController_Creation**
   - **Why**: Tests flow controller initialization
   - **Necessity**: Multi-file transfer management
   - **What it validates**: Non-nil controller, stats initialization

7. **TestFlowController_TransferSlots**
   - **Why**: Validates concurrent transfer limiting
   - **Necessity**: Resource management
   - **What it validates**: Slot acquisition, release, blocking

8. **TestFlowController_PerFileLimit**
   - **Why**: Tests per-file rate limiting
   - **Necessity**: Fair bandwidth distribution
   - **What it validates**: Individual file rate limiting

9. **TestFlowController_Stats**
   - **Why**: Validates statistics tracking
   - **Necessity**: Monitoring
   - **What it validates**: Accurate stats

### Monitoring Tests

#### [test/unit/monitoring/monitoring_test.go](test/unit/monitoring/monitoring_test.go)

1. **TestNewMetrics** through **TestMetricsJSON** (16 tests)
   - **Why**: Comprehensive metrics testing
   - **Necessity**: Observability requires accurate metrics
   - **What it validates**:
     - Metric recording (sync ops, network, flow control, peers, conflicts, errors)
     - Concurrent access safety
     - JSON serialization
     - Snapshot independence
     - Monitoring server lifecycle

---

## Integration Tests

Integration tests verify that multiple components work together correctly.

### Basic Integration Tests

#### [test/integration/basic_test.go](test/integration/basic_test.go)

1. **TestConfigLoad**
   - **Why**: Tests config loading with actual files
   - **Necessity**: End-to-end config validation
   - **What it validates**: YAML parsing, field values

2. **TestDatabaseInit**
   - **Why**: Validates database initialization
   - **Necessity**: Database persistence
   - **What it validates**: DB creation, schema, empty state

### File ID Persistence Tests

#### [test/integration/fileid_persistence_test.go](test/integration/fileid_persistence_test.go)

1. **TestFileID_PersistsAcrossRenames**
   - **Why**: Critical rename detection test
   - **Necessity**: FIDs must survive renames for detection to work
   - **What it validates**: FID stability across rename operations

2. **TestFileID_PersistsAcrossRestarts**
   - **Why**: Tests persistence across process lifecycle
   - **Necessity**: FIDs must survive application restarts
   - **What it validates**: Database persistence, xattr persistence

3. **TestFileID_XattrFallback**
   - **Why**: Validates fallback mechanism
   - **Necessity**: Not all filesystems support xattr
   - **What it validates**: Database fallback when xattr unavailable

4. **TestFileID_PersistenceAcrossMultipleOperations**
   - **Why**: Complex operation sequence testing
   - **Necessity**: Real-world scenarios involve multiple operations
   - **What it validates**: FID stability through create, rename, modify, rename

### System Integration Tests

#### [test/integration/system_test.go](test/integration/system_test.go)

1. **TestFullApplicationLifecycle**
   - **Why**: End-to-end application startup/shutdown
   - **Necessity**: Validate complete system integration
   - **What it validates**: Config, DB, logger, sync engine, transport lifecycle

2. **TestFileSynchronizationLifecycle**
   - **Why**: Tests file sync from creation to deletion
   - **Necessity**: Core functionality validation
   - **What it validates**: File indexing, modification detection, deletion handling

3. **TestMultiplePeerSimulation**
   - **Why**: Simulates two-peer sync scenario
   - **Necessity**: P2P requires multi-peer testing
   - **What it validates**: Peer setup, file transfer, content verification

4. **TestConfigurationValidation**
   - **Why**: Comprehensive config validation
   - **Necessity**: Invalid configs must be rejected
   - **What it validates**: 5 valid/invalid config scenarios

5. **TestDatabaseMigration**
   - **Why**: Tests schema initialization
   - **Necessity**: Database upgrades
   - **What it validates**: Table creation, basic operations

6. **TestLargeFileHandling**
   - **Why**: Large file support validation
   - **Necessity**: System must handle 1MB+ files
   - **What it validates**: Chunking, indexing large files

7. **TestConcurrentFileOperations**
   - **Why**: Stress test with concurrent operations
   - **Necessity**: Real-world has concurrent activity
   - **What it validates**: Thread-safety, no data corruption

### Performance Tests

#### [test/integration/performance_test.go](test/integration/performance_test.go)

1. **TestPerformanceBenchmarks**
   - **Why**: Performance regression detection
   - **Necessity**: Monitor sync speed, throughput
   - **What it validates**: File sync times, concurrent sync, network throughput, memory usage, disk I/O

### Additional Integration Tests

- **test/integration/failure_test.go**: Failure scenarios
- **test/integration/edge_cases_test.go**: Edge case handling
- **test/integration/docker_system_test.go**: Docker-based system tests
- **test/integration/database_corruption_test.go**: Database resilience

---

## System Tests

System tests validate end-to-end functionality in realistic environments.

### P2P Synchronization Tests

#### [test/system/p2p_sync_test.go](test/system/p2p_sync_test.go)

1. **TestPeerToPeerFileSync**
   - **Why**: Critical end-to-end P2P test
   - **Necessity**: Validates complete sync flow between peers
   - **What it validates**: Filesystem events → sync operations → network transfer → remote file reception → loop prevention

### Conflict Resolution Tests

#### [test/system/conflict_resolution_test.go](test/system/conflict_resolution_test.go)

1. **TestConflictResolutionTextFiles**
   - **Why**: Tests 3-way merge in real scenario
   - **Necessity**: Concurrent edits happen in P2P systems
   - **What it validates**: Conflict detection, 3-way merge invocation, resolution propagation

### Rename Detection Tests

#### [test/system/rename_detection_test.go](test/system/rename_detection_test.go)

1. **TestRenameDetection_EndToEnd**
   - **Why**: End-to-end rename detection validation
   - **Necessity**: Rename detection is complex, multi-component feature
   - **What it validates**: Create → delete → create pattern = rename, FID preservation

### Additional System Tests

- **test/system/network_messages_test.go**: Message flow testing
- **test/system/load_balancing_test.go**: Load distribution
- **test/system/sync_loop_prevention_test.go**: Prevents infinite loops
- **test/system/network_resilience_test.go**: Network failure handling
- **test/system/multi_peer_test.go**: Multi-peer scenarios
- **test/system/integration_e2e_test.go**: Full E2E scenarios
- **test/system/operation_replay_test.go**: Operation replay prevention
- **test/system/encryption_test.go**: End-to-end encryption

---

## Test Organization Summary

### By Type

- **Unit Tests**: 80+ tests across 25+ files
- **Integration Tests**: 15+ tests across 8 files
- **System Tests**: 10+ tests across 10 files

### By Module

- **Cryptography**: 8 tests (encryption, handshake, key management)
- **Database**: 3 tests (CRUD, schema, error handling)
- **Filesystem**: 22 tests (watcher, rename detector, atomic operations)
- **Hashing**: 17 tests (BLAKE3, FID generation, validation)
- **Chunking**: 10 tests (chunker, buffer, manager)
- **Compression**: 2 tests (zstd, gzip)
- **State**: 2 tests (reconciler, load balancer)
- **Sync**: 8 tests (vector clock, conflict resolution, merging)
- **Network**: 30+ tests (messages, transport, connection, handler, messenger)
- **Observability**: 4 tests (logger, filtering, context)
- **Configuration**: 2 tests (loading, validation)
- **Flow Control**: 9 tests (rate limiter, flow controller)
- **Monitoring**: 16 tests (metrics, concurrency, serialization)

---

## Why Each Test Category Is Necessary

### Security Tests (Crypto, Handshake)
- **Why**: P2P system must be secure from eavesdropping and tampering
- **Critical for**: Authentication, encryption, session establishment
- **What breaks without them**: Vulnerable to MITM attacks, unauthorized access, data leaks

### Data Integrity Tests (Hashing, Chunking)
- **Why**: File corruption must be detected and prevented
- **Critical for**: File verification, chunk validation, deduplication
- **What breaks without them**: Silent data corruption, incorrect file reassembly

### Synchronization Tests (Sync, Conflict, Vector Clock)
- **Why**: Core P2P sync logic must handle concurrency correctly
- **Critical for**: Conflict resolution, causality tracking, operation ordering
- **What breaks without them**: Lost updates, incorrect conflict resolution, data inconsistency

### Network Tests (Transport, Messages, Connection)
- **Why**: Reliable communication is essential for P2P
- **Critical for**: Message delivery, connection management, protocol handling
- **What breaks without them**: Failed transfers, connection leaks, protocol errors

### Filesystem Tests (Watcher, Rename Detector)
- **Why**: Filesystem operations must be detected and handled correctly
- **Critical for**: Change detection, rename optimization, loop prevention
- **What breaks without them**: Infinite sync loops, missed changes, inefficient transfers

### Performance Tests (Flow Control, Rate Limiting, Benchmarks)
- **Why**: System must perform well under load
- **Critical for**: Bandwidth management, concurrent transfers, scalability
- **What breaks without them**: Resource exhaustion, poor user experience, system crashes

### Integration Tests
- **Why**: Components must work together seamlessly
- **Critical for**: End-to-end functionality, cross-module interactions
- **What breaks without them**: Integration bugs, incompatible interfaces, system failures

### System Tests
- **Why**: Real-world scenarios must work correctly
- **Critical for**: Complete workflows, edge cases, failure handling
- **What breaks without them**: Production failures, unhandled edge cases, poor reliability

---

## Test Quality Metrics

- **Code Coverage**: High coverage across all modules
- **Edge Cases**: Extensive testing of boundary conditions, empty inputs, concurrent access
- **Error Handling**: Comprehensive error path testing
- **Thread Safety**: Concurrent access tests for all shared state
- **Performance**: Benchmarks and stress tests included
- **Documentation**: Each test has clear purpose and validation criteria

---

## Conclusion

This test suite provides comprehensive coverage of the P2P Folder Sync system:

1. **Unit tests** ensure individual components work correctly in isolation
2. **Integration tests** verify components interact properly
3. **System tests** validate end-to-end functionality in realistic scenarios

The tests cover:
- ✅ Security (encryption, authentication, handshake)
- ✅ Data integrity (hashing, checksums, validation)
- ✅ Synchronization (conflict resolution, vector clocks, causality)
- ✅ Network communication (protocols, messages, connections)
- ✅ Filesystem operations (watching, rename detection, atomic writes)
- ✅ Performance (rate limiting, flow control, benchmarks)
- ✅ Observability (logging, metrics, monitoring)
- ✅ Configuration management
- ✅ Error handling and edge cases
- ✅ Concurrent access and thread safety

Every test serves a specific purpose in ensuring the system is **secure**, **reliable**, **performant**, and **correct**.
