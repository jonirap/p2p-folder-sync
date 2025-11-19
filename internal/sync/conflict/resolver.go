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
	// Need to have data to merge
	if len(op1.Data) > 0 && len(op2.Data) > 0 && isTextFile(op1.Data) && isTextFile(op2.Data) {
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
	// Perform a true 3-way merge with conflict detection
	return threeWayMergeWithBase(base, branch1, branch2)
}

// threeWayMergeWithBase performs a proper 3-way merge with conflict markers
func threeWayMergeWithBase(base, branch1, branch2 string) (string, error) {
	baseLines := strings.Split(base, "\n")
	lines1 := strings.Split(branch1, "\n")
	lines2 := strings.Split(branch2, "\n")

	// Check if either branch is identical to base
	if branch1 == base {
		return branch2, nil
	}
	if branch2 == base {
		return branch1, nil
	}
	if branch1 == branch2 {
		return branch1, nil
	}

	// Find common prefix and suffix with base
	commonPrefix := findCommonPrefixCount(baseLines, lines1, lines2)
	commonSuffix := findCommonSuffixCount(baseLines, lines1, lines2)

	// Extract middle sections (the changes)
	baseMiddle := extractMiddle(baseLines, commonPrefix, commonSuffix)
	middle1 := extractMiddle(lines1, commonPrefix, commonSuffix)
	middle2 := extractMiddle(lines2, commonPrefix, commonSuffix)

	// Check if changes conflict
	if !arraysEqual(baseMiddle, middle1) && !arraysEqual(baseMiddle, middle2) && !arraysEqual(middle1, middle2) {
		// Both sides changed - this is a conflict
		result := make([]string, 0)

		// Add common prefix
		result = append(result, baseLines[:commonPrefix]...)

		// Add conflict markers
		result = append(result, "<<<<<<< HEAD")
		result = append(result, middle1...)
		result = append(result, "=======")
		result = append(result, middle2...)
		result = append(result, ">>>>>>> remote")

		// Add common suffix
		if commonSuffix > 0 {
			result = append(result, baseLines[len(baseLines)-commonSuffix:]...)
		}

		return strings.Join(result, "\n"), nil
	}

	// No conflict - one side changed or both made same change
	if !arraysEqual(baseMiddle, middle1) {
		// Branch1 changed, use it
		return branch1, nil
	}
	// Branch2 changed (or both made same change), use it
	return branch2, nil
}

// findCommonPrefixCount finds count of common lines at the start
func findCommonPrefixCount(base, lines1, lines2 []string) int {
	minLen := min3(len(base), len(lines1), len(lines2))
	count := 0
	for i := 0; i < minLen; i++ {
		if base[i] == lines1[i] && base[i] == lines2[i] {
			count++
		} else {
			break
		}
	}
	return count
}

// findCommonSuffixCount finds count of common lines at the end
func findCommonSuffixCount(base, lines1, lines2 []string) int {
	minLen := min3(len(base), len(lines1), len(lines2))
	count := 0
	for i := 1; i <= minLen; i++ {
		if base[len(base)-i] == lines1[len(lines1)-i] && base[len(base)-i] == lines2[len(lines2)-i] {
			count++
		} else {
			break
		}
	}
	return count
}

// extractMiddle extracts the middle section excluding prefix and suffix
func extractMiddle(lines []string, prefixCount, suffixCount int) []string {
	if prefixCount+suffixCount >= len(lines) {
		return []string{}
	}
	return lines[prefixCount : len(lines)-suffixCount]
}

// arraysEqual checks if two string arrays are equal
func arraysEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// min3 returns the minimum of three integers
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
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
