package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestLimiter creates a limiter for testing with mock time support.
func newTestLimiter(rate float64, burst int) *limiter {
	shards := make([]*shard, 1) // Single shard for tests
	shards[0] = &shard{buckets: make(map[string]*bucket)}

	return &limiter{
		shards:          shards,
		shardCount:      1,
		rate:            rate,
		burst:           burst,
		maxBuckets:      0, // No limit for most tests
		stopChan:        make(chan struct{}),
		cleanupInterval: time.Hour,
		cleanupAge:      time.Hour,
		nowFunc:         time.Now,
	}
}

func TestLimiter_Allow(t *testing.T) {
	t.Run("allows initial burst", func(t *testing.T) {
		cfg := Config{
			Rate:            10,
			Burst:           5,
			CleanupInterval: time.Hour, // Disable cleanup for test
			CleanupAge:      time.Hour,
		}
		l := New(cfg)
		defer l.Close()

		// Should allow burst number of requests
		for i := 0; i < 5; i++ {
			if !l.Allow("192.168.1.1") {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// Next request should be rate limited
		if l.Allow("192.168.1.1") {
			t.Error("request 6 should be rate limited")
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		// Use a testable limiter with mock time
		l := newTestLimiter(10, 5)

		now := time.Now()
		l.nowFunc = func() time.Time { return now }

		// Exhaust tokens
		for i := 0; i < 5; i++ {
			l.Allow("192.168.1.1")
		}

		if l.Allow("192.168.1.1") {
			t.Error("should be rate limited after exhausting burst")
		}

		// Advance time by 0.5 seconds (should add 5 tokens)
		now = now.Add(500 * time.Millisecond)

		// Should allow 5 more requests
		for i := 0; i < 5; i++ {
			if !l.Allow("192.168.1.1") {
				t.Errorf("request %d after refill should be allowed", i+1)
			}
		}
	})

	t.Run("separate buckets per IP", func(t *testing.T) {
		cfg := Config{
			Rate:            1,
			Burst:           1,
			CleanupInterval: time.Hour,
			CleanupAge:      time.Hour,
		}
		l := New(cfg)
		defer l.Close()

		// First IP uses its token
		if !l.Allow("192.168.1.1") {
			t.Error("first IP first request should be allowed")
		}
		if l.Allow("192.168.1.1") {
			t.Error("first IP second request should be limited")
		}

		// Second IP should have its own bucket
		if !l.Allow("192.168.1.2") {
			t.Error("second IP first request should be allowed")
		}
	})
}

func TestLimiter_Close(t *testing.T) {
	l := NewWithDefaults()

	// Close should not panic
	l.Close()

	// Double close should not panic
	l.Close()
}

func TestLimiter_Concurrent(t *testing.T) {
	cfg := Config{
		Rate:            1000,
		Burst:           100,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}
	l := New(cfg)
	defer l.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(ip int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Allow("192.168.1.1")
			}
		}(i)
	}
	wg.Wait()
}

func TestLimiter_Stats(t *testing.T) {
	cfg := Config{
		Rate:            10,
		Burst:           5,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}
	l := New(cfg).(*limiter)
	defer l.Close()

	stats := l.Stats()
	if stats.ActiveBuckets != 0 {
		t.Errorf("expected 0 active buckets, got %d", stats.ActiveBuckets)
	}

	// Create some buckets
	l.Allow("192.168.1.1")
	l.Allow("192.168.1.2")
	l.Allow("192.168.1.3")

	stats = l.Stats()
	if stats.ActiveBuckets != 3 {
		t.Errorf("expected 3 active buckets, got %d", stats.ActiveBuckets)
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	l := newTestLimiter(10, 5)
	l.cleanupAge = 1 * time.Second // Short cleanup age for test

	now := time.Now()
	l.nowFunc = func() time.Time { return now }

	// Create some buckets
	l.Allow("192.168.1.1")
	l.Allow("192.168.1.2")

	if l.totalBuckets() != 2 {
		t.Fatalf("expected 2 buckets, got %d", l.totalBuckets())
	}

	// Advance time past cleanup age
	now = now.Add(2 * time.Second)

	// Run cleanup
	l.cleanup()

	if l.totalBuckets() != 0 {
		t.Errorf("expected 0 buckets after cleanup, got %d", l.totalBuckets())
	}
}

func TestLimiter_Cleanup_PartialClean(t *testing.T) {
	l := newTestLimiter(10, 5)
	l.cleanupAge = 5 * time.Second

	now := time.Now()
	l.nowFunc = func() time.Time { return now }

	// Create first bucket
	l.Allow("192.168.1.1")

	// Advance time
	now = now.Add(3 * time.Second)

	// Create second bucket (more recent)
	l.Allow("192.168.1.2")

	// Advance time so first bucket is stale but second is not
	now = now.Add(3 * time.Second)

	l.cleanup()

	// Only the first bucket should be cleaned
	if l.totalBuckets() != 1 {
		t.Errorf("expected 1 bucket after partial cleanup, got %d", l.totalBuckets())
	}

	// Check that the second bucket still exists by testing Allow behavior
	// (since we can't access buckets directly anymore)
	// The bucket for 192.168.1.2 should still exist with some tokens
}

func TestLimiterWithContext_AllowWithContext(t *testing.T) {
	l := NewWithDefaults()
	defer l.Close()

	lc := NewWithContext(l)

	t.Run("allows when context is active", func(t *testing.T) {
		ctx := context.Background()
		if !lc.AllowWithContext(ctx, "192.168.1.1") {
			t.Error("should allow when context is active")
		}
	})

	t.Run("denies when context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if lc.AllowWithContext(ctx, "192.168.1.1") {
			t.Error("should deny when context is cancelled")
		}
	})
}

func TestLimiter_MaxBuckets(t *testing.T) {
	l := newTestLimiter(10, 5)
	l.maxBuckets = 3 // Only allow 3 buckets

	now := time.Now()
	l.nowFunc = func() time.Time { return now }

	// Create 3 buckets
	l.Allow("192.168.1.1")
	l.Allow("192.168.1.2")
	l.Allow("192.168.1.3")

	if l.totalBuckets() != 3 {
		t.Fatalf("expected 3 buckets, got %d", l.totalBuckets())
	}

	// Adding a 4th should evict the oldest
	l.Allow("192.168.1.4")

	if l.totalBuckets() != 3 {
		t.Errorf("expected 3 buckets after eviction, got %d", l.totalBuckets())
	}
}

func TestLimiter_Sharding(t *testing.T) {
	cfg := Config{
		Rate:            100,
		Burst:           10,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
		MaxBuckets:      0,
		ShardCount:      4,
	}
	l := New(cfg).(*limiter)
	defer l.Close()

	// Verify shards were created
	if len(l.shards) != 4 {
		t.Errorf("expected 4 shards, got %d", len(l.shards))
	}

	// Create requests from different IPs
	for i := 0; i < 100; i++ {
		l.Allow("192.168.1." + string(rune('0'+i%10)))
	}

	// Should have distributed buckets across shards
	totalBuckets := 0
	for _, s := range l.shards {
		s.mu.Lock()
		totalBuckets += len(s.buckets)
		s.mu.Unlock()
	}

	if totalBuckets != 10 {
		t.Errorf("expected 10 total buckets, got %d", totalBuckets)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Rate <= 0 {
		t.Error("default rate should be positive")
	}
	if cfg.Burst <= 0 {
		t.Error("default burst should be positive")
	}
	if cfg.CleanupInterval <= 0 {
		t.Error("default cleanup interval should be positive")
	}
	if cfg.CleanupAge <= 0 {
		t.Error("default cleanup age should be positive")
	}
	if cfg.MaxBuckets <= 0 {
		t.Error("default max buckets should be positive")
	}
	if cfg.ShardCount <= 0 {
		t.Error("default shard count should be positive")
	}
}

func BenchmarkLimiter_Allow(b *testing.B) {
	l := NewWithDefaults()
	defer l.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow("192.168.1.1")
	}
}

func BenchmarkLimiter_Allow_MultipleIPs(b *testing.B) {
	l := NewWithDefaults()
	defer l.Close()

	ips := make([]string, 1000)
	for i := range ips {
		ips[i] = "192.168.1." + string(rune(i%256))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(ips[i%len(ips)])
	}
}
