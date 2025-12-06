package sync_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

func TestVectorClockIncrement(t *testing.T) {
	vc := sync.NewVectorClock()
	vc.Increment("peer1")

	if vc.Get("peer1") != 1 {
		t.Errorf("Expected 1, got %d", vc.Get("peer1"))
	}

	vc.Increment("peer1")
	if vc.Get("peer1") != 2 {
		t.Errorf("Expected 2, got %d", vc.Get("peer1"))
	}
}

func TestVectorClockMerge(t *testing.T) {
	vc1 := sync.NewVectorClock()
	vc2 := sync.NewVectorClock()

	vc1.Increment("peer1")
	vc1.Increment("peer1")
	vc2.Increment("peer1")

	vc1.Merge(vc2)

	if vc1.Get("peer1") != 2 {
		t.Errorf("Expected 2 after merge, got %d", vc1.Get("peer1"))
	}
}

func TestVectorClockCompare(t *testing.T) {
	vc1 := sync.NewVectorClock()
	vc2 := sync.NewVectorClock()

	vc1.Increment("peer1")
	vc2.Increment("peer1")
	vc2.Increment("peer2")

	result := vc1.Compare(vc2)
	if result != -1 {
		t.Errorf("Expected -1 (less), got %d", result)
	}
}

func TestVectorClockIsConcurrent(t *testing.T) {
	// Test that empty vector clocks are concurrent
	vc1 := sync.NewVectorClock()
	vc2 := sync.NewVectorClock()

	if !vc1.IsConcurrent(vc2) {
		t.Error("Two empty vector clocks should be concurrent")
	}

	// Test that sequential edits are NOT concurrent
	vc1.Increment("peer1")       // vc1 = {peer1: 1}
	vc2.Set("peer1", 1)           // vc2 = {peer1: 1}
	vc2.Increment("peer1")        // vc2 = {peer1: 2}

	if vc1.IsConcurrent(vc2) {
		t.Error("vc1={peer1:1} and vc2={peer1:2} should NOT be concurrent (sequential)")
	}

	// Test that truly concurrent edits ARE concurrent
	vc3 := sync.NewVectorClock()
	vc4 := sync.NewVectorClock()
	vc3.Increment("peer1")  // vc3 = {peer1: 1}
	vc4.Increment("peer2")  // vc4 = {peer2: 1}

	if !vc3.IsConcurrent(vc4) {
		t.Error("vc3={peer1:1} and vc4={peer2:1} SHOULD be concurrent")
	}
}
