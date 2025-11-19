package conflict

import (
	"strings"
	"testing"
)

func TestMergeLines(t *testing.T) {
	tests := []struct {
		name     string
		lines1   []string
		lines2   []string
		expected string
	}{
		{
			name: "Additions at end should merge",
			lines1: []string{
				"Line 1: Common content",
				"Line 2: More common content",
				"Line 3: End of common content",
				"Line 4: Added by peer1",
				"Line 5: More peer1 content",
			},
			lines2: []string{
				"Line 1: Common content",
				"Line 2: Modified by peer2",
				"Line 2.5: Inserted by peer2",
				"Line 3: End of common content",
			},
			expected: `Line 1: Common content
Line 2: Modified by peer2
Line 2.5: Inserted by peer2
Line 3: End of common content
Line 4: Added by peer1
Line 5: More peer1 content`,
		},
		{
			name: "Simple prefix merge",
			lines1: []string{"a", "b", "c", "d"},
			lines2: []string{"a", "b", "e", "f"},
			expected: `a
b
c
d
e
f`,
		},
		{
			name: "Conflict case",
			lines1: []string{"a", "x", "c"},
			lines2: []string{"a", "y", "c"},
			expected: `a
<<<<<<< branch1
x
=======
y
>>>>>>> branch2
c`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeLines(tt.lines1, tt.lines2)
			resultStr := strings.Join(result, "\n")

			if resultStr != tt.expected {
				t.Errorf("mergeLines() = %q, want %q", resultStr, tt.expected)
				t.Logf("Lines1: %v", tt.lines1)
				t.Logf("Lines2: %v", tt.lines2)
			}
		})
	}
}
