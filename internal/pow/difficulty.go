package pow

import (
	"context"
	"sync/atomic"
	"time"
)

// DifficultyManager manages dynamic difficulty adjustment based on system load.
type DifficultyManager interface {
	// Current returns the current difficulty level.
	Current() uint8

	// Update recalculates difficulty based on system metrics.
	// activeConns: number of active connections
	// connRate: connections per second (typically 5-minute average)
	// cpuLoad: CPU utilization (0.0 to 1.0)
	Update(activeConns int, connRate float64, cpuLoad float64)

	// Start begins the background update loop.
	// The provided function is called periodically to gather metrics.
	Start(ctx context.Context, gather func() (activeConns int, connRate float64, cpuLoad float64))

	// Stop stops the background update loop.
	Stop()
}

// DifficultyConfig holds configuration for the difficulty manager.
type DifficultyConfig struct {
	// Base is the base difficulty level (default: 20)
	Base uint8

	// Min is the minimum difficulty level (default: 16)
	Min uint8

	// Max is the maximum difficulty level (default: 24)
	Max uint8

	// UpdateInterval is how often to recalculate difficulty (default: 10s)
	UpdateInterval time.Duration

	// ConnectionThresholdHigh triggers +2 difficulty when exceeded (default: 5000)
	ConnectionThresholdHigh int

	// ConnectionThresholdMedium triggers +1 difficulty when exceeded (default: 1000)
	ConnectionThresholdMedium int

	// RateThresholdHigh triggers +2 difficulty when exceeded (default: 100)
	RateThresholdHigh float64

	// RateThresholdMedium triggers +1 difficulty when exceeded (default: 50)
	RateThresholdMedium float64

	// CPUThreshold triggers +1 difficulty when exceeded (default: 0.8)
	CPUThreshold float64
}

// DefaultDifficultyConfig returns the default difficulty configuration.
func DefaultDifficultyConfig() DifficultyConfig {
	return DifficultyConfig{
		Base:                      20,
		Min:                       16,
		Max:                       24,
		UpdateInterval:            10 * time.Second,
		ConnectionThresholdHigh:   5000,
		ConnectionThresholdMedium: 1000,
		RateThresholdHigh:         100,
		RateThresholdMedium:       50,
		CPUThreshold:              0.8,
	}
}

// difficultyManager implements DifficultyManager.
type difficultyManager struct {
	config   DifficultyConfig
	current  atomic.Uint32
	stopChan chan struct{}
	stopped  atomic.Bool
}

// NewDifficultyManager creates a new difficulty manager with the given configuration.
func NewDifficultyManager(config DifficultyConfig) DifficultyManager {
	// Validate and set defaults
	if config.Base == 0 {
		config.Base = 20
	}
	if config.Min == 0 {
		config.Min = MinDifficulty
	}
	if config.Max == 0 {
		config.Max = MaxDifficulty
	}
	if config.UpdateInterval == 0 {
		config.UpdateInterval = 10 * time.Second
	}
	if config.ConnectionThresholdHigh == 0 {
		config.ConnectionThresholdHigh = 5000
	}
	if config.ConnectionThresholdMedium == 0 {
		config.ConnectionThresholdMedium = 1000
	}
	if config.RateThresholdHigh == 0 {
		config.RateThresholdHigh = 100
	}
	if config.RateThresholdMedium == 0 {
		config.RateThresholdMedium = 50
	}
	if config.CPUThreshold == 0 {
		config.CPUThreshold = 0.8
	}

	// Ensure min <= base <= max
	if config.Base < config.Min {
		config.Base = config.Min
	}
	if config.Base > config.Max {
		config.Base = config.Max
	}

	dm := &difficultyManager{
		config:   config,
		stopChan: make(chan struct{}),
	}
	dm.current.Store(uint32(config.Base))

	return dm
}

// Current returns the current difficulty level.
func (m *difficultyManager) Current() uint8 {
	return uint8(m.current.Load())
}

// Update recalculates difficulty based on system metrics.
func (m *difficultyManager) Update(activeConns int, connRate float64, cpuLoad float64) {
	adjustment := 0

	// Connection count factor
	if activeConns > m.config.ConnectionThresholdHigh {
		adjustment += 2
	} else if activeConns > m.config.ConnectionThresholdMedium {
		adjustment += 1
	}

	// Connection rate factor
	if connRate > m.config.RateThresholdHigh {
		adjustment += 2
	} else if connRate > m.config.RateThresholdMedium {
		adjustment += 1
	}

	// CPU load factor
	if cpuLoad > m.config.CPUThreshold {
		adjustment += 1
	}

	// Calculate new difficulty
	newDifficulty := int(m.config.Base) + adjustment

	// Clamp to [min, max]
	if newDifficulty < int(m.config.Min) {
		newDifficulty = int(m.config.Min)
	}
	if newDifficulty > int(m.config.Max) {
		newDifficulty = int(m.config.Max)
	}

	m.current.Store(uint32(newDifficulty))
}

// Start begins the background update loop.
func (m *difficultyManager) Start(ctx context.Context, gather func() (int, float64, float64)) {
	go m.updateLoop(ctx, gather)
}

// updateLoop periodically updates difficulty based on gathered metrics.
func (m *difficultyManager) updateLoop(ctx context.Context, gather func() (int, float64, float64)) {
	ticker := time.NewTicker(m.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			if gather != nil {
				activeConns, connRate, cpuLoad := gather()
				m.Update(activeConns, connRate, cpuLoad)
			}
		}
	}
}

// Stop stops the background update loop.
func (m *difficultyManager) Stop() {
	if m.stopped.CompareAndSwap(false, true) {
		close(m.stopChan)
	}
}

// MetricsGatherer is a helper function type for gathering system metrics.
type MetricsGatherer func() (activeConns int, connRate float64, cpuLoad float64)

// StaticDifficultyManager returns a difficulty manager that always returns the given difficulty.
// This is useful for testing or when dynamic difficulty is disabled.
func StaticDifficultyManager(difficulty uint8) DifficultyManager {
	dm := &staticDifficultyManager{difficulty: difficulty}
	return dm
}

type staticDifficultyManager struct {
	difficulty uint8
}

func (m *staticDifficultyManager) Current() uint8 {
	return m.difficulty
}

func (m *staticDifficultyManager) Update(activeConns int, connRate float64, cpuLoad float64) {
	// No-op for static manager
}

func (m *staticDifficultyManager) Start(ctx context.Context, gather func() (int, float64, float64)) {
	// No-op for static manager
}

func (m *staticDifficultyManager) Stop() {
	// No-op for static manager
}
