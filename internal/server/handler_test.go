package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
	"github.com/ddos_protection/internal/quotes"
)

// mockConn implements net.Conn for testing
type mockConn struct {
	readData    []byte
	readPos     int
	readErr     error
	writeData   []byte
	writeErr    error
	closeErr    error
	deadlineErr error
	localAddr   net.Addr
	remoteAddr  net.Addr
}

func (m *mockConn) Read(b []byte) (int, error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n := copy(b, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeData = append(m.writeData, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	return m.closeErr
}

func (m *mockConn) LocalAddr() net.Addr {
	if m.localAddr != nil {
		return m.localAddr
	}
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (m *mockConn) RemoteAddr() net.Addr {
	if m.remoteAddr != nil {
		return m.remoteAddr
	}
	return &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return m.deadlineErr
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return m.deadlineErr
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return m.deadlineErr
}

// mockTimeoutError implements net.Error with timeout
type mockTimeoutError struct{}

func (m *mockTimeoutError) Error() string   { return "timeout" }
func (m *mockTimeoutError) Timeout() bool   { return true }
func (m *mockTimeoutError) Temporary() bool { return true }

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    connectionState
		expected string
	}{
		{stateConnected, "CONNECTED"},
		{stateChallenging, "CHALLENGING"},
		{stateAuthorized, "AUTHORIZED"},
		{stateCompleted, "COMPLETED"},
		{connectionState(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("connectionState(%d).String() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	t.Run("with nil logger", func(t *testing.T) {
		config := DefaultHandlerConfig()
		config.Logger = nil

		h := NewHandler(generator, verifier, quoteStore, config)
		if h == nil {
			t.Fatal("expected handler, got nil")
		}
	})

	t.Run("with custom logger", func(t *testing.T) {
		config := DefaultHandlerConfig()
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

		h := NewHandler(generator, verifier, quoteStore, config)
		if h == nil {
			t.Fatal("expected handler, got nil")
		}
	})
}

func TestHandler_HandleReadError(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("EOF returns nil", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateConnected,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		err := h.handleReadError(conn, cc, io.EOF)
		if err != nil {
			t.Errorf("expected nil error for EOF, got %v", err)
		}
	})

	t.Run("timeout in connected state", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateConnected,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		err := h.handleReadError(conn, cc, &mockTimeoutError{})
		if err == nil {
			t.Error("expected error for timeout")
		}
	})

	t.Run("timeout in challenging state sends error", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		err := h.handleReadError(conn, cc, &mockTimeoutError{})
		if err == nil {
			t.Error("expected error for timeout")
		}

		// Should have written error message
		if len(conn.writeData) == 0 {
			t.Error("expected error message to be written")
		}
	})

	t.Run("generic read error", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateConnected,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		err := h.handleReadError(conn, cc, errors.New("read error"))
		if err == nil {
			t.Error("expected error for read error")
		}
	})
}

func TestHandler_HandleVerificationError(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.MaxAttempts = 3

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("invalid signature is fatal", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   0,
		}

		done, err := h.handleVerificationError(conn, cc, pow.ErrInvalidSignature)
		if !done {
			t.Error("expected done=true for invalid signature")
		}
		if err == nil {
			t.Error("expected error for invalid signature")
		}
	})

	t.Run("challenge expired is fatal", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   0,
		}

		done, err := h.handleVerificationError(conn, cc, pow.ErrChallengeExpired)
		if !done {
			t.Error("expected done=true for expired challenge")
		}
		if err == nil {
			t.Error("expected error for expired challenge")
		}
	})

	t.Run("future timestamp is fatal", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   0,
		}

		done, err := h.handleVerificationError(conn, cc, pow.ErrFutureTimestamp)
		if !done {
			t.Error("expected done=true for future timestamp")
		}
		if err == nil {
			t.Error("expected error for future timestamp")
		}
	})

	t.Run("insufficient work allows retry", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   0,
		}

		done, err := h.handleVerificationError(conn, cc, pow.ErrInsufficientWork)
		if done {
			t.Error("expected done=false for insufficient work (retry allowed)")
		}
		if err != nil {
			t.Errorf("expected nil error for retry, got %v", err)
		}
		if cc.attempts != 1 {
			t.Errorf("expected attempts=1, got %d", cc.attempts)
		}
	})

	t.Run("insufficient work max attempts exceeded", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   2, // Will become 3 (max)
		}

		done, err := h.handleVerificationError(conn, cc, pow.ErrInsufficientWork)
		if !done {
			t.Error("expected done=true when max attempts exceeded")
		}
		if err == nil {
			t.Error("expected error when max attempts exceeded")
		}
	})

	t.Run("unknown error is fatal", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
			attempts:   0,
		}

		done, err := h.handleVerificationError(conn, cc, errors.New("unknown error"))
		if !done {
			t.Error("expected done=true for unknown error")
		}
		if err == nil {
			t.Error("expected error for unknown error")
		}
	})
}

func TestHandler_HandleAuthorized(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("unexpected message returns done", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateAuthorized,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewRequestChallenge()
		done, err := h.handleAuthorized(context.Background(), conn, cc, msg)
		if !done {
			t.Error("expected done=true for unexpected message in authorized state")
		}
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestHandler_HandleChallenging(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret, ClockSkew: time.Hour})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("invalid message type", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewRequestChallenge() // Wrong type
		done, err := h.handleChallenging(context.Background(), conn, cc, msg)
		if !done {
			t.Error("expected done=true for invalid message type")
		}
		if err == nil {
			t.Error("expected error for invalid message type")
		}
	})

	t.Run("invalid solution format", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateChallenging,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewSolution([]byte("invalid solution format"))
		done, err := h.handleChallenging(context.Background(), conn, cc, msg)
		if !done {
			t.Error("expected done=true for invalid solution format")
		}
		if err == nil {
			t.Error("expected error for invalid solution format")
		}
	})
}

func TestHandler_HandleConnected(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("invalid message type", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateConnected,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewSolution([]byte("wrong type"))
		done, err := h.handleConnected(context.Background(), conn, cc, msg)
		if !done {
			t.Error("expected done=true for invalid message type")
		}
		if err == nil {
			t.Error("expected error for invalid message type")
		}
	})

	t.Run("valid request challenge", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      stateConnected,
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewRequestChallenge()
		done, err := h.handleConnected(context.Background(), conn, cc, msg)
		if done {
			t.Error("expected done=false after valid request")
		}
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if cc.state != stateChallenging {
			t.Errorf("expected state=CHALLENGING, got %s", cc.state)
		}
		if cc.challenge == nil {
			t.Error("expected challenge to be set")
		}
	})
}

func TestHandler_SendMessage(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("write deadline error", func(t *testing.T) {
		conn := &mockConn{
			deadlineErr: errors.New("deadline error"),
		}

		msg := protocol.NewRequestChallenge()
		err := h.sendMessage(conn, msg)
		if err == nil {
			t.Error("expected error for deadline error")
		}
	})

	t.Run("write error", func(t *testing.T) {
		conn := &mockConn{
			writeErr: errors.New("write error"),
		}

		msg := protocol.NewRequestChallenge()
		err := h.sendMessage(conn, msg)
		if err == nil {
			t.Error("expected error for write error")
		}
	})

	t.Run("successful write", func(t *testing.T) {
		conn := &mockConn{}

		msg := protocol.NewRequestChallenge()
		err := h.sendMessage(conn, msg)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(conn.writeData) == 0 {
			t.Error("expected data to be written")
		}
	})
}

func TestHandler_Handle_ContextCancellation(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("cancelled context", func(t *testing.T) {
		// Create a connection that will block on read
		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- h.Handle(ctx, server)
		}()

		// Cancel context
		cancel()

		// Should return context error
		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected context.Canceled, got %v", err)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for handler to return")
		}
	})
}

func TestHandler_Handle_SetDeadlineError(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("set deadline error", func(t *testing.T) {
		conn := &mockConn{
			deadlineErr: errors.New("deadline error"),
		}

		err := h.Handle(context.Background(), conn)
		if err == nil {
			t.Error("expected error for set deadline error")
		}
	})
}

func TestHandler_ProcessMessage_InvalidState(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!!")
	generator, _ := pow.NewGenerator(secret)
	verifier := pow.NewVerifier(pow.VerifierConfig{Secret: secret})
	quoteStore := quotes.NewMemoryStore([]string{"Test quote"})

	config := DefaultHandlerConfig()
	config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewHandler(generator, verifier, quoteStore, config)

	t.Run("invalid state", func(t *testing.T) {
		conn := &mockConn{}
		cc := &connectionContext{
			state:      connectionState(99), // Invalid state
			remoteAddr: "192.168.1.1:12345",
			logger:     config.Logger,
		}

		msg := protocol.NewRequestChallenge()
		done, err := h.processMessage(context.Background(), conn, cc, msg)
		if !done {
			t.Error("expected done=true for invalid state")
		}
		if err == nil {
			t.Error("expected error for invalid state")
		}
	})
}
