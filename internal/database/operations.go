package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// LogEntry represents an operation log entry
type LogEntry struct {
	Sequence             int64
	OperationID          string
	Timestamp            time.Time
	OperationType        string
	PeerID               string
	VectorClock          map[string]int64
	Acknowledged         bool
	Persisted            bool
	FileID               *string
	Path                 string
	FromPath             *string
	Checksum             *string
	Size                 *int64
	Mtime                *time.Time
	Mode                 *uint32
	ChunkCount           int
	Data                 []byte
	Compressed           bool
	OriginalSize         *int64
	CompressionAlgorithm *string
}

// AppendOperation appends an operation to the log
func (d *DB) AppendOperation(entry *LogEntry) error {
	vcJSON, err := json.Marshal(entry.VectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	query := `
	INSERT INTO operations (
		operation_id, timestamp, operation_type, peer_id, vector_clock,
		acknowledged, persisted, file_id, path, from_path, checksum, size,
		mtime, mode, chunk_count, data, compressed, original_size, compression_algorithm
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var fileIDVal interface{}
	if entry.FileID != nil {
		fileIDVal = *entry.FileID
	}

	var fromPathVal interface{}
	if entry.FromPath != nil {
		fromPathVal = *entry.FromPath
	}

	var checksumVal interface{}
	if entry.Checksum != nil {
		checksumVal = *entry.Checksum
	}

	var sizeVal interface{}
	if entry.Size != nil {
		sizeVal = *entry.Size
	}

	var mtimeVal interface{}
	if entry.Mtime != nil {
		mtimeVal = entry.Mtime.Unix()
	}

	var modeVal interface{}
	if entry.Mode != nil {
		modeVal = *entry.Mode
	}

	var originalSizeVal interface{}
	if entry.OriginalSize != nil {
		originalSizeVal = *entry.OriginalSize
	}

	var compressionAlgoVal interface{}
	if entry.CompressionAlgorithm != nil {
		compressionAlgoVal = *entry.CompressionAlgorithm
	}

	acknowledged := 0
	if entry.Acknowledged {
		acknowledged = 1
	}

	persisted := 0
	if entry.Persisted {
		persisted = 1
	}

	compressed := 0
	if entry.Compressed {
		compressed = 1
	}

	_, err = d.db.Exec(query,
		entry.OperationID,
		entry.Timestamp.Unix(),
		entry.OperationType,
		entry.PeerID,
		string(vcJSON),
		acknowledged,
		persisted,
		fileIDVal,
		entry.Path,
		fromPathVal,
		checksumVal,
		sizeVal,
		mtimeVal,
		modeVal,
		entry.ChunkCount,
		entry.Data,
		compressed,
		originalSizeVal,
		compressionAlgoVal,
	)
	if err != nil {
		return fmt.Errorf("failed to append operation: %w", err)
	}

	return nil
}

// MarkOperationAcknowledged marks an operation as acknowledged
func (d *DB) MarkOperationAcknowledged(operationID string) error {
	query := `UPDATE operations SET acknowledged = 1 WHERE operation_id = ?`
	_, err := d.db.Exec(query, operationID)
	if err != nil {
		return fmt.Errorf("failed to mark operation acknowledged: %w", err)
	}
	return nil
}

// MarkOperationPersisted marks an operation as persisted
func (d *DB) MarkOperationPersisted(operationID string) error {
	query := `UPDATE operations SET persisted = 1 WHERE operation_id = ?`
	_, err := d.db.Exec(query, operationID)
	if err != nil {
		return fmt.Errorf("failed to mark operation persisted: %w", err)
	}
	return nil
}

// GetUnacknowledgedOperations returns all unacknowledged operations
func (d *DB) GetUnacknowledgedOperations() ([]*LogEntry, error) {
	query := `
	SELECT sequence, operation_id, timestamp, operation_type, peer_id, vector_clock,
		acknowledged, persisted, file_id, path, from_path, checksum, size,
		mtime, mode, chunk_count, data, compressed, original_size, compression_algorithm
	FROM operations
	WHERE acknowledged = 0
	ORDER BY sequence ASC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query unacknowledged operations: %w", err)
	}
	defer rows.Close()

	return d.scanLogEntries(rows)
}

// GetOperationsByFileID returns all operations for a specific file
func (d *DB) GetOperationsByFileID(fileID string) ([]*LogEntry, error) {
	query := `
	SELECT sequence, operation_id, timestamp, operation_type, peer_id, vector_clock,
		acknowledged, persisted, file_id, path, from_path, checksum, size,
		mtime, mode, chunk_count, data, compressed, original_size, compression_algorithm
	FROM operations
	WHERE file_id = ?
	ORDER BY sequence ASC
	`

	rows, err := d.db.Query(query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to query operations by file_id: %w", err)
	}
	defer rows.Close()

	return d.scanLogEntries(rows)
}

// GetAllOperations returns all operations in the log
func (d *DB) GetAllOperations() ([]*LogEntry, error) {
	query := `
	SELECT sequence, operation_id, timestamp, operation_type, peer_id, vector_clock,
		acknowledged, persisted, file_id, path, from_path, checksum, size,
		mtime, mode, chunk_count, data, compressed, original_size, compression_algorithm
	FROM operations
	ORDER BY sequence ASC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all operations: %w", err)
	}
	defer rows.Close()

	return d.scanLogEntries(rows)
}

// scanLogEntries scans rows into LogEntry structs
func (d *DB) scanLogEntries(rows *sql.Rows) ([]*LogEntry, error) {
	var entries []*LogEntry

	for rows.Next() {
		var entry LogEntry
		var timestampUnix float64
		var vcJSON string
		var acknowledged, persisted, compressed int
		var fileIDVal, fromPathVal, checksumVal sql.NullString
		var sizeVal, mtimeUnix sql.NullFloat64
		var modeVal sql.NullInt64
		var originalSizeVal sql.NullInt64
		var compressionAlgoVal sql.NullString

		if err := rows.Scan(
			&entry.Sequence,
			&entry.OperationID,
			&timestampUnix,
			&entry.OperationType,
			&entry.PeerID,
			&vcJSON,
			&acknowledged,
			&persisted,
			&fileIDVal,
			&entry.Path,
			&fromPathVal,
			&checksumVal,
			&sizeVal,
			&mtimeUnix,
			&modeVal,
			&entry.ChunkCount,
			&entry.Data,
			&compressed,
			&originalSizeVal,
			&compressionAlgoVal,
		); err != nil {
			return nil, fmt.Errorf("failed to scan log entry: %w", err)
		}

		entry.Timestamp = time.Unix(int64(timestampUnix), 0)
		entry.Acknowledged = acknowledged != 0
		entry.Persisted = persisted != 0
		entry.Compressed = compressed != 0

		if fileIDVal.Valid {
			entry.FileID = &fileIDVal.String
		}

		if fromPathVal.Valid {
			entry.FromPath = &fromPathVal.String
		}

		if checksumVal.Valid {
			entry.Checksum = &checksumVal.String
		}

		if sizeVal.Valid {
			size := int64(sizeVal.Float64)
			entry.Size = &size
		}

		if mtimeUnix.Valid {
			mtime := time.Unix(int64(mtimeUnix.Float64), 0)
			entry.Mtime = &mtime
		}

		if modeVal.Valid {
			mode := uint32(modeVal.Int64)
			entry.Mode = &mode
		}

		if originalSizeVal.Valid {
			size := originalSizeVal.Int64
			entry.OriginalSize = &size
		}

		if compressionAlgoVal.Valid {
			algo := compressionAlgoVal.String
			entry.CompressionAlgorithm = &algo
		}

		if err := json.Unmarshal([]byte(vcJSON), &entry.VectorClock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log entries: %w", err)
	}

	return entries, nil
}

// CompactLog removes old acknowledged operations
func (d *DB) CompactLog(maxEntries int) error {
	// Count total entries
	var count int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&count); err != nil {
		return fmt.Errorf("failed to count operations: %w", err)
	}

	if count <= maxEntries {
		return nil // No compaction needed
	}

	// Delete oldest acknowledged entries
	query := `
	DELETE FROM operations
	WHERE sequence IN (
		SELECT sequence FROM operations
		WHERE acknowledged = 1
		ORDER BY sequence ASC
		LIMIT ?
	)
	`

	toDelete := count - maxEntries
	_, err := d.db.Exec(query, toDelete)
	if err != nil {
		return fmt.Errorf("failed to compact log: %w", err)
	}

	return nil
}
