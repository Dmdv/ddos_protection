package server

import (
	"context"
	"sync/atomic"
)

// Pool manages a fixed number of connection slots using a semaphore pattern.
type Pool struct {
	sem     chan struct{}
	max     int
	active  atomic.Int64
	waiting atomic.Int64
}

// NewPool creates a new connection pool with the given maximum capacity.
func NewPool(maxConnections int) *Pool {
	if maxConnections <= 0 {
		maxConnections = 1
	}

	return &Pool{
		sem: make(chan struct{}, maxConnections),
		max: maxConnections,
	}
}

// Acquire attempts to acquire a connection slot.
// Returns true if a slot was acquired, false if the pool is full.
// This is a non-blocking operation.
func (p *Pool) Acquire() bool {
	select {
	case p.sem <- struct{}{}:
		p.active.Add(1)
		return true
	default:
		return false
	}
}

// AcquireWithContext attempts to acquire a connection slot with context support.
// Returns true if a slot was acquired, false if context cancelled or pool full.
func (p *Pool) AcquireWithContext(ctx context.Context) bool {
	p.waiting.Add(1)
	defer p.waiting.Add(-1)

	select {
	case p.sem <- struct{}{}:
		p.active.Add(1)
		return true
	case <-ctx.Done():
		return false
	}
}

// TryAcquireBlocking blocks until a slot is available or context is cancelled.
// Unlike Acquire, this will wait for a slot to become available.
func (p *Pool) TryAcquireBlocking(ctx context.Context) bool {
	p.waiting.Add(1)
	defer p.waiting.Add(-1)

	select {
	case p.sem <- struct{}{}:
		p.active.Add(1)
		return true
	case <-ctx.Done():
		return false
	}
}

// Release returns a connection slot to the pool.
// Must be called exactly once for each successful Acquire.
func (p *Pool) Release() {
	select {
	case <-p.sem:
		p.active.Add(-1)
	default:
		// This should never happen if Release is called correctly
		// But handle it gracefully to avoid panics
	}
}

// Active returns the number of currently active connections.
func (p *Pool) Active() int {
	return int(p.active.Load())
}

// Waiting returns the number of goroutines waiting for a slot.
func (p *Pool) Waiting() int {
	return int(p.waiting.Load())
}

// Available returns the number of available slots.
func (p *Pool) Available() int {
	return p.max - int(p.active.Load())
}

// Max returns the maximum pool capacity.
func (p *Pool) Max() int {
	return p.max
}

// IsFull returns true if the pool has no available slots.
func (p *Pool) IsFull() bool {
	return p.Active() >= p.max
}

// Stats returns current pool statistics.
type PoolStats struct {
	Active    int
	Available int
	Waiting   int
	Max       int
}

// Stats returns the current pool statistics.
func (p *Pool) Stats() PoolStats {
	active := int(p.active.Load())
	return PoolStats{
		Active:    active,
		Available: p.max - active,
		Waiting:   int(p.waiting.Load()),
		Max:       p.max,
	}
}
