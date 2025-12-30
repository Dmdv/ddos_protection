package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
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

	// MaxBuckets is the maximum number of buckets allowed (0 = unlimited).
	// When limit is reached, oldest buckets are evicted.
	MaxBuckets int

	// ShardCount is the number of shards for concurrent access (default: 16).
	// Higher values reduce lock contention at the cost of memory.
	ShardCount int
}

// DefaultConfig returns the default rate limiter configuration.
func DefaultConfig() Config {
	return Config{
		Rate:            10,              // 10 requests per second
		Burst:           20,              // Allow bursts up to 20
		CleanupInterval: 1 * time.Minute, // Clean up every minute
		CleanupAge:      5 * time.Minute, // Remove after 5 minutes idle
		MaxBuckets:      100000,          // Max 100K tracked IPs
		ShardCount:      16,              // 16 shards for concurrency
	}
}

// bucket represents a token bucket for a single IP.
type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// shard represents a single shard of the rate limiter.
type shard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

// limiter implements Limiter using sharded token buckets.
type limiter struct {
	shards      []*shard
	shardCount  int
	rate        float64
	burst       int
	maxBuckets  int
	bucketCount atomic.Int64 // Total bucket count across all shards
	stopChan    chan struct{}
	stopOnce    sync.Once

	cleanupInterval time.Duration
	cleanupAge      time.Duration

	// nowFunc is used for testing
	nowFunc func() time.Time
}

// New creates a new rate limiter with the given configuration.
func New(cfg Config) Limiter {
	shardCount := cfg.ShardCount
	if shardCount <= 0 {
		shardCount = 16 // Default shard count
	}

	shards := make([]*shard, shardCount)
	for i := range shards {
		shards[i] = &shard{
			buckets: make(map[string]*bucket),
		}
	}

	l := &limiter{
		shards:          shards,
		shardCount:      shardCount,
		rate:            cfg.Rate,
		burst:           cfg.Burst,
		maxBuckets:      cfg.MaxBuckets,
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

// getShard returns the shard for a given IP using FNV-1a hash.
func (l *limiter) getShard(ip string) *shard {
	// FNV-1a hash
	var hash uint32 = 2166136261
	for i := 0; i < len(ip); i++ {
		hash ^= uint32(ip[i])
		hash *= 16777619
	}
	return l.shards[hash%uint32(l.shardCount)]
}

// totalBuckets returns total bucket count (atomic, non-blocking).
func (l *limiter) totalBuckets() int {
	return int(l.bucketCount.Load())
}

// Allow checks if a request from the given IP is allowed.
func (l *limiter) Allow(ip string) bool {
	s := l.getShard(ip)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := l.nowFunc()

	b, ok := s.buckets[ip]
	if !ok {
		// Check bucket limit before creating new bucket
		if l.maxBuckets > 0 && l.totalBuckets() >= l.maxBuckets {
			// Evict oldest bucket from this shard
			l.evictOldestFromShard(s)
		}

		// Create new bucket with full tokens
		b = &bucket{
			tokens:   float64(l.burst),
			lastSeen: now,
		}
		s.buckets[ip] = b
		l.bucketCount.Add(1)
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

// evictOldestFromShard removes the oldest bucket from a shard.
// Caller must hold shard lock.
func (l *limiter) evictOldestFromShard(s *shard) {
	var oldestIP string
	var oldestTime time.Time

	for ip, b := range s.buckets {
		if oldestIP == "" || b.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = b.lastSeen
		}
	}

	if oldestIP != "" {
		delete(s.buckets, oldestIP)
		l.bucketCount.Add(-1)
	}
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
	now := l.nowFunc()
	for _, s := range l.shards {
		s.mu.Lock()
		for ip, b := range s.buckets {
			if now.Sub(b.lastSeen) > l.cleanupAge {
				delete(s.buckets, ip)
				l.bucketCount.Add(-1)
			}
		}
		s.mu.Unlock()
	}
}

// Stats returns current limiter statistics.
type Stats struct {
	ActiveBuckets int
}

// Stats returns current limiter statistics (for metrics).
func (l *limiter) Stats() Stats {
	return Stats{ActiveBuckets: l.totalBuckets()}
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
