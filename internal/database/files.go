package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// FileMetadata represents file metadata stored in the database
type FileMetadata struct {
	FileID               string
	Path                 string
	Checksum             string
	Size                 int64
	Mtime                time.Time
	Mode                 *uint32
	PeerID               string
	VectorClock          map[string]int64
	Compressed           bool
	OriginalSize         *int64
	CompressionAlgorithm *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// InsertFile inserts or updates a file in the database
func (d *DB) InsertFile(fm *FileMetadata) error {
	vcJSON, err := json.Marshal(fm.VectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	query := `
	INSERT INTO files (
		file_id, path, checksum, size, mtime, mode, peer_id, vector_clock,
		compressed, original_size, compression_algorithm, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
	ON CONFLICT(file_id) DO UPDATE SET
		path = excluded.path,
		checksum = excluded.checksum,
		size = excluded.size,
		mtime = excluded.mtime,
		mode = excluded.mode,
		peer_id = excluded.peer_id,
		vector_clock = excluded.vector_clock,
		compressed = excluded.compressed,
		original_size = excluded.original_size,
		compression_algorithm = excluded.compression_algorithm,
		updated_at = unixepoch()
	`

	var modeVal interface{}
	if fm.Mode != nil {
		modeVal = *fm.Mode
	}

	var originalSizeVal interface{}
	if fm.OriginalSize != nil {
		originalSizeVal = *fm.OriginalSize
	}

	var compressionAlgoVal interface{}
	if fm.CompressionAlgorithm != nil {
		compressionAlgoVal = *fm.CompressionAlgorithm
	}

	_, err = d.db.Exec(query,
		fm.FileID,
		fm.Path,
		fm.Checksum,
		fm.Size,
		fm.Mtime.Unix(),
		modeVal,
		fm.PeerID,
		string(vcJSON),
		fm.Compressed,
		originalSizeVal,
		compressionAlgoVal,
	)
	if err != nil {
		return fmt.Errorf("failed to insert file: %w", err)
	}

	return nil
}

// GetFile retrieves a file by file_id
func (d *DB) GetFile(fileID string) (*FileMetadata, error) {
	query := `
	SELECT file_id, path, checksum, size, mtime, mode, peer_id, vector_clock,
		compressed, original_size, compression_algorithm, created_at, updated_at
	FROM files
	WHERE file_id = ?
	`

	var fm FileMetadata
	var mtimeUnix float64
	var modeVal sql.NullInt64
	var vcJSON string
	var compressed int
	var originalSizeVal sql.NullInt64
	var compressionAlgoVal sql.NullString
	var createdAtUnix, updatedAtUnix float64

	err := d.db.QueryRow(query, fileID).Scan(
		&fm.FileID,
		&fm.Path,
		&fm.Checksum,
		&fm.Size,
		&mtimeUnix,
		&modeVal,
		&fm.PeerID,
		&vcJSON,
		&compressed,
		&originalSizeVal,
		&compressionAlgoVal,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found: %s", fileID)
		}
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	fm.Mtime = time.Unix(int64(mtimeUnix), 0)
	fm.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	fm.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
	fm.Compressed = compressed != 0

	if modeVal.Valid {
		mode := uint32(modeVal.Int64)
		fm.Mode = &mode
	}

	if originalSizeVal.Valid {
		size := originalSizeVal.Int64
		fm.OriginalSize = &size
	}

	if compressionAlgoVal.Valid {
		algo := compressionAlgoVal.String
		fm.CompressionAlgorithm = &algo
	}

	if err := json.Unmarshal([]byte(vcJSON), &fm.VectorClock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
	}

	return &fm, nil
}

// GetFileByPath retrieves a file by path
func (d *DB) GetFileByPath(path string) (*FileMetadata, error) {
	query := `
	SELECT file_id, path, checksum, size, mtime, mode, peer_id, vector_clock,
		compressed, original_size, compression_algorithm, created_at, updated_at
	FROM files
	WHERE path = ?
	ORDER BY updated_at DESC
	LIMIT 1
	`

	var fm FileMetadata
	var mtimeUnix float64
	var modeVal sql.NullInt64
	var vcJSON string
	var compressed int
	var originalSizeVal sql.NullInt64
	var compressionAlgoVal sql.NullString
	var createdAtUnix, updatedAtUnix float64

	err := d.db.QueryRow(query, path).Scan(
		&fm.FileID,
		&fm.Path,
		&fm.Checksum,
		&fm.Size,
		&mtimeUnix,
		&modeVal,
		&fm.PeerID,
		&vcJSON,
		&compressed,
		&originalSizeVal,
		&compressionAlgoVal,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to get file by path: %w", err)
	}

	fm.Mtime = time.Unix(int64(mtimeUnix), 0)
	fm.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	fm.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
	fm.Compressed = compressed != 0

	if modeVal.Valid {
		mode := uint32(modeVal.Int64)
		fm.Mode = &mode
	}

	if originalSizeVal.Valid {
		size := originalSizeVal.Int64
		fm.OriginalSize = &size
	}

	if compressionAlgoVal.Valid {
		algo := compressionAlgoVal.String
		fm.CompressionAlgorithm = &algo
	}

	if err := json.Unmarshal([]byte(vcJSON), &fm.VectorClock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
	}

	return &fm, nil
}

// GetFileByID retrieves a file by file ID
func (d *DB) GetFileByID(fileID string) (*FileMetadata, error) {
	query := `
	SELECT file_id, path, checksum, size, mtime, mode, peer_id, vector_clock,
		compressed, original_size, compression_algorithm, created_at, updated_at
	FROM files
	WHERE file_id = ?
	ORDER BY updated_at DESC
	LIMIT 1
	`

	var fm FileMetadata
	var mtimeUnix float64
	var modeVal sql.NullInt64
	var vcJSON string
	var compressed int
	var originalSizeVal sql.NullInt64
	var compressionAlgoVal sql.NullString
	var createdAtUnix, updatedAtUnix float64

	err := d.db.QueryRow(query, fileID).Scan(
		&fm.FileID, &fm.Path, &fm.Checksum, &fm.Size, &mtimeUnix, &modeVal,
		&fm.PeerID, &vcJSON, &compressed, &originalSizeVal, &compressionAlgoVal,
		&createdAtUnix, &updatedAtUnix,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found: %s", fileID)
		}
		return nil, fmt.Errorf("failed to get file by ID: %w", err)
	}

	fm.Mtime = time.Unix(int64(mtimeUnix), 0)
	fm.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	fm.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
	fm.Compressed = compressed != 0

	if modeVal.Valid {
		mode := uint32(modeVal.Int64)
		fm.Mode = &mode
	}

	if originalSizeVal.Valid {
		size := originalSizeVal.Int64
		fm.OriginalSize = &size
	}

	if compressionAlgoVal.Valid {
		algo := compressionAlgoVal.String
		fm.CompressionAlgorithm = &algo
	}

	if vcJSON != "" {
		if err := json.Unmarshal([]byte(vcJSON), &fm.VectorClock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
		}
	}

	return &fm, nil
}

// DeleteFile deletes a file from the database
func (d *DB) DeleteFile(fileID string) error {
	query := `DELETE FROM files WHERE file_id = ?`
	_, err := d.db.Exec(query, fileID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// GetAllFiles returns all files in the database
func (d *DB) GetAllFiles() ([]*FileMetadata, error) {
	query := `
	SELECT file_id, path, checksum, size, mtime, mode, peer_id, vector_clock,
		compressed, original_size, compression_algorithm, created_at, updated_at
	FROM files
	ORDER BY path
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []*FileMetadata
	for rows.Next() {
		var fm FileMetadata
		var mtimeUnix float64
		var modeVal sql.NullInt64
		var vcJSON string
		var compressed int
		var originalSizeVal sql.NullInt64
		var compressionAlgoVal sql.NullString
		var createdAtUnix, updatedAtUnix float64

		if err := rows.Scan(
			&fm.FileID,
			&fm.Path,
			&fm.Checksum,
			&fm.Size,
			&mtimeUnix,
			&modeVal,
			&fm.PeerID,
			&vcJSON,
			&compressed,
			&originalSizeVal,
			&compressionAlgoVal,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		fm.Mtime = time.Unix(int64(mtimeUnix), 0)
		fm.CreatedAt = time.Unix(int64(createdAtUnix), 0)
		fm.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
		fm.Compressed = compressed != 0

		if modeVal.Valid {
			mode := uint32(modeVal.Int64)
			fm.Mode = &mode
		}

		if originalSizeVal.Valid {
			size := originalSizeVal.Int64
			fm.OriginalSize = &size
		}

		if compressionAlgoVal.Valid {
			algo := compressionAlgoVal.String
			fm.CompressionAlgorithm = &algo
		}

		if err := json.Unmarshal([]byte(vcJSON), &fm.VectorClock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
		}

		files = append(files, &fm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating files: %w", err)
	}

	// Return empty slice instead of nil
	if files == nil {
		files = []*FileMetadata{}
	}

	return files, nil
}
