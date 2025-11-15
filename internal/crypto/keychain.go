package crypto

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Keychain manages storage and retrieval of encryption keys
type Keychain struct {
	db *sql.DB
}

// NewKeychain creates a new keychain
func NewKeychain(dbPath string) (*Keychain, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("failed to open keychain: %w", err)
	}

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS keys (
		peer_id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL,
		session_key TEXT,
		session_key_expires_at REAL,
		created_at REAL DEFAULT (unixepoch()),
		updated_at REAL DEFAULT (unixepoch())
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create keychain schema: %w", err)
	}

	return &Keychain{db: db}, nil
}

// Close closes the keychain database
func (k *Keychain) Close() error {
	return k.db.Close()
}

// StorePeerKey stores a peer's public key
func (k *Keychain) StorePeerKey(peerID string, publicKey []byte) error {
	encodedKey := base64.StdEncoding.EncodeToString(publicKey)
	query := `
	INSERT INTO keys (peer_id, public_key, updated_at)
	VALUES (?, ?, unixepoch())
	ON CONFLICT(peer_id) DO UPDATE SET
		public_key = excluded.public_key,
		updated_at = unixepoch()
	`
	_, err := k.db.Exec(query, peerID, encodedKey)
	if err != nil {
		return fmt.Errorf("failed to store peer key: %w", err)
	}
	return nil
}

// GetPeerKey retrieves a peer's public key
func (k *Keychain) GetPeerKey(peerID string) ([]byte, error) {
	var encodedKey string
	query := `SELECT public_key FROM keys WHERE peer_id = ?`
	err := k.db.QueryRow(query, peerID).Scan(&encodedKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("peer key not found: %s", peerID)
		}
		return nil, fmt.Errorf("failed to get peer key: %w", err)
	}

	publicKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode peer key: %w", err)
	}
	return publicKey, nil
}

// StoreSessionKey stores a session key for a peer
func (k *Keychain) StoreSessionKey(peerID string, sessionKey []byte, expiresAt time.Time) error {
	encodedKey := base64.StdEncoding.EncodeToString(sessionKey)
	query := `
	UPDATE keys
	SET session_key = ?, session_key_expires_at = ?, updated_at = unixepoch()
	WHERE peer_id = ?
	`
	_, err := k.db.Exec(query, encodedKey, expiresAt.Unix(), peerID)
	if err != nil {
		return fmt.Errorf("failed to store session key: %w", err)
	}
	return nil
}

// GetSessionKey retrieves a session key for a peer
func (k *Keychain) GetSessionKey(peerID string) ([]byte, *time.Time, error) {
	var encodedKey string
	var expiresAtUnix sql.NullFloat64
	query := `SELECT session_key, session_key_expires_at FROM keys WHERE peer_id = ?`
	err := k.db.QueryRow(query, peerID).Scan(&encodedKey, &expiresAtUnix)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("session key not found: %s", peerID)
		}
		return nil, nil, fmt.Errorf("failed to get session key: %w", err)
	}

	if encodedKey == "" {
		return nil, nil, fmt.Errorf("session key not set for peer: %s", peerID)
	}

	sessionKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode session key: %w", err)
	}

	var expiresAt *time.Time
	if expiresAtUnix.Valid {
		exp := time.Unix(int64(expiresAtUnix.Float64), 0)
		expiresAt = &exp
	}

	return sessionKey, expiresAt, nil
}

// IsSessionKeyExpired checks if a session key is expired
func (k *Keychain) IsSessionKeyExpired(peerID string) (bool, error) {
	var expiresAtUnix sql.NullFloat64
	query := `SELECT session_key_expires_at FROM keys WHERE peer_id = ?`
	err := k.db.QueryRow(query, peerID).Scan(&expiresAtUnix)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // Consider expired if peer not found
		}
		return true, fmt.Errorf("failed to check session key expiry: %w", err)
	}

	if !expiresAtUnix.Valid {
		return true, nil // No expiry set, consider expired
	}

	expiresAt := time.Unix(int64(expiresAtUnix.Float64), 0)
	return time.Now().After(expiresAt), nil
}

// RotateSessionKey marks a session key as expired (forces rotation)
func (k *Keychain) RotateSessionKey(peerID string) error {
	query := `UPDATE keys SET session_key = NULL, session_key_expires_at = NULL WHERE peer_id = ?`
	_, err := k.db.Exec(query, peerID)
	if err != nil {
		return fmt.Errorf("failed to rotate session key: %w", err)
	}
	return nil
}

