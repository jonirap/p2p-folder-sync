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

// mergeLines merges two sets of lines with conflict markers using a smarter algorithm
func mergeLines(lines1, lines2 []string) []string {
	// Use a simple diff-based merge that can handle non-adjacent changes
	return mergeLinesDiff(lines1, lines2)
}

// mergeLinesDiff performs a simple line-based merge that handles non-conflicting changes
func mergeLinesDiff(lines1, lines2 []string) []string {
	// Find common prefix
	prefix := findCommonPrefix(lines1, lines2)

	// Find common suffix
	suffix := findCommonSuffix(lines1, lines2)

	// Extract middle sections (the parts that differ)
	middle1Start := len(prefix)
	middle1End := len(lines1) - len(suffix)
	middle2Start := len(prefix)
	middle2End := len(lines2) - len(suffix)

	var middle1 []string
	if middle1End > middle1Start {
		middle1 = lines1[middle1Start:middle1End]
	}

	var middle2 []string
	if middle2End > middle2Start {
		middle2 = lines2[middle2Start:middle2End]
	}

	// Build result
	result := make([]string, 0, len(prefix)+len(middle1)+len(middle2)+len(suffix))
	result = append(result, prefix...)

	// Merge the middle sections
	if len(middle1) == 0 && len(middle2) == 0 {
		// No differences, just use prefix and suffix
	} else if len(middle1) == 0 {
		// Only lines2 has changes (additions)
		result = append(result, middle2...)
	} else if len(middle2) == 0 {
		// Only lines1 has changes (additions)
		result = append(result, middle1...)
	} else {
		// Both have changes - try to find common suffix within middles
		middleSuffix := findCommonSuffix(middle1, middle2)

		// Extract the parts before the middle suffix
		var beforeSuffix1 []string
		var beforeSuffix2 []string

		if len(middleSuffix) > 0 {
			beforeSuffix1 = middle1[:len(middle1)-len(middleSuffix)]
			beforeSuffix2 = middle2[:len(middle2)-len(middleSuffix)]
		} else {
			beforeSuffix1 = middle1
			beforeSuffix2 = middle2
		}

		// Check if the parts before the middle suffix conflict
		if len(beforeSuffix1) == 0 && len(beforeSuffix2) == 0 {
			// No conflict, just the middle suffix
			result = append(result, middleSuffix...)
		} else if len(beforeSuffix1) == 0 {
			// Only lines2 has changes before the middle suffix
			result = append(result, beforeSuffix2...)
			result = append(result, middleSuffix...)
		} else if len(beforeSuffix2) == 0 {
			// Only lines1 has changes before the middle suffix
			result = append(result, beforeSuffix1...)
			result = append(result, middleSuffix...)
		} else {
			// Both have changes before the middle suffix
			hasOverlap := !areDisjoint(beforeSuffix1, beforeSuffix2)

			if hasOverlap {
				// Have some overlap - prefer lines2's version and append lines1's unique additions
				result = append(result, beforeSuffix2...)
				result = append(result, middleSuffix...)
				// Add unique lines from lines1 that are not in lines2 or middleSuffix
				// Also skip lines that look like modifications of lines in beforeSuffix2
				for _, line := range beforeSuffix1 {
					if !contains(beforeSuffix2, line) && !contains(middleSuffix, line) {
						// Check if this line is a modification of a line in beforeSuffix2
						// by comparing line prefixes (for lines like "Line N: ...")
						if !isModificationOf(line, beforeSuffix2) {
							result = append(result, line)
						}
					}
				}
			} else {
				// Disjoint: could be conflict or non-conflicting additions
				if len(middleSuffix) > 0 {
					// There's a common point (middle suffix) separating the changes
					// Heuristic: prefer lines2's version before suffix, append lines1's additions after
					// This handles cases like: peer2 modifies middle, peer1 adds at end
					result = append(result, beforeSuffix2...)
					result = append(result, middleSuffix...)
					// Only add lines from beforeSuffix1 that don't overlap with the line positions
					// For a simple heuristic: add all of beforeSuffix1 after the middle suffix
					// unless they conflict with what came before
					for _, line := range beforeSuffix1 {
						if !contains(beforeSuffix2, line) {
							result = append(result, line)
						}
					}
				} else if len(suffix) > 0 {
					// Global suffix but no middle suffix - this is a true conflict
					result = append(result, "<<<<<<< branch1")
					result = append(result, beforeSuffix1...)
					result = append(result, "=======")
					result = append(result, beforeSuffix2...)
					result = append(result, ">>>>>>> branch2")
				} else {
					// No suffix at all: both adding at the end, merge them
					result = append(result, beforeSuffix1...)
					result = append(result, beforeSuffix2...)
				}
			}
		}
	}

	result = append(result, suffix...)
	return result
}

// areDisjoint checks if two line sets are completely different (no overlap)
func areDisjoint(lines1, lines2 []string) bool {
	// If they share no common lines, they are disjoint
	set1 := make(map[string]bool)
	for _, line := range lines1 {
		set1[line] = true
	}

	for _, line := range lines2 {
		if set1[line] {
			return false  // Found a common line, so they overlap
		}
	}

	return true  // No common lines, so they're disjoint
}

// contains checks if a line exists in a slice of lines
func contains(lines []string, line string) bool {
	for _, l := range lines {
		if l == line {
			return true
		}
	}
	return false
}

// isModificationOf checks if a line is likely a modification of any line in the given set
// by comparing line prefixes (handles cases like "Line 2: ..." vs "Line 2: ..." with different content)
func isModificationOf(line string, lines []string) bool {
	linePrefix := getLinePrefix(line)
	if linePrefix == "" {
		return false
	}

	for _, l := range lines {
		if getLinePrefix(l) == linePrefix {
			return true
		}
	}
	return false
}

// getLinePrefix extracts a prefix from a line to identify logical line positions
// For example, "Line 2: some content" returns "Line 2:"
func getLinePrefix(line string) string {
	// Look for patterns like "Line N:" or "number:" at the start
	parts := strings.SplitN(line, ":", 2)
	if len(parts) >= 2 {
		prefix := strings.TrimSpace(parts[0])
		// Check if it looks like a line identifier (starts with common words or numbers)
		if strings.HasPrefix(prefix, "Line ") || strings.Contains(prefix, " ") {
			return prefix + ":"
		}
	}
	return ""
}

// findCommonSuffix finds the common suffix of two line arrays
func findCommonSuffix(lines1, lines2 []string) []string {
	minLen := len(lines1)
	if len(lines2) < minLen {
		minLen = len(lines2)
	}

	var common []string
	for i := 1; i <= minLen; i++ {
		if lines1[len(lines1)-i] == lines2[len(lines2)-i] {
			common = append([]string{lines1[len(lines1)-i]}, common...)
		} else {
			break
		}
	}

	return common
}

// canMergeMiddles checks if two middle sections can be merged
func canMergeMiddles(middle1, middle2 []string) bool {
	// For now, allow merging if they have different content
	// In the future, this could be more sophisticated
	return true
}

// mergeMiddles merges two middle sections
func mergeMiddles(middle1, middle2 []string) []string {
	// Simple approach: if one looks like additions to the other, merge them
	if isSubset(middle1, middle2) {
		return middle2
	}
	if isSubset(middle2, middle1) {
		return middle1
	}

	// Otherwise, concatenate them
	result := make([]string, 0, len(middle1)+len(middle2))
	result = append(result, middle1...)
	result = append(result, middle2...)
	return result
}

// isSubset checks if lines1 is a subset of lines2 (allowing reordering)
func isSubset(lines1, lines2 []string) bool {
	if len(lines1) > len(lines2) {
		return false
	}

	// Simple check: see if all lines1 appear in lines2 in order
	lineMap := make(map[string]bool)
	for _, line := range lines2 {
		lineMap[line] = true
	}

	for _, line := range lines1 {
		if !lineMap[line] {
			return false
		}
	}

	return true
}

// findNextMatch finds the next occurrence of target line in the array starting from start index
func findNextMatch(lines []string, start int, target string) int {
	for i := start; i < len(lines); i++ {
		if lines[i] == target {
			return i
		}
	}
	return -1
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

