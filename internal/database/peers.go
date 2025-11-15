package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PeerInfo represents peer information stored in the database
type PeerInfo struct {
	PeerID         string
	Address        *string
	Port           *int
	PublicKey      *string
	Certificate    *string
	Capabilities   map[string]interface{}
	LastSeen       *time.Time
	ConnectionState string
	TrustLevel     string
	CreatedAt      time.Time
}

// InsertOrUpdatePeer inserts or updates a peer in the database
func (d *DB) InsertOrUpdatePeer(peer *PeerInfo) error {
	capabilitiesJSON := "{}"
	if peer.Capabilities != nil {
		capJSON, err := json.Marshal(peer.Capabilities)
		if err != nil {
			return fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		capabilitiesJSON = string(capJSON)
	}

	query := `
	INSERT INTO peers (
		peer_id, address, port, public_key, certificate, capabilities,
		last_seen, connection_state, trust_level, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
	ON CONFLICT(peer_id) DO UPDATE SET
		address = excluded.address,
		port = excluded.port,
		public_key = excluded.public_key,
		certificate = excluded.certificate,
		capabilities = excluded.capabilities,
		last_seen = excluded.last_seen,
		connection_state = excluded.connection_state,
		trust_level = excluded.trust_level
	`

	var addressVal, publicKeyVal, certVal interface{}
	if peer.Address != nil {
		addressVal = *peer.Address
	}
	if peer.PublicKey != nil {
		publicKeyVal = *peer.PublicKey
	}
	if peer.Certificate != nil {
		certVal = *peer.Certificate
	}

	var portVal interface{}
	if peer.Port != nil {
		portVal = *peer.Port
	}

	var lastSeenVal interface{}
	if peer.LastSeen != nil {
		lastSeenVal = peer.LastSeen.Unix()
	}

	_, err := d.db.Exec(query,
		peer.PeerID,
		addressVal,
		portVal,
		publicKeyVal,
		certVal,
		capabilitiesJSON,
		lastSeenVal,
		peer.ConnectionState,
		peer.TrustLevel,
	)
	if err != nil {
		return fmt.Errorf("failed to insert/update peer: %w", err)
	}

	return nil
}

// GetPeer retrieves a peer by peer_id
func (d *DB) GetPeer(peerID string) (*PeerInfo, error) {
	query := `
	SELECT peer_id, address, port, public_key, certificate, capabilities,
		last_seen, connection_state, trust_level, created_at
	FROM peers
	WHERE peer_id = ?
	`

	var peer PeerInfo
	var addressVal, publicKeyVal, certVal sql.NullString
	var portVal sql.NullInt64
	var capabilitiesJSON string
	var lastSeenUnix sql.NullFloat64
	var createdAtUnix float64

	err := d.db.QueryRow(query, peerID).Scan(
		&peer.PeerID,
		&addressVal,
		&portVal,
		&publicKeyVal,
		&certVal,
		&capabilitiesJSON,
		&lastSeenUnix,
		&peer.ConnectionState,
		&peer.TrustLevel,
		&createdAtUnix,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("peer not found: %s", peerID)
		}
		return nil, fmt.Errorf("failed to get peer: %w", err)
	}

	peer.CreatedAt = time.Unix(int64(createdAtUnix), 0)

	if addressVal.Valid {
		peer.Address = &addressVal.String
	}

	if portVal.Valid {
		port := int(portVal.Int64)
		peer.Port = &port
	}

	if publicKeyVal.Valid {
		peer.PublicKey = &publicKeyVal.String
	}

	if certVal.Valid {
		peer.Certificate = &certVal.String
	}

	if lastSeenUnix.Valid {
		lastSeen := time.Unix(int64(lastSeenUnix.Float64), 0)
		peer.LastSeen = &lastSeen
	}

	if err := json.Unmarshal([]byte(capabilitiesJSON), &peer.Capabilities); err != nil {
		peer.Capabilities = make(map[string]interface{})
	}

	return &peer, nil
}

// GetAllPeers returns all peers in the database
func (d *DB) GetAllPeers() ([]*PeerInfo, error) {
	query := `
	SELECT peer_id, address, port, public_key, certificate, capabilities,
		last_seen, connection_state, trust_level, created_at
	FROM peers
	ORDER BY last_seen DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query peers: %w", err)
	}
	defer rows.Close()

	var peers []*PeerInfo
	for rows.Next() {
		var peer PeerInfo
		var addressVal, publicKeyVal, certVal sql.NullString
		var portVal sql.NullInt64
		var capabilitiesJSON string
		var lastSeenUnix sql.NullFloat64
		var createdAtUnix float64

		if err := rows.Scan(
			&peer.PeerID,
			&addressVal,
			&portVal,
			&publicKeyVal,
			&certVal,
			&capabilitiesJSON,
			&lastSeenUnix,
			&peer.ConnectionState,
			&peer.TrustLevel,
			&createdAtUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan peer: %w", err)
		}

		peer.CreatedAt = time.Unix(int64(createdAtUnix), 0)

		if addressVal.Valid {
			peer.Address = &addressVal.String
		}

		if portVal.Valid {
			port := int(portVal.Int64)
			peer.Port = &port
		}

		if publicKeyVal.Valid {
			peer.PublicKey = &publicKeyVal.String
		}

		if certVal.Valid {
			peer.Certificate = &certVal.String
		}

		if lastSeenUnix.Valid {
			lastSeen := time.Unix(int64(lastSeenUnix.Float64), 0)
			peer.LastSeen = &lastSeen
		}

		if err := json.Unmarshal([]byte(capabilitiesJSON), &peer.Capabilities); err != nil {
			peer.Capabilities = make(map[string]interface{})
		}

		peers = append(peers, &peer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating peers: %w", err)
	}

	return peers, nil
}

// UpdatePeerLastSeen updates the last_seen timestamp for a peer
func (d *DB) UpdatePeerLastSeen(peerID string) error {
	query := `UPDATE peers SET last_seen = unixepoch() WHERE peer_id = ?`
	_, err := d.db.Exec(query, peerID)
	if err != nil {
		return fmt.Errorf("failed to update peer last_seen: %w", err)
	}
	return nil
}

// UpdatePeerConnectionState updates the connection state for a peer
func (d *DB) UpdatePeerConnectionState(peerID string, state string) error {
	query := `UPDATE peers SET connection_state = ? WHERE peer_id = ?`
	_, err := d.db.Exec(query, state, peerID)
	if err != nil {
		return fmt.Errorf("failed to update peer connection state: %w", err)
	}
	return nil
}


