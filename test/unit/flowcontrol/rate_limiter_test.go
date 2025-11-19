package flowcontrol_test

import (
	"context"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/flowcontrol"
)

func TestRateLimiter_Creation(t *testing.T) {
	rl := flowcontrol.NewRateLimiter(1000, 2000)
	defer rl.Stop()

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}

	t.Log("SUCCESS: Rate limiter created successfully")
}

func TestRateLimiter_BasicLimit(t *testing.T) {
	// 100 bytes per second, 200 byte burst
	rl := flowcontrol.NewRateLimiter(100, 200)
	defer rl.Stop()

	ctx := context.Background()

	// Should be able to consume burst immediately
	start := time.Now()
	err := rl.Wait(ctx, 200)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("Burst consumption took too long: %v", elapsed)
	}

	t.Logf("SUCCESS: Consumed 200 bytes in %v", elapsed)
}

func TestRateLimiter_RateEnforcement(t *testing.T) {
	// 1000 bytes per second
	rl := flowcontrol.NewRateLimiter(1000, 1000)
	defer rl.Stop()

	ctx := context.Background()

	// Consume full burst
	err := rl.Wait(ctx, 1000)
	if err != nil {
		t.Fatalf("First wait failed: %v", err)
	}

	// Try to consume more - should wait for refill
	start := time.Now()
	err = rl.Wait(ctx, 500)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Second wait failed: %v", err)
	}

	// Should take at least 400ms (500 bytes at 1000 bytes/sec, accounting for some refill)
	if elapsed < 400*time.Millisecond {
		t.Errorf("Rate limiting not enforced properly. Expected >400ms, got %v", elapsed)
	}

	t.Logf("SUCCESS: Rate limiting enforced (waited %v for 500 bytes)", elapsed)
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	rl := flowcontrol.NewRateLimiter(100, 100)
	defer rl.Stop()

	// Consume all tokens
	ctx := context.Background()
	rl.Wait(ctx, 100)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Try to wait - should be cancelled
	start := time.Now()
	err := rl.Wait(ctx, 500)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	if elapsed > 200*time.Millisecond {
		t.Errorf("Cancellation took too long: %v", elapsed)
	}

	t.Logf("SUCCESS: Context cancellation works (cancelled after %v)", elapsed)
}

func TestRateLimiter_DynamicRateChange(t *testing.T) {
	rl := flowcontrol.NewRateLimiter(1000, 1000)
	defer rl.Stop()

	if rl.GetRate() != 1000 {
		t.Errorf("Initial rate incorrect: expected 1000, got %d", rl.GetRate())
	}

	// Change rate
	rl.SetRate(2000)

	if rl.GetRate() != 2000 {
		t.Errorf("Updated rate incorrect: expected 2000, got %d", rl.GetRate())
	}

	t.Log("SUCCESS: Dynamic rate change works")
}

func TestFlowController_Creation(t *testing.T) {
	fc := flowcontrol.NewFlowController(1000000, 10) // 1MB/s, 10 concurrent
	defer fc.Stop()

	if fc == nil {
		t.Fatal("NewFlowController returned nil")
	}

	stats := fc.GetStats()
	if stats.MaxConcurrent != 10 {
		t.Errorf("Expected max concurrent 10, got %d", stats.MaxConcurrent)
	}

	t.Log("SUCCESS: Flow controller created successfully")
}

func TestFlowController_TransferSlots(t *testing.T) {
	fc := flowcontrol.NewFlowController(1000000, 2) // Only 2 concurrent transfers
	defer fc.Stop()

	ctx := context.Background()

	// Acquire first slot
	err := fc.AcquireTransferSlot(ctx, "file1")
	if err != nil {
		t.Fatalf("Failed to acquire first slot: %v", err)
	}

	// Acquire second slot
	err = fc.AcquireTransferSlot(ctx, "file2")
	if err != nil {
		t.Fatalf("Failed to acquire second slot: %v", err)
	}

	// Check active transfers
	if fc.GetActiveTransferCount() != 2 {
		t.Errorf("Expected 2 active transfers, got %d", fc.GetActiveTransferCount())
	}

	// Third slot should block (test with timeout)
	ctx3, cancel3 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel3()

	err = fc.AcquireTransferSlot(ctx3, "file3")
	if err == nil {
		t.Error("Expected timeout error for third slot, got nil")
	}

	// Release one slot
	fc.ReleaseTransferSlot("file1")

	// Now should be able to acquire
	err = fc.AcquireTransferSlot(context.Background(), "file3")
	if err != nil {
		t.Errorf("Failed to acquire slot after release: %v", err)
	}

	t.Log("SUCCESS: Transfer slot management works correctly")
}

func TestFlowController_PerFileLimit(t *testing.T) {
	fc := flowcontrol.NewFlowController(10000000, 10) // High global limit
	defer fc.Stop()

	// Set per-file limit of 1000 bytes/sec for file1
	fc.SetPerFileLimit("file1", 1000)

	ctx := context.Background()

	// Should be fast (within global limit)
	start := time.Now()
	err := fc.Wait(ctx, "file2", 5000) // Different file
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Wait failed for file2: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("file2 wait took too long (should be fast): %v", elapsed)
	}

	// file1 should be rate limited - exhaust the burst (2000 bytes)
	err = fc.Wait(ctx, "file1", 2000) // Consume full burst (rate is 1000, burst is 2000)
	if err != nil {
		t.Fatalf("First wait failed for file1: %v", err)
	}

	// Now measure the second wait which should be rate limited
	start = time.Now()
	err = fc.Wait(ctx, "file1", 500) // Should wait for tokens to refill at 1000 bytes/sec
	elapsed = time.Since(start)

	if err != nil {
		t.Fatalf("Second wait failed for file1: %v", err)
	}

	// Should take at least 400ms (500 bytes at 1000 bytes/sec, accounting for some refill during measurement)
	if elapsed < 400*time.Millisecond {
		t.Errorf("Per-file limit not enforced. Expected >400ms, got %v", elapsed)
	}

	t.Logf("SUCCESS: Per-file rate limiting works (waited %v for rate-limited file)", elapsed)
}

func TestFlowController_Stats(t *testing.T) {
	fc := flowcontrol.NewFlowController(5000000, 5)
	defer fc.Stop()

	stats := fc.GetStats()

	if stats.MaxConcurrent != 5 {
		t.Errorf("Expected max concurrent 5, got %d", stats.MaxConcurrent)
	}

	if stats.GlobalBandwidth != 5000000 {
		t.Errorf("Expected bandwidth 5000000, got %d", stats.GlobalBandwidth)
	}

	if stats.ActiveTransfers != 0 {
		t.Errorf("Expected 0 active transfers, got %d", stats.ActiveTransfers)
	}

	// Acquire a slot
	ctx := context.Background()
	fc.AcquireTransferSlot(ctx, "test-file")

	stats = fc.GetStats()
	if stats.ActiveTransfers != 1 {
		t.Errorf("Expected 1 active transfer, got %d", stats.ActiveTransfers)
	}

	t.Log("SUCCESS: Flow control statistics work correctly")
}
