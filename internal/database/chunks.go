package database

import (
	"database/sql"
	"fmt"
	"time"
)

// ChunkInfo represents chunk information stored in the database
type ChunkInfo struct {
	FileID     string
	ChunkID    int
	ChunkHash  string
	Offset     int64
	Length     int64
	Received   bool
	ReceivedAt *time.Time
}

// InsertOrUpdateChunk inserts or updates a chunk in the database
func (d *DB) InsertOrUpdateChunk(chunk *ChunkInfo) error {
	query := `
	INSERT INTO chunks (
		file_id, chunk_id, chunk_hash, offset, length, received, received_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_id, chunk_id) DO UPDATE SET
		chunk_hash = excluded.chunk_hash,
		offset = excluded.offset,
		length = excluded.length,
		received = excluded.received,
		received_at = excluded.received_at
	`

	received := 0
	if chunk.Received {
		received = 1
	}

	var receivedAtVal interface{}
	if chunk.ReceivedAt != nil {
		receivedAtVal = chunk.ReceivedAt.Unix()
	}

	_, err := d.db.Exec(query,
		chunk.FileID,
		chunk.ChunkID,
		chunk.ChunkHash,
		chunk.Offset,
		chunk.Length,
		received,
		receivedAtVal,
	)
	if err != nil {
		return fmt.Errorf("failed to insert/update chunk: %w", err)
	}

	return nil
}

// GetChunksForFile returns all chunks for a specific file
func (d *DB) GetChunksForFile(fileID string) ([]*ChunkInfo, error) {
	query := `
	SELECT file_id, chunk_id, chunk_hash, offset, length, received, received_at
	FROM chunks
	WHERE file_id = ?
	ORDER BY chunk_id ASC
	`

	rows, err := d.db.Query(query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*ChunkInfo
	for rows.Next() {
		var chunk ChunkInfo
		var received int
		var receivedAtUnix sql.NullFloat64

		if err := rows.Scan(
			&chunk.FileID,
			&chunk.ChunkID,
			&chunk.ChunkHash,
			&chunk.Offset,
			&chunk.Length,
			&received,
			&receivedAtUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		chunk.Received = received != 0

		if receivedAtUnix.Valid {
			receivedAt := time.Unix(int64(receivedAtUnix.Float64), 0)
			chunk.ReceivedAt = &receivedAt
		}

		chunks = append(chunks, &chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chunks: %w", err)
	}

	return chunks, nil
}

// GetMissingChunks returns chunks that haven't been received yet
func (d *DB) GetMissingChunks(fileID string) ([]*ChunkInfo, error) {
	query := `
	SELECT file_id, chunk_id, chunk_hash, offset, length, received, received_at
	FROM chunks
	WHERE file_id = ? AND received = 0
	ORDER BY chunk_id ASC
	`

	rows, err := d.db.Query(query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to query missing chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*ChunkInfo
	for rows.Next() {
		var chunk ChunkInfo
		var received int
		var receivedAtUnix sql.NullFloat64

		if err := rows.Scan(
			&chunk.FileID,
			&chunk.ChunkID,
			&chunk.ChunkHash,
			&chunk.Offset,
			&chunk.Length,
			&received,
			&receivedAtUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		chunk.Received = false

		if receivedAtUnix.Valid {
			receivedAt := time.Unix(int64(receivedAtUnix.Float64), 0)
			chunk.ReceivedAt = &receivedAt
		}

		chunks = append(chunks, &chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chunks: %w", err)
	}

	return chunks, nil
}

// MarkChunkReceived marks a chunk as received
func (d *DB) MarkChunkReceived(fileID string, chunkID int) error {
	query := `
	UPDATE chunks
	SET received = 1, received_at = unixepoch()
	WHERE file_id = ? AND chunk_id = ?
	`
	_, err := d.db.Exec(query, fileID, chunkID)
	if err != nil {
		return fmt.Errorf("failed to mark chunk received: %w", err)
	}
	return nil
}

// DeleteChunksForFile deletes all chunks for a specific file
func (d *DB) DeleteChunksForFile(fileID string) error {
	query := `DELETE FROM chunks WHERE file_id = ?`
	_, err := d.db.Exec(query, fileID)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	return nil
}


