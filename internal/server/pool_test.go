package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPool_Acquire(t *testing.T) {
	t.Run("acquires up to max connections", func(t *testing.T) {
		pool := NewPool(3)

		// Should acquire 3 slots
		for i := 0; i < 3; i++ {
			if !pool.Acquire() {
				t.Errorf("acquire %d should succeed", i+1)
			}
		}

		if pool.Active() != 3 {
			t.Errorf("expected 3 active, got %d", pool.Active())
		}
	})

	t.Run("fails when pool is full", func(t *testing.T) {
		pool := NewPool(2)

		pool.Acquire()
		pool.Acquire()

		if pool.Acquire() {
			t.Error("acquire should fail when pool is full")
		}
	})

	t.Run("non-blocking", func(t *testing.T) {
		pool := NewPool(1)
		pool.Acquire()

		// Should return immediately
		start := time.Now()
		pool.Acquire()
		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			t.Errorf("Acquire should be non-blocking, took %v", elapsed)
		}
	})
}

func TestPool_Release(t *testing.T) {
	t.Run("releases slot for reuse", func(t *testing.T) {
		pool := NewPool(1)

		if !pool.Acquire() {
			t.Fatal("first acquire should succeed")
		}

		if pool.Acquire() {
			t.Fatal("second acquire should fail")
		}

		pool.Release()

		if !pool.Acquire() {
			t.Error("acquire after release should succeed")
		}
	})

	t.Run("decrements active count", func(t *testing.T) {
		pool := NewPool(3)

		pool.Acquire()
		pool.Acquire()
		pool.Acquire()

		if pool.Active() != 3 {
			t.Fatalf("expected 3 active, got %d", pool.Active())
		}

		pool.Release()

		if pool.Active() != 2 {
			t.Errorf("expected 2 active after release, got %d", pool.Active())
		}
	})

	t.Run("extra release does not panic", func(t *testing.T) {
		pool := NewPool(1)

		// Release without acquire should not panic
		pool.Release()
		pool.Release()
	})
}

func TestPool_AcquireWithContext(t *testing.T) {
	t.Run("acquires when slot available", func(t *testing.T) {
		pool := NewPool(1)
		ctx := context.Background()

		if !pool.AcquireWithContext(ctx) {
			t.Error("should acquire when slot available")
		}
	})

	t.Run("fails on cancelled context", func(t *testing.T) {
		pool := NewPool(1)
		pool.Acquire() // Fill pool

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if pool.AcquireWithContext(ctx) {
			t.Error("should fail on cancelled context")
		}
	})

	t.Run("waits for slot", func(t *testing.T) {
		pool := NewPool(1)
		pool.Acquire() // Fill pool

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Release slot after 50ms
		go func() {
			time.Sleep(50 * time.Millisecond)
			pool.Release()
		}()

		if !pool.AcquireWithContext(ctx) {
			t.Error("should acquire after slot released")
		}
	})
}

func TestPool_TryAcquireBlocking(t *testing.T) {
	t.Run("acquires when slot available", func(t *testing.T) {
		pool := NewPool(1)
		ctx := context.Background()

		if !pool.TryAcquireBlocking(ctx) {
			t.Error("should acquire when slot available")
		}
	})

	t.Run("waits and acquires", func(t *testing.T) {
		pool := NewPool(1)
		pool.Acquire()

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(50 * time.Millisecond)
			pool.Release()
		}()

		if !pool.TryAcquireBlocking(ctx) {
			t.Error("should acquire after wait")
		}
	})
}

func TestPool_Stats(t *testing.T) {
	pool := NewPool(5)

	pool.Acquire()
	pool.Acquire()
	pool.Acquire()

	stats := pool.Stats()

	if stats.Active != 3 {
		t.Errorf("expected Active=3, got %d", stats.Active)
	}
	if stats.Available != 2 {
		t.Errorf("expected Available=2, got %d", stats.Available)
	}
	if stats.Max != 5 {
		t.Errorf("expected Max=5, got %d", stats.Max)
	}
}

func TestPool_Available(t *testing.T) {
	pool := NewPool(5)

	if pool.Available() != 5 {
		t.Errorf("expected 5 available initially, got %d", pool.Available())
	}

	pool.Acquire()
	pool.Acquire()

	if pool.Available() != 3 {
		t.Errorf("expected 3 available after 2 acquires, got %d", pool.Available())
	}
}

func TestPool_IsFull(t *testing.T) {
	pool := NewPool(2)

	if pool.IsFull() {
		t.Error("pool should not be full initially")
	}

	pool.Acquire()
	pool.Acquire()

	if !pool.IsFull() {
		t.Error("pool should be full after max acquires")
	}

	pool.Release()

	if pool.IsFull() {
		t.Error("pool should not be full after release")
	}
}

func TestPool_Max(t *testing.T) {
	pool := NewPool(42)

	if pool.Max() != 42 {
		t.Errorf("expected max=42, got %d", pool.Max())
	}
}

func TestPool_Waiting(t *testing.T) {
	pool := NewPool(1)
	pool.Acquire() // Fill pool

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start waiting goroutine
	go func() {
		pool.AcquireWithContext(ctx)
	}()

	// Give goroutine time to start waiting
	time.Sleep(10 * time.Millisecond)

	if pool.Waiting() == 0 {
		t.Error("expected at least 1 waiting")
	}

	// Release to allow waiting goroutine to proceed
	pool.Release()
}

func TestPool_Concurrent(t *testing.T) {
	pool := NewPool(100)

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if pool.Acquire() {
				defer pool.Release()
				// Simulate some work
				time.Sleep(time.Microsecond)
			}
		}()
	}
	wg.Wait()

	if pool.Active() != 0 {
		t.Errorf("expected 0 active after all released, got %d", pool.Active())
	}
}

func TestPool_ZeroMaxConnections(t *testing.T) {
	// Should default to 1
	pool := NewPool(0)

	if pool.Max() != 1 {
		t.Errorf("expected max=1 for zero input, got %d", pool.Max())
	}
}

func TestPool_NegativeMaxConnections(t *testing.T) {
	// Should default to 1
	pool := NewPool(-5)

	if pool.Max() != 1 {
		t.Errorf("expected max=1 for negative input, got %d", pool.Max())
	}
}

func BenchmarkPool_AcquireRelease(b *testing.B) {
	pool := NewPool(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Acquire()
		pool.Release()
	}
}

func BenchmarkPool_Concurrent(b *testing.B) {
	pool := NewPool(1000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if pool.Acquire() {
				pool.Release()
			}
		}
	})
}
