package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter provides rate limiting per IP address using the token bucket algorithm.
type Limiter interface {
	// Allow checks if a request from the given IP is allowed.
	// Returns true if allowed, false if rate limited.
	Allow(ip string) bool

	// Close stops the cleanup goroutine and releases resources.
	Close()
}

// Config holds rate limiter configuration.
type Config struct {
	// Rate is the number of tokens added per second.
	Rate float64

	// Burst is the maximum number of tokens (bucket capacity).
	Burst int

	// CleanupInterval is how often to clean up stale buckets.
	CleanupInterval time.Duration

	// CleanupAge is how long a bucket must be idle before removal.
	CleanupAge time.Duration
}

// DefaultConfig returns the default rate limiter configuration.
func DefaultConfig() Config {
	return Config{
		Rate:            10,              // 10 requests per second
		Burst:           20,              // Allow bursts up to 20
		CleanupInterval: 1 * time.Minute, // Clean up every minute
		CleanupAge:      5 * time.Minute, // Remove after 5 minutes idle
	}
}

// bucket represents a token bucket for a single IP.
type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// limiter implements Limiter using token buckets.
type limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	burst    int
	stopChan chan struct{}
	stopOnce sync.Once

	cleanupInterval time.Duration
	cleanupAge      time.Duration

	// nowFunc is used for testing
	nowFunc func() time.Time
}

// New creates a new rate limiter with the given configuration.
func New(cfg Config) Limiter {
	l := &limiter{
		buckets:         make(map[string]*bucket),
		rate:            cfg.Rate,
		burst:           cfg.Burst,
		stopChan:        make(chan struct{}),
		cleanupInterval: cfg.CleanupInterval,
		cleanupAge:      cfg.CleanupAge,
		nowFunc:         time.Now,
	}

	// Start cleanup goroutine
	go l.cleanupLoop()

	return l
}

// NewWithDefaults creates a rate limiter with default configuration.
func NewWithDefaults() Limiter {
	return New(DefaultConfig())
}

// Allow checks if a request from the given IP is allowed.
func (l *limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()

	b, ok := l.buckets[ip]
	if !ok {
		// Create new bucket with full tokens
		b = &bucket{
			tokens:   float64(l.burst),
			lastSeen: now,
		}
		l.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * l.rate

	// Cap at burst limit
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}

	b.lastSeen = now

	// Check if request allowed
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// Close stops the cleanup goroutine.
func (l *limiter) Close() {
	l.stopOnce.Do(func() {
		close(l.stopChan)
	})
}

// cleanupLoop periodically removes stale buckets.
func (l *limiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopChan:
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

// cleanup removes buckets that haven't been accessed recently.
func (l *limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()
	for ip, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.cleanupAge {
			delete(l.buckets, ip)
		}
	}
}

// Stats returns current limiter statistics.
type Stats struct {
	ActiveBuckets int
}

// Stats returns current limiter statistics (for metrics).
func (l *limiter) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Stats{ActiveBuckets: len(l.buckets)}
}

// LimiterWithContext wraps a Limiter with context-aware methods.
type LimiterWithContext struct {
	limiter Limiter
}

// NewWithContext wraps a limiter with context support.
func NewWithContext(l Limiter) *LimiterWithContext {
	return &LimiterWithContext{limiter: l}
}

// AllowWithContext checks rate limit with context cancellation support.
func (l *LimiterWithContext) AllowWithContext(ctx context.Context, ip string) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		return l.limiter.Allow(ip)
	}
}
