package flowcontrol

import (
	"context"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting for bandwidth control
type RateLimiter struct {
	rate          int64         // bytes per second
	burst         int64         // maximum burst size
	tokens        int64         // current available tokens
	lastRefill    time.Time     // last refill time
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewRateLimiter creates a new rate limiter
// rate: bytes per second, burst: maximum burst size in bytes
func NewRateLimiter(rate, burst int64) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	rl := &RateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     burst, // Start with full bucket
		lastRefill: time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start token refill goroutine
	go rl.refillTokens()

	return rl
}

// Wait blocks until n bytes can be consumed
func (rl *RateLimiter) Wait(ctx context.Context, n int64) error {
	for {
		// Try to consume tokens
		if rl.tryConsume(n) {
			return nil
		}

		// Wait a bit before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rl.ctx.Done():
			return rl.ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Continue loop
		}
	}
}

// tryConsume attempts to consume n tokens, returns true if successful
func (rl *RateLimiter) tryConsume(n int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int64(elapsed.Seconds() * float64(rl.rate))

	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.burst {
			rl.tokens = rl.burst
		}
		rl.lastRefill = now
	}

	// Try to consume
	if rl.tokens >= n {
		rl.tokens -= n
		return true
	}

	return false
}

// refillTokens periodically refills the token bucket
func (rl *RateLimiter) refillTokens() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			elapsed := now.Sub(rl.lastRefill)
			tokensToAdd := int64(elapsed.Seconds() * float64(rl.rate))

			if tokensToAdd > 0 {
				rl.tokens += tokensToAdd
				if rl.tokens > rl.burst {
					rl.tokens = rl.burst
				}
				rl.lastRefill = now
			}
			rl.mu.Unlock()
		}
	}
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	rl.cancel()
}

// GetAvailableTokens returns the current number of available tokens
func (rl *RateLimiter) GetAvailableTokens() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.tokens
}

// SetRate updates the rate limit (bytes per second)
func (rl *RateLimiter) SetRate(rate int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
}

// GetRate returns the current rate limit
func (rl *RateLimiter) GetRate() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.rate
}
