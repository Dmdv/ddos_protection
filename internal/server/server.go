package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
	"github.com/ddos_protection/internal/quotes"
	"github.com/ddos_protection/internal/ratelimit"
)

// Server is the main TCP server for the Word of Wisdom service.
type Server struct {
	config      Config
	listener    net.Listener
	listenerMu  sync.RWMutex // Protects listener
	pool        *Pool
	rateLimiter ratelimit.Limiter
	handler     *Handler
	logger      *slog.Logger

	// Lifecycle management
	running      atomic.Bool
	accepting    atomic.Bool // True while accept loop is active
	wg           sync.WaitGroup
	acceptLoopWg sync.WaitGroup // Tracks accept loop goroutine
	closeOnce    sync.Once

	// Metrics
	totalConnections   atomic.Int64
	activeConnections  atomic.Int64
	rejectedRateLimit  atomic.Int64
	rejectedPoolFull   atomic.Int64
	successfulRequests atomic.Int64
	failedRequests     atomic.Int64
}

// Config holds server configuration.
type Config struct {
	// Address is the TCP address to listen on.
	Address string

	// MaxConnections is the maximum number of concurrent connections.
	MaxConnections int

	// GracefulTimeout is how long to wait for connections to close during shutdown.
	GracefulTimeout time.Duration

	// Handler configuration
	HandlerConfig HandlerConfig

	// Rate limiter configuration
	RateLimitConfig ratelimit.Config

	// Logger for server events
	Logger *slog.Logger
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{
		Address:         ":8080",
		MaxConnections:  10000,
		GracefulTimeout: 30 * time.Second,
		HandlerConfig:   DefaultHandlerConfig(),
		RateLimitConfig: ratelimit.DefaultConfig(),
		Logger:          slog.Default(),
	}
}

// New creates a new server with the given configuration and dependencies.
func New(
	config Config,
	generator pow.Generator,
	verifier pow.Verifier,
	quoteStore quotes.Store,
) *Server {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Create rate limiter
	rateLimiter := ratelimit.New(config.RateLimitConfig)

	// Create connection pool
	pool := NewPool(config.MaxConnections)

	// Create handler
	handler := NewHandler(generator, verifier, quoteStore, config.HandlerConfig)

	return &Server{
		config:      config,
		pool:        pool,
		rateLimiter: rateLimiter,
		handler:     handler,
		logger:      config.Logger,
	}
}

// Start begins accepting connections on the configured address.
// This method blocks until the context is cancelled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	if s.running.Load() {
		return errors.New("server already running")
	}

	// Create listener
	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.Address, err)
	}

	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()
	s.running.Store(true)

	s.logger.Info("server started",
		slog.String("address", listener.Addr().String()),
		slog.Int("max_connections", s.config.MaxConnections),
	)

	// Accept loop
	s.accepting.Store(true)
	s.acceptLoopWg.Add(1)
	go func() {
		defer s.acceptLoopWg.Done()
		defer s.accepting.Store(false)
		s.acceptLoop(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Initiate shutdown
	return s.shutdown()
}

// acceptLoop accepts incoming connections until the context is cancelled.
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set accept deadline to allow checking context
		if tcpListener, ok := s.listener.(*net.TCPListener); ok {
			_ = tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.isShuttingDown(err) {
				return
			}

			// Check for temporary errors (like timeout)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			s.logger.Error("accept error", slog.String("error", err.Error()))
			continue
		}

		s.totalConnections.Add(1)

		// Handle connection in goroutine
		s.wg.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection processes a single client connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	clientIP := extractIP(remoteAddr)

	logger := s.logger.With(
		slog.String("remote_addr", remoteAddr),
		slog.String("client_ip", clientIP),
	)

	// Check rate limit
	if !s.rateLimiter.Allow(clientIP) {
		s.rejectedRateLimit.Add(1)
		logger.Debug("rate limited")
		s.sendErrorAndClose(conn, protocol.ErrCodeRateLimited)
		return
	}

	// Acquire connection slot
	if !s.pool.Acquire() {
		s.rejectedPoolFull.Add(1)
		logger.Debug("connection pool full")
		s.sendErrorAndClose(conn, protocol.ErrCodeServerBusy)
		return
	}
	defer s.pool.Release()

	s.activeConnections.Add(1)
	defer s.activeConnections.Add(-1)

	logger.Debug("connection accepted")

	// Handle the connection
	if err := s.handler.Handle(ctx, conn); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Debug("handler error", slog.String("error", err.Error()))
			s.failedRequests.Add(1)
		}
		return
	}

	s.successfulRequests.Add(1)
}

// sendErrorAndClose sends an error message and closes the connection.
func (s *Server) sendErrorAndClose(conn net.Conn, code protocol.ErrorCode) {
	// Set a short timeout for the error message
	_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	_ = protocol.WriteMessage(conn, protocol.NewError(code))
}

// shutdown gracefully stops the server.
func (s *Server) shutdown() error {
	var shutdownErr error

	s.closeOnce.Do(func() {
		s.logger.Info("server shutting down")

		// Stop accepting new connections
		s.listenerMu.Lock()
		listener := s.listener
		s.listenerMu.Unlock()
		if listener != nil {
			if err := listener.Close(); err != nil {
				shutdownErr = fmt.Errorf("close listener: %w", err)
			}
		}

		// Wait for accept loop to exit before waiting on connection handlers
		// This prevents race between wg.Add() and wg.Wait()
		s.acceptLoopWg.Wait()

		// Wait for existing connections with timeout
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			s.logger.Info("all connections closed gracefully")
		case <-time.After(s.config.GracefulTimeout):
			s.logger.Warn("graceful shutdown timeout, some connections may be interrupted",
				slog.Duration("timeout", s.config.GracefulTimeout),
			)
		}

		// Close rate limiter
		s.rateLimiter.Close()

		s.running.Store(false)
		s.logger.Info("server stopped")
	})

	return shutdownErr
}

// Shutdown initiates a graceful shutdown of the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.shutdown()
}

// isShuttingDown checks if the error indicates the server is shutting down.
func (s *Server) isShuttingDown(err error) bool {
	if !s.running.Load() {
		return true
	}

	// Check for "use of closed network connection" error
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// Check for specific error message (for older Go versions)
	if err != nil && err.Error() == "use of closed network connection" {
		return true
	}

	return false
}

// Addr returns the server's listening address.
// Returns nil if the server is not running.
func (s *Server) Addr() net.Addr {
	s.listenerMu.RLock()
	defer s.listenerMu.RUnlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Stats returns current server statistics.
type Stats struct {
	Running            bool
	Address            string
	TotalConnections   int64
	ActiveConnections  int64
	RejectedRateLimit  int64
	RejectedPoolFull   int64
	SuccessfulRequests int64
	FailedRequests     int64
	PoolStats          PoolStats
}

// Stats returns the current server statistics.
func (s *Server) Stats() Stats {
	addr := ""
	s.listenerMu.RLock()
	if s.listener != nil {
		addr = s.listener.Addr().String()
	}
	s.listenerMu.RUnlock()

	return Stats{
		Running:            s.running.Load(),
		Address:            addr,
		TotalConnections:   s.totalConnections.Load(),
		ActiveConnections:  s.activeConnections.Load(),
		RejectedRateLimit:  s.rejectedRateLimit.Load(),
		RejectedPoolFull:   s.rejectedPoolFull.Load(),
		SuccessfulRequests: s.successfulRequests.Load(),
		FailedRequests:     s.failedRequests.Load(),
		PoolStats:          s.pool.Stats(),
	}
}

// extractIP extracts the IP address from a remote address string.
func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
