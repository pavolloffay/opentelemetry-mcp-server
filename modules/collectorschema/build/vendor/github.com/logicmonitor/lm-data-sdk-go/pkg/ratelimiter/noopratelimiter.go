package ratelimiter

import (
	"context"
)

// NoopRateLimiter is a no-operation implementation of the RateLimiter interface.
type NoopRateLimiter struct{}

// Acquire always returns true with no error, effectively disabling rate limiting.
func (n *NoopRateLimiter) Acquire(payloadMetadata interface{}) (bool, error) {
	return true, nil
}

// Run does nothing.
func (n *NoopRateLimiter) Run(ctx context.Context) {}

// Shutdown does nothing.
func (n *NoopRateLimiter) Shutdown(ctx context.Context) {}
