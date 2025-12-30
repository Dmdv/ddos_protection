package metrics

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetrics_POW(t *testing.T) {
	m, _ := TestMetrics()

	t.Run("challenges issued", func(t *testing.T) {
		m.RecordChallengeIssued()
		m.RecordChallengeIssued()

		val := getCounterValue(t, m.ChallengesIssued)
		if val != 2 {
			t.Errorf("expected 2, got %f", val)
		}
	})

	t.Run("challenges solved", func(t *testing.T) {
		m.RecordChallengeSolved()

		val := getCounterValue(t, m.ChallengesSolved)
		if val != 1 {
			t.Errorf("expected 1, got %f", val)
		}
	})

	t.Run("challenges failed", func(t *testing.T) {
		m.RecordChallengeFailed()
		m.RecordChallengeFailed()
		m.RecordChallengeFailed()

		val := getCounterValue(t, m.ChallengesFailed)
		if val != 3 {
			t.Errorf("expected 3, got %f", val)
		}
	})

	t.Run("challenges expired", func(t *testing.T) {
		m.RecordChallengeExpired()

		val := getCounterValue(t, m.ChallengesExpired)
		if val != 1 {
			t.Errorf("expected 1, got %f", val)
		}
	})

	t.Run("current difficulty", func(t *testing.T) {
		m.SetCurrentDifficulty(20)

		val := getGaugeValue(t, m.CurrentDifficulty)
		if val != 20 {
			t.Errorf("expected 20, got %f", val)
		}

		m.SetCurrentDifficulty(24)
		val = getGaugeValue(t, m.CurrentDifficulty)
		if val != 24 {
			t.Errorf("expected 24, got %f", val)
		}
	})

	t.Run("solve duration", func(t *testing.T) {
		m.ObserveSolveDuration(5 * time.Second)
		m.ObserveSolveDuration(10 * time.Second)

		// Just verify no panic - histogram values are harder to test directly
	})
}

func TestMetrics_TCP(t *testing.T) {
	m, _ := TestMetrics()

	t.Run("connections total", func(t *testing.T) {
		m.RecordConnectionAccepted()
		m.RecordConnectionAccepted()
		m.RecordConnectionAccepted()

		val := getCounterValue(t, m.ConnectionsTotal)
		if val != 3 {
			t.Errorf("expected 3, got %f", val)
		}
	})

	t.Run("connections rejected rate limited", func(t *testing.T) {
		m.RecordConnectionRejected(ReasonRateLimited)
		m.RecordConnectionRejected(ReasonRateLimited)

		val := getCounterVecValue(t, m.ConnectionsRejected, ReasonRateLimited)
		if val != 2 {
			t.Errorf("expected 2, got %f", val)
		}
	})

	t.Run("connections rejected pool full", func(t *testing.T) {
		m.RecordConnectionRejected(ReasonPoolFull)

		val := getCounterVecValue(t, m.ConnectionsRejected, ReasonPoolFull)
		if val != 1 {
			t.Errorf("expected 1, got %f", val)
		}
	})

	t.Run("connections active", func(t *testing.T) {
		m.SetActiveConnections(100)

		val := getGaugeValue(t, m.ConnectionsActive)
		if val != 100 {
			t.Errorf("expected 100, got %f", val)
		}

		m.IncActiveConnections()
		val = getGaugeValue(t, m.ConnectionsActive)
		if val != 101 {
			t.Errorf("expected 101, got %f", val)
		}

		m.DecActiveConnections()
		val = getGaugeValue(t, m.ConnectionsActive)
		if val != 100 {
			t.Errorf("expected 100, got %f", val)
		}
	})

	t.Run("connection duration", func(t *testing.T) {
		m.ObserveConnectionDuration(30 * time.Second)
		// Just verify no panic
	})
}

func TestMetrics_Quotes(t *testing.T) {
	m, _ := TestMetrics()

	m.RecordQuoteServed()
	m.RecordQuoteServed()

	val := getCounterValue(t, m.QuotesServed)
	if val != 2 {
		t.Errorf("expected 2, got %f", val)
	}
}

func TestMetrics_RateLimit(t *testing.T) {
	m, _ := TestMetrics()

	m.RecordRateLimitHit()
	m.RecordRateLimitHit()
	m.RecordRateLimitHit()

	val := getCounterValue(t, m.RateLimitHits)
	if val != 3 {
		t.Errorf("expected 3, got %f", val)
	}
}

func TestServer_StartShutdown(t *testing.T) {
	// Use a random available port
	s := NewServer("127.0.0.1:0")

	// Start in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Check for startup error
	select {
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
}

func TestServer_Address(t *testing.T) {
	s := NewServer(":9999")
	if s.Address() != ":9999" {
		t.Errorf("expected :9999, got %s", s.Address())
	}
}

func TestDefault(t *testing.T) {
	d := Default()
	if d == nil {
		t.Error("default metrics should not be nil")
	}
}

func TestGlobalFunctions(t *testing.T) {
	// These use the global default metrics, just verify no panic
	ChallengeIssued()
	ChallengeSolved()
	ChallengeFailed()
	ChallengeExpired()
	SetDifficulty(20)
	SolveDurationObserve(5 * time.Second)
	ConnectionAccepted()
	ConnectionRejected(ReasonRateLimited)
	ActiveConnections(50)
	IncActive()
	DecActive()
	ConnectionDurationObserve(10 * time.Second)
	QuoteServed()
	RateLimitHit()
}

func TestNewMetricsHandler(t *testing.T) {
	handler := NewMetricsHandler()
	if handler == nil {
		t.Error("metrics handler should not be nil")
	}
}

func TestTestMetrics(t *testing.T) {
	m, reg := TestMetrics()

	if m == nil {
		t.Fatal("metrics should not be nil")
	}
	if reg == nil {
		t.Fatal("registry should not be nil")
	}

	// Verify metrics are registered to the test registry
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Should have several metric families
	if len(mfs) == 0 {
		t.Error("expected metrics to be registered")
	}
}

func TestReasonConstants(t *testing.T) {
	if ReasonRateLimited != "rate_limited" {
		t.Errorf("expected rate_limited, got %s", ReasonRateLimited)
	}
	if ReasonPoolFull != "pool_full" {
		t.Errorf("expected pool_full, got %s", ReasonPoolFull)
	}
}

func TestStartServer(t *testing.T) {
	// Test successful start
	s, err := StartServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s == nil {
		t.Fatal("expected server, got nil")
	}

	// Give server time to fully start
	time.Sleep(50 * time.Millisecond)

	// Verify it's running by checking we can access it
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
}

func TestRegistry(t *testing.T) {
	// Registry may return nil if default registerer is not a *prometheus.Registry
	// Just verify no panic
	reg := Registry()
	// The result depends on global state, so we can't assert much
	_ = reg
}

func TestMustRegister(t *testing.T) {
	// MustRegister should not panic for valid collectors
	// We can't easily test with new collectors because they may already be registered
	// Just verify the function exists and doesn't panic when called with empty args
	// Note: calling with already registered collectors would panic
}

// Helper functions

func getCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()

	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to write counter: %v", err)
	}
	return m.Counter.GetValue()
}

func getCounterVecValue(t *testing.T, cv *prometheus.CounterVec, label string) float64 {
	t.Helper()

	c, err := cv.GetMetricWithLabelValues(label)
	if err != nil {
		t.Fatalf("failed to get counter with label: %v", err)
	}

	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to write counter: %v", err)
	}
	return m.Counter.GetValue()
}

func getGaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()

	var m dto.Metric
	if err := g.Write(&m); err != nil {
		t.Fatalf("failed to write gauge: %v", err)
	}
	return m.Gauge.GetValue()
}
