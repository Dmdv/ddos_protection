package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ddos_protection/internal/config"
	"github.com/ddos_protection/internal/logging"
	"github.com/ddos_protection/internal/metrics"
	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/quotes"
	"github.com/ddos_protection/internal/ratelimit"
	"github.com/ddos_protection/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Fall back to panic since logger isn't initialized yet
		panic("failed to load configuration: " + err.Error())
	}

	if err := cfg.Validate(); err != nil {
		panic("configuration validation failed: " + err.Error())
	}

	// Initialize logger
	logger := logging.New(logging.Config{
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
		Output: os.Stdout,
	})
	logging.SetDefault(logger)

	logger.Info("starting Word of Wisdom server",
		"version", "1.0.0",
		"address", cfg.ServerAddress,
		"metrics_address", cfg.MetricsAddress,
	)

	// Start metrics server
	metricsServer, err := metrics.StartServer(cfg.MetricsAddress)
	if err != nil {
		logger.Error("failed to start metrics server", logging.Err(err))
		os.Exit(1)
	}
	logger.Info("metrics server started", "address", cfg.MetricsAddress)

	// Create PoW generator and verifier
	generator, err := pow.NewGenerator(cfg.PowSecret)
	if err != nil {
		logger.Error("failed to create PoW generator", logging.Err(err))
		os.Exit(1)
	}

	verifier, err := pow.NewVerifier(pow.VerifierConfig{
		Secret:           cfg.PowSecret,
		ChallengeTimeout: cfg.ChallengeTimeout,
		ClockSkew:        cfg.ClockSkewTolerance,
	})
	if err != nil {
		logger.Error("failed to create PoW verifier", logging.Err(err))
		os.Exit(1)
	}

	// Create difficulty manager
	difficultyConfig := pow.DifficultyConfig{
		Base:                      uint8(cfg.PowDifficulty),
		Min:                       uint8(cfg.PowMinDifficulty),
		Max:                       uint8(cfg.PowMaxDifficulty),
		UpdateInterval:            10 * time.Second,
		ConnectionThresholdHigh:   5000,
		ConnectionThresholdMedium: 1000,
		RateThresholdHigh:         100,
		RateThresholdMedium:       50,
		CPUThreshold:              0.8,
	}
	difficultyManager := pow.NewDifficultyManager(difficultyConfig)

	// Create quote store
	quoteStore := quotes.NewMemoryStore(nil) // Uses default quotes

	// Create server configuration
	serverConfig := server.Config{
		Address:         cfg.ServerAddress,
		MaxConnections:  cfg.MaxConnections,
		GracefulTimeout: cfg.GracefulTimeout,
		HandlerConfig: server.HandlerConfig{
			ReadTimeout:      cfg.ReadTimeout,
			WriteTimeout:     cfg.WriteTimeout,
			IdleTimeout:      cfg.IdleTimeout,
			ChallengeTimeout: cfg.ChallengeTimeout,
			MaxAttempts:      cfg.MaxAttemptsPerIP,
			Difficulty:       uint8(cfg.PowDifficulty),
			ClockSkew:        cfg.ClockSkewTolerance,
			Logger:           logger.Logger,
		},
		RateLimitConfig: ratelimit.Config{
			Rate:            float64(cfg.RateLimitRPS),
			Burst:           cfg.RateLimitBurst,
			CleanupInterval: 5 * time.Minute,
		},
		Logger: logger.Logger,
	}

	// Create and start server
	srv := server.New(serverConfig, generator, verifier, quoteStore)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start difficulty manager
	difficultyManager.Start(ctx, func() (int, float64, float64) {
		stats := srv.Stats()
		// Calculate connection rate (simplified - in production would use sliding window)
		connRate := float64(stats.ActiveConnections) / 10.0 // Approximate rate
		// CPU load would come from runtime metrics in production
		cpuLoad := 0.5 // Placeholder
		return int(stats.ActiveConnections), connRate, cpuLoad
	})
	defer difficultyManager.Stop()

	// Update metrics with current difficulty
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.SetDifficulty(int(difficultyManager.Current()))
			}
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", "signal", sig.String())
	case err := <-errChan:
		logger.Error("server error", logging.Err(err))
	}

	// Cancel context to initiate shutdown
	cancel()

	// Shutdown metrics server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", logging.Err(err))
	}

	logger.Info("server shutdown complete")
}
