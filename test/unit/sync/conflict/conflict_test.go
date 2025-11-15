package conflict_test

import (
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/sync/conflict"
)

func TestNewResolver(t *testing.T) {
	strategies := []string{"intelligent_merge", "last_write_wins"}

	for _, strategy := range strategies {
		resolver := conflict.NewResolver(strategy)
		if resolver == nil {
			t.Errorf("Expected resolver for strategy %s", strategy)
		}
		// Note: strategy field is unexported, so we can't test it directly
	}
}

func TestResolver_ResolveLWW(t *testing.T) {
	resolver := conflict.NewResolver("last_write_wins")

	// Create test operations
	op1 := &conflict.SyncOperation{
		ID:        "op1",
		Path:      "/test/file.txt",
		Timestamp: time.Now().Unix(),
		PeerID:    "peer1",
		Data:      []byte("content 1"),
	}

	op2 := &conflict.SyncOperation{
		ID:        "op2",
		Path:      "/test/file.txt",
		Timestamp: time.Now().Unix() + 1, // op2 is newer
		PeerID:    "peer2",
		Data:      []byte("content 2"),
	}

	result, err := resolver.ResolveLWW(op1, op2)
	if err != nil {
		t.Fatalf("Failed to resolve LWW: %v", err)
	}

	// Should return op2 since it has newer timestamp
	if result.ID != "op2" {
		t.Errorf("Expected op2 to win, got %s", result.ID)
	}
}

func TestResolver_Resolve3Way(t *testing.T) {
	resolver := conflict.NewResolver("intelligent_merge")

	// Test simple 3-way merge
	result, err := resolver.Resolve3Way("base", "base\nline1", "base\nline2")
	if err != nil {
		t.Fatalf("Failed to resolve 3-way merge: %v", err)
	}

	// Should contain conflict markers
	if !contains(result, "<<<<<<<") || !contains(result, "=======") || !contains(result, ">>>>>>>") {
		t.Errorf("Expected conflict markers in result: %s", result)
	}
}

func TestResolver_ResolveLWWFallback(t *testing.T) {
	resolver := conflict.NewResolver("last_write_wins")

	// Test timestamp comparison
	result := resolver.ResolveLWWFallback("content1", "content2", 100, 200)
	if result != "content2" {
		t.Errorf("Expected content2 (newer timestamp), got %s", result)
	}

	// Test same timestamp
	result = resolver.ResolveLWWFallback("content1", "content2", 100, 100)
	if result != "content1" {
		t.Errorf("Expected content1 (first in tiebreaker), got %s", result)
	}
}

func TestResolver_SelectStrategy(t *testing.T) {
	resolver := conflict.NewResolver("intelligent_merge")

	// Test text file
	textOp := &conflict.SyncOperation{
		Path: "/test/file.txt",
		Data: []byte("text content"),
	}

	strategy, err := resolver.SelectStrategy(textOp)
	if err != nil {
		t.Fatalf("Failed to select strategy: %v", err)
	}

	if strategy != "intelligent_merge" {
		t.Errorf("Expected intelligent_merge for text file, got %s", strategy)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || contains(s[1:], substr) || (len(s) > len(substr) && s[:len(substr)] == substr))
}
