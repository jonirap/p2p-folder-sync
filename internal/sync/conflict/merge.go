package conflict

import (
	"strings"
)

// SyncOperation represents a sync operation for conflict resolution
type SyncOperation struct {
	ID        string
	Type      string
	Path      string
	FromPath  *string
	FileID    string
	Checksum  string
	Size      int64
	Timestamp int64
	PeerID    string
	Data      []byte
}

// ThreeWayMerge performs a 3-way merge for text files
func ThreeWayMerge(op1, op2 *SyncOperation) (*SyncOperation, error) {
	// For a true 3-way merge, we'd need the base version
	// For now, we'll do a simple 2-way merge with conflict markers

	data1 := string(op1.Data)
	data2 := string(op2.Data)

	// Simple line-based merge
	lines1 := strings.Split(data1, "\n")
	lines2 := strings.Split(data2, "\n")

	// Merge lines
	merged := mergeLines(lines1, lines2)
	mergedData := []byte(strings.Join(merged, "\n"))

	// Create merged operation (use op1 as base, update data)
	mergedOp := *op1
	mergedOp.Data = mergedData
	mergedOp.Checksum = "" // Will be recalculated
	mergedOp.Timestamp = maxInt64(op1.Timestamp, op2.Timestamp) + 1 // Newer timestamp

	return &mergedOp, nil
}

// mergeLines merges two sets of lines with conflict markers
func mergeLines(lines1, lines2 []string) []string {
	var merged []string

	// Find common prefix
	commonPrefix := findCommonPrefix(lines1, lines2)

	// Add common prefix
	merged = append(merged, commonPrefix...)

	// Find remaining parts
	remaining1 := lines1[len(commonPrefix):]
	remaining2 := lines2[len(commonPrefix):]

	// If one is empty, add the other
	if len(remaining1) == 0 && len(remaining2) > 0 {
		merged = append(merged, remaining2...)
		return merged
	}
	if len(remaining2) == 0 && len(remaining1) > 0 {
		merged = append(merged, remaining1...)
		return merged
	}

	// If both have remaining content, check for simple additions
	if len(remaining1) > 0 && len(remaining2) > 0 {
		// Check if one is just additions to the other
		if isAddition(remaining1, remaining2) {
			// remaining2 is addition to remaining1
			merged = append(merged, remaining1...)
			merged = append(merged, remaining2[len(remaining1):]...)
		} else if isAddition(remaining2, remaining1) {
			// remaining1 is addition to remaining2
			merged = append(merged, remaining2...)
			merged = append(merged, remaining1[len(remaining2):]...)
		} else {
			// Complex conflict - add conflict markers
			merged = append(merged, "<<<<<<< branch1")
			merged = append(merged, remaining1...)
			merged = append(merged, "=======")
			merged = append(merged, remaining2...)
			merged = append(merged, ">>>>>>> branch2")
		}
	}

	return merged
}

// findCommonPrefix finds the common prefix of two line arrays
func findCommonPrefix(lines1, lines2 []string) []string {
	minLen := len(lines1)
	if len(lines2) < minLen {
		minLen = len(lines2)
	}

	var common []string
	for i := 0; i < minLen; i++ {
		if lines1[i] == lines2[i] {
			common = append(common, lines1[i])
		} else {
			break
		}
	}

	return common
}

// isAddition checks if lines1 is an addition to lines2 (lines2 is prefix of lines1)
func isAddition(lines1, lines2 []string) bool {
	if len(lines1) < len(lines2) {
		return false
	}

	// Check if lines2 is prefix of lines1
	for i, line := range lines2 {
		if i >= len(lines1) || lines1[i] != line {
			return false
		}
	}

	return true
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

