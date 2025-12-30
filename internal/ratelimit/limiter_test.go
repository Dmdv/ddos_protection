package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

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
		cfg := Config{
			Rate:            10, // 10 tokens per second
			Burst:           5,
			CleanupInterval: time.Hour,
			CleanupAge:      time.Hour,
		}

		// Use a testable limiter with mock time
		l := &limiter{
			buckets:         make(map[string]*bucket),
			rate:            cfg.Rate,
			burst:           cfg.Burst,
			stopChan:        make(chan struct{}),
			cleanupInterval: cfg.CleanupInterval,
			cleanupAge:      cfg.CleanupAge,
		}

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
	cfg := Config{
		Rate:            10,
		Burst:           5,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}

	l := &limiter{
		buckets:         make(map[string]*bucket),
		rate:            cfg.Rate,
		burst:           cfg.Burst,
		stopChan:        make(chan struct{}),
		cleanupInterval: cfg.CleanupInterval,
		cleanupAge:      1 * time.Second, // Short cleanup age for test
	}

	now := time.Now()
	l.nowFunc = func() time.Time { return now }

	// Create some buckets
	l.Allow("192.168.1.1")
	l.Allow("192.168.1.2")

	if len(l.buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(l.buckets))
	}

	// Advance time past cleanup age
	now = now.Add(2 * time.Second)

	// Run cleanup
	l.cleanup()

	if len(l.buckets) != 0 {
		t.Errorf("expected 0 buckets after cleanup, got %d", len(l.buckets))
	}
}

func TestLimiter_Cleanup_PartialClean(t *testing.T) {
	cfg := Config{
		Rate:            10,
		Burst:           5,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}

	l := &limiter{
		buckets:         make(map[string]*bucket),
		rate:            cfg.Rate,
		burst:           cfg.Burst,
		stopChan:        make(chan struct{}),
		cleanupInterval: cfg.CleanupInterval,
		cleanupAge:      5 * time.Second,
	}

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
	if len(l.buckets) != 1 {
		t.Errorf("expected 1 bucket after partial cleanup, got %d", len(l.buckets))
	}

	if _, ok := l.buckets["192.168.1.2"]; !ok {
		t.Error("recent bucket should not be cleaned")
	}
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
