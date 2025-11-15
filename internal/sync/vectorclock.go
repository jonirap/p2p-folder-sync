package sync

import (
	"encoding/json"
	"fmt"
)

// VectorClock represents a vector clock for tracking causal relationships
type VectorClock map[string]int64

// NewVectorClock creates a new vector clock
func NewVectorClock() VectorClock {
	return make(VectorClock)
}

// Increment increments the clock for a specific peer
func (vc VectorClock) Increment(peerID string) {
	vc[peerID] = vc[peerID] + 1
}

// Get returns the clock value for a peer
func (vc VectorClock) Get(peerID string) int64 {
	return vc[peerID]
}

// Set sets the clock value for a peer
func (vc VectorClock) Set(peerID string, value int64) {
	vc[peerID] = value
}

// Merge merges another vector clock into this one (takes max of each peer's counter)
func (vc VectorClock) Merge(other VectorClock) {
	for peerID, value := range other {
		if vc[peerID] < value {
			vc[peerID] = value
		}
	}
}

// Compare compares two vector clocks
// Returns: -1 if vc < other, 0 if concurrent, 1 if vc > other
func (vc VectorClock) Compare(other VectorClock) int {
	less := false
	greater := false

	allPeers := make(map[string]bool)
	for peerID := range vc {
		allPeers[peerID] = true
	}
	for peerID := range other {
		allPeers[peerID] = true
	}

	for peerID := range allPeers {
		v1 := vc[peerID]
		v2 := other[peerID]
		if v1 < v2 {
			less = true
		} else if v1 > v2 {
			greater = true
		}
	}

	if less && !greater {
		return -1
	} else if greater && !less {
		return 1
	}
	return 0 // Concurrent
}

// IsConcurrent checks if two vector clocks are concurrent
func (vc VectorClock) IsConcurrent(other VectorClock) bool {
	return vc.Compare(other) == 0
}

// MarshalJSON implements json.Marshaler
func (vc VectorClock) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int64(vc))
}

// UnmarshalJSON implements json.Unmarshaler
func (vc *VectorClock) UnmarshalJSON(data []byte) error {
	var m map[string]int64
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to unmarshal vector clock: %w", err)
	}
	*vc = VectorClock(m)
	return nil
}

