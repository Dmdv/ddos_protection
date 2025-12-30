package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
	"github.com/ddos_protection/internal/quotes"
	"github.com/ddos_protection/internal/ratelimit"
)

// TestServer tests the main server functionality.
func TestServer_Start(t *testing.T) {
	t.Run("starts and accepts connections", func(t *testing.T) {
		server := createTestServer(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start server in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- server.Start(ctx)
		}()

		// Wait for server to start
		time.Sleep(50 * time.Millisecond)

		if server.Addr() == nil {
			t.Fatal("server address is nil after start")
		}

		// Connect to server
		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		conn.Close()

		// Stop server
		cancel()

		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server did not stop")
		}
	})

	t.Run("double start returns error", func(t *testing.T) {
		server := createTestServer(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		// Second start should fail
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		err := server.Start(ctx2)
		if err == nil {
			t.Error("expected error for double start")
		}
	})
}

func TestServer_Shutdown(t *testing.T) {
	t.Run("graceful shutdown", func(t *testing.T) {
		server := createTestServer(t)

		ctx, cancel := context.WithCancel(context.Background())

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		// Cancel context to trigger shutdown
		cancel()

		// Shutdown should complete
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown error: %v", err)
		}
	})
}

func TestServer_Stats(t *testing.T) {
	server := createTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	stats := server.Stats()

	if !stats.Running {
		t.Error("server should be running")
	}
	if stats.Address == "" {
		t.Error("address should not be empty")
	}
	if stats.PoolStats.Max != 100 {
		t.Errorf("expected pool max=100, got %d", stats.PoolStats.Max)
	}
}

func TestServer_RateLimiting(t *testing.T) {
	config := DefaultConfig()
	config.Address = "127.0.0.1:0"
	config.MaxConnections = 100
	config.RateLimitConfig = ratelimit.Config{
		Rate:            1, // 1 request per second
		Burst:           1, // Burst of 1
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HandlerConfig.Logger = config.Logger

	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	server := New(config, generator, verifier, quoteStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// First connection should succeed
	conn1, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("first connection failed: %v", err)
	}
	defer conn1.Close()

	// Subsequent connections should be rate limited
	var rateLimitedCount int
	for i := 0; i < 5; i++ {
		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			continue
		}

		// Try to read error message
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		msg, err := protocol.ReadMessage(conn)
		conn.Close()

		if err == nil && msg.Type == protocol.TypeError {
			code := msg.GetErrorCode()
			if code == protocol.ErrCodeRateLimited {
				rateLimitedCount++
			}
		}
	}

	if rateLimitedCount == 0 {
		t.Error("expected at least one rate limited connection")
	}
}

func TestServer_PoolFull(t *testing.T) {
	config := DefaultConfig()
	config.Address = "127.0.0.1:0"
	config.MaxConnections = 2 // Very small pool
	config.RateLimitConfig = ratelimit.Config{
		Rate:            1000,
		Burst:           1000,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HandlerConfig.Logger = config.Logger
	config.HandlerConfig.IdleTimeout = 5 * time.Second

	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	server := New(config, generator, verifier, quoteStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Fill pool with connections that stay open
	var conns []net.Conn
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("connection %d failed: %v", i, err)
		}
		conns = append(conns, conn)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Give server time to accept connections
	time.Sleep(50 * time.Millisecond)

	// Next connection should get server busy error
	conn, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("third connection dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	if msg.Type != protocol.TypeError {
		t.Errorf("expected ERROR message, got %s", msg.Type)
	}

	code := msg.GetErrorCode()
	if code != protocol.ErrCodeServerBusy {
		t.Errorf("expected ERR_SERVER_BUSY, got %s", code)
	}
}

func TestServer_HappyPath(t *testing.T) {
	server := createTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Connect to server
	conn, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Set deadline for entire exchange
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send REQUEST_CHALLENGE
	if err := protocol.WriteMessage(conn, protocol.NewRequestChallenge()); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	// Receive CHALLENGE
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read challenge: %v", err)
	}

	if msg.Type != protocol.TypeChallenge {
		t.Fatalf("expected CHALLENGE, got %s", msg.Type)
	}

	// Parse and solve challenge
	challenge, err := pow.UnmarshalChallenge(msg.Payload)
	if err != nil {
		t.Fatalf("failed to unmarshal challenge: %v", err)
	}

	solver := pow.NewSolver()
	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		t.Fatalf("failed to solve challenge: %v", err)
	}

	// Send SOLUTION
	solutionBytes := solution.Marshal()
	if err := protocol.WriteMessage(conn, protocol.NewSolution(solutionBytes)); err != nil {
		t.Fatalf("failed to send solution: %v", err)
	}

	// Receive QUOTE
	msg, err = protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read quote: %v", err)
	}

	if msg.Type != protocol.TypeQuote {
		t.Fatalf("expected QUOTE, got %s", msg.Type)
	}

	quote := string(msg.Payload)
	if quote != "Test quote" {
		t.Errorf("expected 'Test quote', got %q", quote)
	}
}

func TestServer_InvalidSolutionRetry(t *testing.T) {
	config := DefaultConfig()
	config.Address = "127.0.0.1:0"
	config.MaxConnections = 100
	config.HandlerConfig.Difficulty = 20
	config.HandlerConfig.MaxAttempts = 3
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HandlerConfig.Logger = config.Logger

	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	server := New(config, generator, verifier, quoteStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Get challenge
	protocol.WriteMessage(conn, protocol.NewRequestChallenge())
	msg, _ := protocol.ReadMessage(conn)

	challenge, _ := pow.UnmarshalChallenge(msg.Payload)

	// Send invalid solution (wrong counter)
	invalidSolution := &pow.Solution{
		Challenge: *challenge,
		Counter:   12345, // Wrong counter
	}

	for i := 0; i < 2; i++ {
		protocol.WriteMessage(conn, protocol.NewSolution(invalidSolution.Marshal()))
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		if msg.Type != protocol.TypeError {
			t.Fatalf("expected ERROR, got %s", msg.Type)
		}

		code := msg.GetErrorCode()
		if code != protocol.ErrCodeInvalidSolution {
			t.Errorf("expected ERR_INVALID_SOLUTION, got %s", code)
		}
	}

	// Third invalid attempt should get TOO_MANY_ATTEMPTS
	protocol.WriteMessage(conn, protocol.NewSolution(invalidSolution.Marshal()))
	msg, err = protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	code := msg.GetErrorCode()
	if code != protocol.ErrCodeTooManyAttempts {
		t.Errorf("expected ERR_TOO_MANY_ATTEMPTS, got %s", code)
	}
}

func TestServer_Concurrent(t *testing.T) {
	server := createTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Capture server address once to avoid race with Start()
	serverAddr := server.Addr().String()

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.Dial("tcp", serverAddr)
			if err != nil {
				return
			}
			defer conn.Close()

			conn.SetDeadline(time.Now().Add(10 * time.Second))

			// Full protocol exchange
			protocol.WriteMessage(conn, protocol.NewRequestChallenge())
			msg, err := protocol.ReadMessage(conn)
			if err != nil || msg.Type != protocol.TypeChallenge {
				return
			}

			challenge, _ := pow.UnmarshalChallenge(msg.Payload)
			solver := pow.NewSolver()
			solution, err := solver.Solve(ctx, challenge)
			if err != nil {
				return
			}

			protocol.WriteMessage(conn, protocol.NewSolution(solution.Marshal()))
			msg, err = protocol.ReadMessage(conn)
			if err != nil || msg.Type != protocol.TypeQuote {
				return
			}

			mu.Lock()
			successCount++
			mu.Unlock()
		}()
	}

	wg.Wait()

	if successCount < 5 {
		t.Errorf("expected at least 5 successful connections, got %d", successCount)
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"127.0.0.1:0", "127.0.0.1"},
		{"invalid", "invalid"}, // Returns as-is on parse failure
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractIP(tt.input)
			if result != tt.expected {
				t.Errorf("extractIP(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestServer_LoadTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	config := DefaultConfig()
	config.Address = "127.0.0.1:0"
	config.MaxConnections = 50
	config.HandlerConfig.Difficulty = 16 // Lower difficulty for faster load tests
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HandlerConfig.Logger = config.Logger
	config.RateLimitConfig = ratelimit.Config{
		Rate:            10000,
		Burst:           10000,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}

	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	server := New(config, generator, verifier, quoteStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	serverAddr := server.Addr().String()

	const (
		numClients     = 30
		targetSuccess  = 20 // At least 20 should succeed
		clientTimeout  = 30 * time.Second
	)

	var wg sync.WaitGroup
	var successCount, errorCount int
	var mu sync.Mutex

	startTime := time.Now()

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()

			conn.SetDeadline(time.Now().Add(clientTimeout))

			// Request challenge
			if err := protocol.WriteMessage(conn, protocol.NewRequestChallenge()); err != nil {
				return
			}

			// Read challenge
			msg, err := protocol.ReadMessage(conn)
			if err != nil || msg.Type != protocol.TypeChallenge {
				return
			}

			challenge, err := pow.UnmarshalChallenge(msg.Payload)
			if err != nil {
				return
			}

			// Solve challenge
			solver := pow.NewSolver()
			solution, err := solver.Solve(ctx, challenge)
			if err != nil {
				return
			}

			// Send solution
			if err := protocol.WriteMessage(conn, protocol.NewSolution(solution.Marshal())); err != nil {
				return
			}

			// Read quote
			msg, err = protocol.ReadMessage(conn)
			if err != nil || msg.Type != protocol.TypeQuote {
				mu.Lock()
				errorCount++
				mu.Unlock()
				return
			}

			mu.Lock()
			successCount++
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	t.Logf("Load test completed: %d/%d successful in %v", successCount, numClients, elapsed)

	if int(successCount) < targetSuccess {
		t.Errorf("load test: expected at least %d successful connections, got %d", targetSuccess, successCount)
	}

	// Verify server stats
	stats := server.Stats()
	t.Logf("Server stats: active=%d, pool_max=%d", stats.PoolStats.Active, stats.PoolStats.Max)
}

func TestServer_Security(t *testing.T) {
	t.Run("rejects forged challenge signature", func(t *testing.T) {
		// Create server with one secret
		config := DefaultConfig()
		config.Address = "127.0.0.1:0"
		config.MaxConnections = 100
		config.HandlerConfig.Difficulty = 16
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config.HandlerConfig.Logger = config.Logger
		config.RateLimitConfig = ratelimit.Config{
			Rate:            1000,
			Burst:           1000,
			CleanupInterval: time.Hour,
			CleanupAge:      time.Hour,
		}

		serverSecret := []byte("server-secret-key-32-bytes-long!")
		generator, _ := pow.NewGenerator(serverSecret)
		verifier := pow.NewVerifier(pow.VerifierConfig{Secret: serverSecret})
		quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

		server := New(config, generator, verifier, quoteStore)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Get legitimate challenge
		protocol.WriteMessage(conn, protocol.NewRequestChallenge())
		msg, _ := protocol.ReadMessage(conn)
		challenge, _ := pow.UnmarshalChallenge(msg.Payload)

		// Create solution with forged signature (using different secret)
		attackerSecret := []byte("attacker-secret-key-32-bytes!!!!")
		attackerGen, err := pow.NewGenerator(attackerSecret)
		if err != nil {
			t.Fatalf("failed to create attacker generator: %v", err)
		}
		forgedChallenge, err := attackerGen.Generate(challenge.Difficulty)
		if err != nil {
			t.Fatalf("failed to generate forged challenge: %v", err)
		}

		// Solve the forged challenge
		solver := pow.NewSolver()
		forgedSolution, err := solver.Solve(ctx, forgedChallenge)
		if err != nil {
			t.Fatalf("failed to solve forged challenge: %v", err)
		}

		// Send forged solution
		protocol.WriteMessage(conn, protocol.NewSolution(forgedSolution.Marshal()))
		msg, err = protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		if msg.Type != protocol.TypeError {
			t.Fatalf("expected ERROR for forged signature, got %s", msg.Type)
		}

		code := msg.GetErrorCode()
		if code != protocol.ErrCodeInvalidSignature {
			t.Errorf("expected ERR_INVALID_SIGNATURE, got %s", code)
		}
	})

	t.Run("rejects malformed solution", func(t *testing.T) {
		server := createTestServer(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Get challenge
		protocol.WriteMessage(conn, protocol.NewRequestChallenge())
		protocol.ReadMessage(conn)

		// Send garbage as solution
		malformedSolution := []byte{0x00, 0x01, 0x02, 0x03}
		protocol.WriteMessage(conn, protocol.NewSolution(malformedSolution))

		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		if msg.Type != protocol.TypeError {
			t.Fatalf("expected ERROR for malformed solution, got %s", msg.Type)
		}
	})

	t.Run("enforces idle timeout", func(t *testing.T) {
		config := DefaultConfig()
		config.Address = "127.0.0.1:0"
		config.MaxConnections = 100
		config.HandlerConfig.IdleTimeout = 100 * time.Millisecond
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config.HandlerConfig.Logger = config.Logger
		config.RateLimitConfig = ratelimit.Config{
			Rate:            1000,
			Burst:           1000,
			CleanupInterval: time.Hour,
			CleanupAge:      time.Hour,
		}

		secret := []byte("test-secret-key-32-bytes-long!!!")
		generator, _ := pow.NewGenerator(secret)
		verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
		quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

		server := New(config, generator, verifier, quoteStore)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		// Don't send anything, wait for timeout
		time.Sleep(200 * time.Millisecond)

		// Connection should be closed by server
		_, err = conn.Read(make([]byte, 1))
		if err == nil {
			t.Error("expected connection to be closed after idle timeout")
		}
	})

	t.Run("handles invalid message type in connected state", func(t *testing.T) {
		server := createTestServer(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go server.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		conn, err := net.Dial("tcp", server.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Send SOLUTION without getting challenge first
		invalidSolution := &pow.Solution{Counter: 12345}
		protocol.WriteMessage(conn, protocol.NewSolution(invalidSolution.Marshal()))

		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		if msg.Type != protocol.TypeError {
			t.Fatalf("expected ERROR for invalid state, got %s", msg.Type)
		}

		code := msg.GetErrorCode()
		if code != protocol.ErrCodeInvalidState {
			t.Errorf("expected ERR_INVALID_STATE, got %s", code)
		}
	})
}

// createTestServer creates a test server with reasonable defaults.
func createTestServer(t *testing.T) *Server {
	t.Helper()

	config := DefaultConfig()
	config.Address = "127.0.0.1:0" // Random port
	config.MaxConnections = 100
	config.HandlerConfig.Difficulty = 16 // Minimum valid difficulty for tests
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HandlerConfig.Logger = config.Logger
	// High rate limit for tests
	config.RateLimitConfig = ratelimit.Config{
		Rate:            1000,
		Burst:           1000,
		CleanupInterval: time.Hour,
		CleanupAge:      time.Hour,
	}

	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	return New(config, generator, verifier, quoteStore)
}
