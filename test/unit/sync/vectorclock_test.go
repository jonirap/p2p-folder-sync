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
