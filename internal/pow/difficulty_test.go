package pow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultDifficultyConfig(t *testing.T) {
	cfg := DefaultDifficultyConfig()

	if cfg.Base != 20 {
		t.Errorf("expected Base=20, got %d", cfg.Base)
	}
	if cfg.Min != 16 {
		t.Errorf("expected Min=16, got %d", cfg.Min)
	}
	if cfg.Max != 24 {
		t.Errorf("expected Max=24, got %d", cfg.Max)
	}
	if cfg.UpdateInterval != 10*time.Second {
		t.Errorf("expected UpdateInterval=10s, got %v", cfg.UpdateInterval)
	}
	if cfg.ConnectionThresholdHigh != 5000 {
		t.Errorf("expected ConnectionThresholdHigh=5000, got %d", cfg.ConnectionThresholdHigh)
	}
	if cfg.ConnectionThresholdMedium != 1000 {
		t.Errorf("expected ConnectionThresholdMedium=1000, got %d", cfg.ConnectionThresholdMedium)
	}
	if cfg.RateThresholdHigh != 100 {
		t.Errorf("expected RateThresholdHigh=100, got %f", cfg.RateThresholdHigh)
	}
	if cfg.RateThresholdMedium != 50 {
		t.Errorf("expected RateThresholdMedium=50, got %f", cfg.RateThresholdMedium)
	}
	if cfg.CPUThreshold != 0.8 {
		t.Errorf("expected CPUThreshold=0.8, got %f", cfg.CPUThreshold)
	}
}

func TestDifficultyManager_Current(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	if dm.Current() != 20 {
		t.Errorf("expected initial difficulty=20, got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_NoAdjustment(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// Low load - no adjustment
	dm.Update(500, 25, 0.5)

	if dm.Current() != 20 {
		t.Errorf("expected difficulty=20 (no adjustment), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_ConnectionsHigh(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// High connections (>5000) - +2
	dm.Update(6000, 0, 0)

	if dm.Current() != 22 {
		t.Errorf("expected difficulty=22 (conn +2), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_ConnectionsMedium(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// Medium connections (>1000, <=5000) - +1
	dm.Update(2000, 0, 0)

	if dm.Current() != 21 {
		t.Errorf("expected difficulty=21 (conn +1), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_RateHigh(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// High rate (>100) - +2
	dm.Update(0, 150, 0)

	if dm.Current() != 22 {
		t.Errorf("expected difficulty=22 (rate +2), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_RateMedium(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// Medium rate (>50, <=100) - +1
	dm.Update(0, 75, 0)

	if dm.Current() != 21 {
		t.Errorf("expected difficulty=21 (rate +1), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_CPUHigh(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// High CPU (>0.8) - +1
	dm.Update(0, 0, 0.9)

	if dm.Current() != 21 {
		t.Errorf("expected difficulty=21 (cpu +1), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_AllFactors(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// All factors high: +2 (conn) +2 (rate) +1 (cpu) = +5
	// Base 20 + 5 = 25, but max is 24
	dm.Update(6000, 150, 0.9)

	if dm.Current() != 24 {
		t.Errorf("expected difficulty=24 (capped at max), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_ClampToMax(t *testing.T) {
	cfg := DifficultyConfig{
		Base: 22,
		Min:  16,
		Max:  24,
	}
	dm := NewDifficultyManager(cfg)

	// +2 (conn) would go to 24, which is max
	dm.Update(6000, 0, 0)

	if dm.Current() != 24 {
		t.Errorf("expected difficulty=24 (capped), got %d", dm.Current())
	}
}

func TestDifficultyManager_Update_ResetOnLowLoad(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// First high load
	dm.Update(6000, 150, 0.9)
	if dm.Current() != 24 {
		t.Errorf("expected difficulty=24 after high load, got %d", dm.Current())
	}

	// Then low load - should reset to base
	dm.Update(100, 10, 0.2)
	if dm.Current() != 20 {
		t.Errorf("expected difficulty=20 after low load, got %d", dm.Current())
	}
}

func TestDifficultyManager_StartStop(t *testing.T) {
	cfg := DifficultyConfig{
		Base:           20,
		Min:            16,
		Max:            24,
		UpdateInterval: 10 * time.Millisecond, // Fast for testing
	}
	dm := NewDifficultyManager(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount atomic.Int32
	gather := func() (int, float64, float64) {
		callCount.Add(1)
		return 6000, 0, 0 // High connections
	}

	dm.Start(ctx, gather)

	// Wait for a few updates
	time.Sleep(50 * time.Millisecond)

	dm.Stop()

	// Should have updated difficulty
	if dm.Current() != 22 {
		t.Errorf("expected difficulty=22 after updates, got %d", dm.Current())
	}

	// Should have called gather multiple times
	if callCount.Load() < 2 {
		t.Errorf("expected at least 2 gather calls, got %d", callCount.Load())
	}
}

func TestDifficultyManager_StartWithContextCancel(t *testing.T) {
	cfg := DifficultyConfig{
		Base:           20,
		Min:            16,
		Max:            24,
		UpdateInterval: 10 * time.Millisecond,
	}
	dm := NewDifficultyManager(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	var callCount atomic.Int32
	gather := func() (int, float64, float64) {
		callCount.Add(1)
		return 0, 0, 0
	}

	dm.Start(ctx, gather)
	time.Sleep(30 * time.Millisecond)

	// Cancel context
	cancel()
	time.Sleep(20 * time.Millisecond)

	countAfterCancel := callCount.Load()

	// Wait more and verify no more calls
	time.Sleep(30 * time.Millisecond)

	if callCount.Load() > countAfterCancel+1 {
		t.Errorf("gather called after context cancel: before=%d, after=%d", countAfterCancel, callCount.Load())
	}
}

func TestDifficultyManager_NilGather(t *testing.T) {
	cfg := DifficultyConfig{
		Base:           20,
		Min:            16,
		Max:            24,
		UpdateInterval: 10 * time.Millisecond,
	}
	dm := NewDifficultyManager(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic with nil gather
	dm.Start(ctx, nil)
	time.Sleep(30 * time.Millisecond)

	dm.Stop()

	// Difficulty should remain at base
	if dm.Current() != 20 {
		t.Errorf("expected difficulty=20 with nil gather, got %d", dm.Current())
	}
}

func TestDifficultyManager_DoubleStop(t *testing.T) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	// Double stop should not panic
	dm.Stop()
	dm.Stop()
}

func TestDifficultyManager_ZeroConfig(t *testing.T) {
	// Zero config should use defaults
	cfg := DifficultyConfig{}
	dm := NewDifficultyManager(cfg)

	// Should have set defaults
	if dm.Current() != 20 {
		t.Errorf("expected default difficulty=20, got %d", dm.Current())
	}
}

func TestDifficultyManager_BaseOutOfRange(t *testing.T) {
	t.Run("base below min", func(t *testing.T) {
		cfg := DifficultyConfig{
			Base: 10, // Below min (16)
			Min:  16,
			Max:  24,
		}
		dm := NewDifficultyManager(cfg)

		// Base should be clamped to min
		if dm.Current() != 16 {
			t.Errorf("expected difficulty=16 (clamped to min), got %d", dm.Current())
		}
	})

	t.Run("base above max", func(t *testing.T) {
		cfg := DifficultyConfig{
			Base: 30, // Above max (24)
			Min:  16,
			Max:  24,
		}
		dm := NewDifficultyManager(cfg)

		// Base should be clamped to max
		if dm.Current() != 24 {
			t.Errorf("expected difficulty=24 (clamped to max), got %d", dm.Current())
		}
	})
}

func TestStaticDifficultyManager(t *testing.T) {
	dm := StaticDifficultyManager(18)

	if dm.Current() != 18 {
		t.Errorf("expected difficulty=18, got %d", dm.Current())
	}

	// Update should be no-op
	dm.Update(10000, 500, 1.0)

	if dm.Current() != 18 {
		t.Errorf("expected difficulty=18 after update (no change), got %d", dm.Current())
	}

	// Start/Stop should be no-op
	ctx := context.Background()
	dm.Start(ctx, nil)
	dm.Stop()

	if dm.Current() != 18 {
		t.Errorf("expected difficulty=18 after start/stop, got %d", dm.Current())
	}
}

func TestDifficultyManager_CombinedAdjustments(t *testing.T) {
	tests := []struct {
		name        string
		activeConns int
		connRate    float64
		cpuLoad     float64
		expected    uint8
	}{
		{"no load", 0, 0, 0, 20},
		{"medium connections only", 2000, 0, 0, 21},
		{"high connections only", 6000, 0, 0, 22},
		{"medium rate only", 0, 75, 0, 21},
		{"high rate only", 0, 150, 0, 22},
		{"high cpu only", 0, 0, 0.9, 21},
		{"medium connections + medium rate", 2000, 75, 0, 22},
		{"high connections + high rate", 6000, 150, 0, 24},
		{"all medium", 2000, 75, 0.9, 23},
		{"all high", 6000, 150, 0.9, 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultDifficultyConfig()
			dm := NewDifficultyManager(cfg)

			dm.Update(tt.activeConns, tt.connRate, tt.cpuLoad)

			if dm.Current() != tt.expected {
				t.Errorf("expected difficulty=%d, got %d", tt.expected, dm.Current())
			}
		})
	}
}

func BenchmarkDifficultyManager_Update(b *testing.B) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dm.Update(5000, 100, 0.7)
	}
}

func BenchmarkDifficultyManager_Current(b *testing.B) {
	cfg := DefaultDifficultyConfig()
	dm := NewDifficultyManager(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dm.Current()
	}
}
