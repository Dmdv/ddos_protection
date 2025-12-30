package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics provides all application metrics.
type Metrics struct {
	// POW Metrics
	ChallengesIssued  prometheus.Counter
	ChallengesSolved  prometheus.Counter
	ChallengesFailed  prometheus.Counter
	ChallengesExpired prometheus.Counter
	CurrentDifficulty prometheus.Gauge
	SolveDuration     prometheus.Histogram

	// TCP Metrics
	ConnectionsTotal    prometheus.Counter
	ConnectionsRejected *prometheus.CounterVec
	ConnectionsActive   prometheus.Gauge
	ConnectionDuration  prometheus.Histogram

	// Quote Metrics
	QuotesServed prometheus.Counter

	// Rate Limit Metrics
	RateLimitHits prometheus.Counter
}

// New creates a new Metrics instance with all metrics registered.
func New() *Metrics {
	return &Metrics{
		// POW Metrics
		ChallengesIssued: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_issued_total",
			Help:      "Total number of challenges generated",
		}),
		ChallengesSolved: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_solved_total",
			Help:      "Total number of successfully verified solutions",
		}),
		ChallengesFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_failed_total",
			Help:      "Total number of failed verification attempts",
		}),
		ChallengesExpired: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_expired_total",
			Help:      "Total number of expired challenges",
		}),
		CurrentDifficulty: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "pow",
			Name:      "current_difficulty",
			Help:      "Current PoW difficulty level in bits",
		}),
		SolveDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pow",
			Name:      "solve_duration_seconds",
			Help:      "Time from challenge issue to solution receipt",
			Buckets:   []float64{0.5, 1, 2, 5, 10, 15, 20, 25, 30, 60},
		}),

		// TCP Metrics
		ConnectionsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "tcp",
			Name:      "connections_total",
			Help:      "Total number of connections accepted",
		}),
		ConnectionsRejected: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tcp",
			Name:      "connections_rejected_total",
			Help:      "Total number of connections rejected",
		}, []string{"reason"}),
		ConnectionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "tcp",
			Name:      "connections_active",
			Help:      "Current number of active connections",
		}),
		ConnectionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "tcp",
			Name:      "connection_duration_seconds",
			Help:      "Connection lifetime distribution",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 70},
		}),

		// Quote Metrics
		QuotesServed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "quotes",
			Name:      "served_total",
			Help:      "Total number of quotes delivered",
		}),

		// Rate Limit Metrics
		RateLimitHits: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "rate",
			Name:      "limit_hits_total",
			Help:      "Total number of rate limit triggers",
		}),
	}
}

// Rejection reasons for ConnectionsRejected counter.
const (
	ReasonRateLimited = "rate_limited"
	ReasonPoolFull    = "pool_full"
)

// RecordChallengeIssued increments the challenges issued counter.
func (m *Metrics) RecordChallengeIssued() {
	m.ChallengesIssued.Inc()
}

// RecordChallengeSolved increments the challenges solved counter.
func (m *Metrics) RecordChallengeSolved() {
	m.ChallengesSolved.Inc()
}

// RecordChallengeFailed increments the challenges failed counter.
func (m *Metrics) RecordChallengeFailed() {
	m.ChallengesFailed.Inc()
}

// RecordChallengeExpired increments the challenges expired counter.
func (m *Metrics) RecordChallengeExpired() {
	m.ChallengesExpired.Inc()
}

// SetCurrentDifficulty sets the current difficulty gauge.
func (m *Metrics) SetCurrentDifficulty(difficulty int) {
	m.CurrentDifficulty.Set(float64(difficulty))
}

// ObserveSolveDuration records the solve duration.
func (m *Metrics) ObserveSolveDuration(duration time.Duration) {
	m.SolveDuration.Observe(duration.Seconds())
}

// RecordConnectionAccepted increments the connections total counter.
func (m *Metrics) RecordConnectionAccepted() {
	m.ConnectionsTotal.Inc()
}

// RecordConnectionRejected increments the connections rejected counter with reason.
func (m *Metrics) RecordConnectionRejected(reason string) {
	m.ConnectionsRejected.WithLabelValues(reason).Inc()
}

// SetActiveConnections sets the active connections gauge.
func (m *Metrics) SetActiveConnections(count int64) {
	m.ConnectionsActive.Set(float64(count))
}

// IncActiveConnections increments the active connections gauge.
func (m *Metrics) IncActiveConnections() {
	m.ConnectionsActive.Inc()
}

// DecActiveConnections decrements the active connections gauge.
func (m *Metrics) DecActiveConnections() {
	m.ConnectionsActive.Dec()
}

// ObserveConnectionDuration records the connection duration.
func (m *Metrics) ObserveConnectionDuration(duration time.Duration) {
	m.ConnectionDuration.Observe(duration.Seconds())
}

// RecordQuoteServed increments the quotes served counter.
func (m *Metrics) RecordQuoteServed() {
	m.QuotesServed.Inc()
}

// RecordRateLimitHit increments the rate limit hits counter.
func (m *Metrics) RecordRateLimitHit() {
	m.RateLimitHits.Inc()
}

// Server provides an HTTP server for Prometheus metrics.
type Server struct {
	httpServer *http.Server
	address    string
}

// NewServer creates a new metrics HTTP server.
func NewServer(address string) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &Server{
		httpServer: &http.Server{
			Addr:         address,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		address: address,
	}
}

// Start starts the metrics HTTP server.
// This method blocks until the server is stopped or an error occurs.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Address returns the server address.
func (s *Server) Address() string {
	return s.address
}

// Global metrics instance for convenience.
// Use New() for better testability.
var defaultMetrics *Metrics

// init creates the default metrics instance.
func init() {
	defaultMetrics = New()
}

// Default returns the default metrics instance.
func Default() *Metrics {
	return defaultMetrics
}

// Helper functions for global metrics.

// ChallengeIssued records a challenge issued using the default metrics.
func ChallengeIssued() {
	defaultMetrics.RecordChallengeIssued()
}

// ChallengeSolved records a challenge solved using the default metrics.
func ChallengeSolved() {
	defaultMetrics.RecordChallengeSolved()
}

// ChallengeFailed records a challenge failed using the default metrics.
func ChallengeFailed() {
	defaultMetrics.RecordChallengeFailed()
}

// ChallengeExpired records a challenge expired using the default metrics.
func ChallengeExpired() {
	defaultMetrics.RecordChallengeExpired()
}

// SetDifficulty sets the current difficulty using the default metrics.
func SetDifficulty(difficulty int) {
	defaultMetrics.SetCurrentDifficulty(difficulty)
}

// SolveDurationObserve records solve duration using the default metrics.
func SolveDurationObserve(d time.Duration) {
	defaultMetrics.ObserveSolveDuration(d)
}

// ConnectionAccepted records a connection accepted using the default metrics.
func ConnectionAccepted() {
	defaultMetrics.RecordConnectionAccepted()
}

// ConnectionRejected records a connection rejected using the default metrics.
func ConnectionRejected(reason string) {
	defaultMetrics.RecordConnectionRejected(reason)
}

// ActiveConnections sets active connections using the default metrics.
func ActiveConnections(count int64) {
	defaultMetrics.SetActiveConnections(count)
}

// IncActive increments active connections using the default metrics.
func IncActive() {
	defaultMetrics.IncActiveConnections()
}

// DecActive decrements active connections using the default metrics.
func DecActive() {
	defaultMetrics.DecActiveConnections()
}

// ConnectionDurationObserve records connection duration using the default metrics.
func ConnectionDurationObserve(d time.Duration) {
	defaultMetrics.ObserveConnectionDuration(d)
}

// QuoteServed records a quote served using the default metrics.
func QuoteServed() {
	defaultMetrics.RecordQuoteServed()
}

// RateLimitHit records a rate limit hit using the default metrics.
func RateLimitHit() {
	defaultMetrics.RecordRateLimitHit()
}

// NewMetricsHandler returns an HTTP handler for Prometheus metrics.
// This is a convenience function for simple setups.
func NewMetricsHandler() http.Handler {
	return promhttp.Handler()
}

// Registry returns the default Prometheus registry.
// Returns nil if the default registerer is not a *prometheus.Registry.
func Registry() *prometheus.Registry {
	reg, ok := prometheus.DefaultRegisterer.(*prometheus.Registry)
	if !ok {
		return nil
	}
	return reg
}

// MustRegister registers the provided collectors with the default registry.
func MustRegister(cs ...prometheus.Collector) {
	prometheus.MustRegister(cs...)
}

// TestMetrics creates isolated metrics for testing.
// These metrics are not registered with the default registry.
func TestMetrics() (*Metrics, *prometheus.Registry) {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		ChallengesIssued: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_issued_total",
			Help:      "Total number of challenges generated",
		}),
		ChallengesSolved: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_solved_total",
			Help:      "Total number of successfully verified solutions",
		}),
		ChallengesFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_failed_total",
			Help:      "Total number of failed verification attempts",
		}),
		ChallengesExpired: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pow",
			Name:      "challenges_expired_total",
			Help:      "Total number of expired challenges",
		}),
		CurrentDifficulty: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pow",
			Name:      "current_difficulty",
			Help:      "Current PoW difficulty level in bits",
		}),
		SolveDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pow",
			Name:      "solve_duration_seconds",
			Help:      "Time from challenge issue to solution receipt",
			Buckets:   []float64{0.5, 1, 2, 5, 10, 15, 20, 25, 30, 60},
		}),
		ConnectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "tcp",
			Name:      "connections_total",
			Help:      "Total number of connections accepted",
		}),
		ConnectionsRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tcp",
			Name:      "connections_rejected_total",
			Help:      "Total number of connections rejected",
		}, []string{"reason"}),
		ConnectionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "tcp",
			Name:      "connections_active",
			Help:      "Current number of active connections",
		}),
		ConnectionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "tcp",
			Name:      "connection_duration_seconds",
			Help:      "Connection lifetime distribution",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 70},
		}),
		QuotesServed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "quotes",
			Name:      "served_total",
			Help:      "Total number of quotes delivered",
		}),
		RateLimitHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "rate",
			Name:      "limit_hits_total",
			Help:      "Total number of rate limit triggers",
		}),
	}

	// Register with the test registry
	reg.MustRegister(
		m.ChallengesIssued,
		m.ChallengesSolved,
		m.ChallengesFailed,
		m.ChallengesExpired,
		m.CurrentDifficulty,
		m.SolveDuration,
		m.ConnectionsTotal,
		m.ConnectionsRejected,
		m.ConnectionsActive,
		m.ConnectionDuration,
		m.QuotesServed,
		m.RateLimitHits,
	)

	return m, reg
}

// StartServer starts the metrics HTTP server in a goroutine and returns the server.
// Returns an error if the server fails to start within the timeout.
func StartServer(address string) (*Server, error) {
	s := NewServer(address)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Give server a moment to start or fail
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("metrics server failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		return s, nil
	}
}
