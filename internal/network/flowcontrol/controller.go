package flowcontrol

import (
	"context"
	"sync"
)

// FlowController manages bandwidth and flow control for file transfers
type FlowController struct {
	globalLimiter    *RateLimiter            // Global bandwidth limit
	perFileLimiters  map[string]*RateLimiter // Per-file rate limiters
	maxConcurrent    int                     // Maximum concurrent transfers
	activeTransfers  map[string]bool         // Currently active transfers
	transferSemaphore chan struct{}         // Semaphore for concurrency control
	mu               sync.RWMutex
}

// NewFlowController creates a new flow controller
func NewFlowController(globalBandwidth int64, maxConcurrent int) *FlowController {
	fc := &FlowController{
		globalLimiter:     NewRateLimiter(globalBandwidth, globalBandwidth*2), // 2x burst
		perFileLimiters:   make(map[string]*RateLimiter),
		maxConcurrent:     maxConcurrent,
		activeTransfers:   make(map[string]bool),
		transferSemaphore: make(chan struct{}, maxConcurrent),
	}

	// Fill semaphore
	for i := 0; i < maxConcurrent; i++ {
		fc.transferSemaphore <- struct{}{}
	}

	return fc
}

// AcquireTransferSlot acquires a slot for a new transfer
func (fc *FlowController) AcquireTransferSlot(ctx context.Context, fileID string) error {
	// Wait for available slot
	select {
	case <-fc.transferSemaphore:
		fc.mu.Lock()
		fc.activeTransfers[fileID] = true
		fc.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseTransferSlot releases a transfer slot
func (fc *FlowController) ReleaseTransferSlot(fileID string) {
	fc.mu.Lock()
	delete(fc.activeTransfers, fileID)
	fc.mu.Unlock()

	// Return slot to semaphore
	fc.transferSemaphore <- struct{}{}
}

// Wait waits for bandwidth to be available for sending n bytes
func (fc *FlowController) Wait(ctx context.Context, fileID string, n int64) error {
	// Apply global rate limit
	if err := fc.globalLimiter.Wait(ctx, n); err != nil {
		return err
	}

	// Apply per-file rate limit if exists
	fc.mu.RLock()
	perFileLimiter, hasPerFile := fc.perFileLimiters[fileID]
	fc.mu.RUnlock()

	if hasPerFile {
		return perFileLimiter.Wait(ctx, n)
	}

	return nil
}

// SetPerFileLimit sets a rate limit for a specific file
func (fc *FlowController) SetPerFileLimit(fileID string, rate int64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if limiter, exists := fc.perFileLimiters[fileID]; exists {
		limiter.SetRate(rate)
	} else {
		fc.perFileLimiters[fileID] = NewRateLimiter(rate, rate*2)
	}
}

// RemovePerFileLimit removes the rate limit for a specific file
func (fc *FlowController) RemovePerFileLimit(fileID string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if limiter, exists := fc.perFileLimiters[fileID]; exists {
		limiter.Stop()
		delete(fc.perFileLimiters, fileID)
	}
}

// SetGlobalBandwidth updates the global bandwidth limit
func (fc *FlowController) SetGlobalBandwidth(bandwidth int64) {
	fc.globalLimiter.SetRate(bandwidth)
}

// GetActiveTransferCount returns the number of active transfers
func (fc *FlowController) GetActiveTransferCount() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return len(fc.activeTransfers)
}

// GetActiveTransfers returns a list of active transfer file IDs
func (fc *FlowController) GetActiveTransfers() []string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	transfers := make([]string, 0, len(fc.activeTransfers))
	for fileID := range fc.activeTransfers {
		transfers = append(transfers, fileID)
	}
	return transfers
}

// Stop stops the flow controller and all rate limiters
func (fc *FlowController) Stop() {
	fc.globalLimiter.Stop()

	fc.mu.Lock()
	for _, limiter := range fc.perFileLimiters {
		limiter.Stop()
	}
	fc.perFileLimiters = make(map[string]*RateLimiter)
	fc.mu.Unlock()
}

// GetStats returns flow control statistics
func (fc *FlowController) GetStats() FlowControlStats {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return FlowControlStats{
		ActiveTransfers:   len(fc.activeTransfers),
		MaxConcurrent:     fc.maxConcurrent,
		AvailableSlots:    len(fc.transferSemaphore),
		GlobalBandwidth:   fc.globalLimiter.GetRate(),
		AvailableTokens:   fc.globalLimiter.GetAvailableTokens(),
	}
}

// FlowControlStats contains flow control statistics
type FlowControlStats struct {
	ActiveTransfers  int   // Number of active transfers
	MaxConcurrent    int   // Maximum concurrent transfers
	AvailableSlots   int   // Available transfer slots
	GlobalBandwidth  int64 // Global bandwidth limit (bytes/sec)
	AvailableTokens  int64 // Available tokens in global bucket
}
