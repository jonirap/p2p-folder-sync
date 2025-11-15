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
	// Simple merge: combine unique lines, mark conflicts
	seen := make(map[string]bool)
	var merged []string

	// Add lines from both, marking conflicts
	maxLen := len(lines1)
	if len(lines2) > maxLen {
		maxLen = len(lines2)
	}

	for i := 0; i < maxLen; i++ {
		line1 := ""
		line2 := ""

		if i < len(lines1) {
			line1 = lines1[i]
		}
		if i < len(lines2) {
			line2 = lines2[i]
		}

		if line1 == line2 {
			// Same line, add once
			if !seen[line1] {
				merged = append(merged, line1)
				seen[line1] = true
			}
		} else {
			// Different lines - add conflict marker
			if line1 != "" && !seen[line1] {
				merged = append(merged, "<<<<<<< branch1")
				merged = append(merged, line1)
				seen[line1] = true
			}
			if line2 != "" && !seen[line2] {
				merged = append(merged, "=======")
				merged = append(merged, line2)
				merged = append(merged, ">>>>>>> branch2")
				seen[line2] = true
			}
		}
	}

	return merged
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

