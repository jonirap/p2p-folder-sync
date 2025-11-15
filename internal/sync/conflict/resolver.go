package conflict

import (
	"path/filepath"
	"strings"
)

// VectorClock represents a vector clock
type VectorClock map[string]int64

// Resolver resolves conflicts between concurrent operations
type Resolver struct {
	strategy string
}

// NewResolver creates a new conflict resolver
func NewResolver(strategy string) *Resolver {
	return &Resolver{
		strategy: strategy,
	}
}

// ResolveConflict resolves a conflict between two operations
func (r *Resolver) ResolveConflict(op1, op2 *SyncOperation, vc1, vc2 VectorClock) (*SyncOperation, error) {
	// Check if operations are concurrent using vector clocks
	if !isConcurrent(vc1, vc2) {
		// Not concurrent, one causally precedes the other
		comparison := compareVectorClocks(vc1, vc2)
		if comparison < 0 {
			return op2, nil // op2 is newer
		}
		return op1, nil // op1 is newer
	}

	// Operations are concurrent - need conflict resolution
	if r.strategy == "intelligent_merge" {
		return r.intelligentMerge(op1, op2)
	}

	// Default to Last Write Wins
	return r.lastWriteWins(op1, op2), nil
}

// isConcurrent checks if two vector clocks are concurrent
func isConcurrent(vc1, vc2 VectorClock) bool {
	return compareVectorClocks(vc1, vc2) == 0
}

// compareVectorClocks compares two vector clocks
func compareVectorClocks(vc1, vc2 VectorClock) int {
	less := false
	greater := false

	allPeers := make(map[string]bool)
	for peerID := range vc1 {
		allPeers[peerID] = true
	}
	for peerID := range vc2 {
		allPeers[peerID] = true
	}

	for peerID := range allPeers {
		v1 := vc1[peerID]
		v2 := vc2[peerID]
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

// lastWriteWins resolves conflict using Last Write Wins strategy
func (r *Resolver) lastWriteWins(op1, op2 *SyncOperation) *SyncOperation {
	// Compare timestamps
	if op1.Timestamp > op2.Timestamp {
		return op1
	} else if op2.Timestamp > op1.Timestamp {
		return op2
	}

	// Same timestamp - use peer ID as tiebreaker
	if op1.PeerID < op2.PeerID {
		return op1
	}
	return op2
}

// intelligentMerge attempts intelligent merge (3-way merge for text files)
func (r *Resolver) intelligentMerge(op1, op2 *SyncOperation) (*SyncOperation, error) {
	// Check if files are text files (simple heuristic: check if data is text)
	if isTextFile(op1.Data) && isTextFile(op2.Data) {
		// Attempt 3-way merge
		merged, err := ThreeWayMerge(op1, op2)
		if err == nil {
			return merged, nil
		}
		// If merge fails, fall back to LWW
	}

	// Fall back to Last Write Wins for binary files or failed merges
	return r.lastWriteWins(op1, op2), nil
}

// SelectStrategy determines resolution strategy based on file type
func (r *Resolver) SelectStrategy(op *SyncOperation) (string, error) {
	// Check if file is text based on extension or content
	ext := strings.ToLower(filepath.Ext(op.Path))

	// Common text file extensions
	textExtensions := map[string]bool{
		".txt":  true,
		".md":   true,
		".go":   true,
		".py":   true,
		".js":   true,
		".ts":   true,
		".json": true,
		".yaml": true,
		".yml":  true,
		".xml":  true,
		".html": true,
		".css":  true,
	}

	if textExtensions[ext] {
		return "intelligent_merge", nil
	}

	// Check content if extension doesn't indicate text
	if isTextFile(op.Data) {
		return "intelligent_merge", nil
	}

	return "last_write_wins", nil
}

// ResolveLWW is a public wrapper for lastWriteWins
func (r *Resolver) ResolveLWW(op1, op2 *SyncOperation) (*SyncOperation, error) {
	return r.lastWriteWins(op1, op2), nil
}

// Resolve3Way performs a string-based 3-way merge for tests
func (r *Resolver) Resolve3Way(base, branch1, branch2 string) (string, error) {
	// Create temporary SyncOperation structs from strings
	op1 := &SyncOperation{
		Data: []byte(branch1),
	}
	op2 := &SyncOperation{
		Data: []byte(branch2),
	}

	// Use existing ThreeWayMerge function
	merged, err := ThreeWayMerge(op1, op2)
	if err != nil {
		return "", err
	}

	return string(merged.Data), nil
}

// ResolveLWWFallback performs string-based LWW for fallback scenarios
func (r *Resolver) ResolveLWWFallback(content1, content2 string, ts1, ts2 int64) string {
	// Compare timestamps
	if ts1 > ts2 {
		return content1
	} else if ts2 > ts1 {
		return content2
	}

	// Same timestamp - could use peer ID as tiebreaker, but for strings we just return content1
	return content1
}

// isTextFile checks if data appears to be text
func isTextFile(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// Simple heuristic: check if all bytes are printable or common whitespace
	for _, b := range data {
		if b < 32 && b != 9 && b != 10 && b != 13 {
			// Not a printable ASCII character or common whitespace
			return false
		}
	}
	return true
}
