package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/ddos_protection/internal/metrics"
	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
	"github.com/ddos_protection/internal/quotes"
)

// connectionState represents the state of a client connection.
type connectionState int

const (
	stateConnected connectionState = iota
	stateChallenging
	stateAuthorized
	stateCompleted
)

func (s connectionState) String() string {
	switch s {
	case stateConnected:
		return "CONNECTED"
	case stateChallenging:
		return "CHALLENGING"
	case stateAuthorized:
		return "AUTHORIZED"
	case stateCompleted:
		return "COMPLETED"
	default:
		return "UNKNOWN"
	}
}

// HandlerConfig holds configuration for the connection handler.
type HandlerConfig struct {
	// ReadTimeout is the timeout for reading a message.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for writing a message.
	WriteTimeout time.Duration

	// IdleTimeout is the timeout for waiting for the first message.
	IdleTimeout time.Duration

	// ChallengeTimeout is how long a challenge remains valid.
	ChallengeTimeout time.Duration

	// MaxAttempts is the maximum number of solution attempts allowed.
	MaxAttempts int

	// Difficulty is the PoW difficulty level.
	Difficulty uint8

	// ClockSkew is the allowed clock skew for challenge timestamp validation.
	ClockSkew time.Duration

	// Logger for handler events.
	Logger *slog.Logger
}

// DefaultHandlerConfig returns the default handler configuration.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      5 * time.Second,
		ChallengeTimeout: 60 * time.Second,
		MaxAttempts:      3,
		Difficulty:       20,
		ClockSkew:        30 * time.Second,
		Logger:           slog.Default(),
	}
}

// Handler manages the protocol for a single connection.
type Handler struct {
	generator pow.Generator
	verifier  pow.Verifier
	quotes    quotes.Store
	config    HandlerConfig
}

// NewHandler creates a new connection handler.
func NewHandler(generator pow.Generator, verifier pow.Verifier, quoteStore quotes.Store, config HandlerConfig) *Handler {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &Handler{
		generator: generator,
		verifier:  verifier,
		quotes:    quoteStore,
		config:    config,
	}
}

// connectionContext holds the state for a single connection.
type connectionContext struct {
	state         connectionState
	remoteAddr    string
	challenge     *pow.Challenge
	challengeTime time.Time
	attempts      int
	logger        *slog.Logger
}

// Handle processes a single client connection through the protocol state machine.
func (h *Handler) Handle(ctx context.Context, conn net.Conn) error {
	cc := &connectionContext{
		state:      stateConnected,
		remoteAddr: conn.RemoteAddr().String(),
		logger: h.config.Logger.With(
			slog.String("remote_addr", conn.RemoteAddr().String()),
		),
	}

	cc.logger.Debug("connection accepted", slog.String("state", cc.state.String()))

	// Set initial idle timeout
	if err := conn.SetReadDeadline(time.Now().Add(h.config.IdleTimeout)); err != nil {
		return fmt.Errorf("set idle deadline: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			cc.logger.Debug("context cancelled")
			return ctx.Err()
		default:
		}

		// Read next message
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return h.handleReadError(conn, cc, err)
		}

		// Process message based on current state
		done, err := h.processMessage(ctx, conn, cc, msg)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// handleReadError processes read errors and determines if they are fatal.
func (h *Handler) handleReadError(conn net.Conn, cc *connectionContext, err error) error {
	if errors.Is(err, io.EOF) {
		cc.logger.Debug("client disconnected")
		return nil
	}

	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		cc.logger.Debug("connection timeout", slog.String("state", cc.state.String()))

		// Send appropriate error based on state
		if cc.state == stateChallenging {
			_ = h.sendError(conn, protocol.ErrCodeChallengeExpired)
		}
		return fmt.Errorf("read timeout: %w", err)
	}

	cc.logger.Debug("read error", slog.String("error", err.Error()))
	return fmt.Errorf("read message: %w", err)
}

// processMessage handles a single message based on the current state.
func (h *Handler) processMessage(ctx context.Context, conn net.Conn, cc *connectionContext, msg *protocol.Message) (done bool, err error) {
	cc.logger.Debug("received message",
		slog.String("type", msg.Type.String()),
		slog.String("state", cc.state.String()),
	)

	switch cc.state {
	case stateConnected:
		return h.handleConnected(ctx, conn, cc, msg)

	case stateChallenging:
		return h.handleChallenging(ctx, conn, cc, msg)

	case stateAuthorized:
		return h.handleAuthorized(ctx, conn, cc, msg)

	default:
		return true, fmt.Errorf("invalid state: %s", cc.state)
	}
}

// handleConnected processes messages in the CONNECTED state.
func (h *Handler) handleConnected(ctx context.Context, conn net.Conn, cc *connectionContext, msg *protocol.Message) (bool, error) {
	// Only REQUEST_CHALLENGE is valid in CONNECTED state
	if msg.Type != protocol.TypeRequestChallenge {
		cc.logger.Debug("invalid message type in connected state",
			slog.String("expected", "REQUEST_CHALLENGE"),
			slog.String("got", msg.Type.String()),
		)
		if err := h.sendError(conn, protocol.ErrCodeInvalidState); err != nil {
			return true, err
		}
		return true, fmt.Errorf("invalid message type: expected REQUEST_CHALLENGE, got %s", msg.Type)
	}

	// Generate challenge
	challenge, err := h.generator.Generate(h.config.Difficulty)
	if err != nil {
		cc.logger.Error("failed to generate challenge", slog.String("error", err.Error()))
		if sendErr := h.sendError(conn, protocol.ErrCodeInvalidMessage); sendErr != nil {
			return true, sendErr
		}
		return true, fmt.Errorf("generate challenge: %w", err)
	}

	// Send challenge
	challengeBytes := challenge.Marshal()
	challengeMsg := protocol.NewChallenge(challengeBytes)

	if err := h.sendMessage(conn, challengeMsg); err != nil {
		return true, err
	}

	// Record challenge issued metric
	metrics.ChallengeIssued()

	// Transition to CHALLENGING state
	cc.state = stateChallenging
	cc.challenge = challenge
	cc.challengeTime = time.Now()
	cc.attempts = 0

	// Set challenge timeout
	if err := conn.SetReadDeadline(time.Now().Add(h.config.ChallengeTimeout)); err != nil {
		return true, fmt.Errorf("set challenge deadline: %w", err)
	}

	cc.logger.Debug("challenge sent",
		slog.String("state", cc.state.String()),
		slog.Int("difficulty", int(h.config.Difficulty)),
	)

	return false, nil
}

// handleChallenging processes messages in the CHALLENGING state.
func (h *Handler) handleChallenging(ctx context.Context, conn net.Conn, cc *connectionContext, msg *protocol.Message) (bool, error) {
	// Only SOLUTION is valid in CHALLENGING state
	if msg.Type != protocol.TypeSolution {
		cc.logger.Debug("invalid message type in challenging state",
			slog.String("expected", "SOLUTION"),
			slog.String("got", msg.Type.String()),
		)
		if err := h.sendError(conn, protocol.ErrCodeInvalidState); err != nil {
			return true, err
		}
		return true, fmt.Errorf("invalid message type: expected SOLUTION, got %s", msg.Type)
	}

	// Parse solution
	solution, err := pow.UnmarshalSolution(msg.Payload)
	if err != nil {
		cc.logger.Debug("failed to parse solution", slog.String("error", err.Error()))
		if err := h.sendError(conn, protocol.ErrCodeInvalidMessage); err != nil {
			return true, err
		}
		return true, fmt.Errorf("parse solution: %w", err)
	}

	// Verify solution
	if err := h.verifier.Verify(solution); err != nil {
		return h.handleVerificationError(conn, cc, err)
	}

	// Solution is valid - transition to AUTHORIZED
	cc.state = stateAuthorized

	// Record successful verification metrics
	metrics.ChallengeSolved()
	if !cc.challengeTime.IsZero() {
		metrics.SolveDurationObserve(time.Since(cc.challengeTime))
	}

	cc.logger.Info("solution verified",
		slog.String("state", cc.state.String()),
		slog.Int("attempts", cc.attempts+1),
	)

	// Immediately send quote (transition to COMPLETED)
	return h.sendQuote(conn, cc)
}

// handleVerificationError processes solution verification failures.
func (h *Handler) handleVerificationError(conn net.Conn, cc *connectionContext, err error) (bool, error) {
	cc.attempts++

	cc.logger.Debug("verification failed",
		slog.String("error", err.Error()),
		slog.Int("attempt", cc.attempts),
		slog.Int("max_attempts", h.config.MaxAttempts),
	)

	// Determine error code based on error type
	var errCode protocol.ErrorCode

	switch {
	case errors.Is(err, pow.ErrInvalidSignature):
		errCode = protocol.ErrCodeInvalidSignature
		metrics.ChallengeFailed()
		// Invalid signature is fatal - close connection
		if sendErr := h.sendError(conn, errCode); sendErr != nil {
			return true, sendErr
		}
		return true, fmt.Errorf("invalid signature")

	case errors.Is(err, pow.ErrChallengeExpired):
		errCode = protocol.ErrCodeChallengeExpired
		metrics.ChallengeExpired()
		// Expired challenge is fatal - need new challenge
		if sendErr := h.sendError(conn, errCode); sendErr != nil {
			return true, sendErr
		}
		return true, fmt.Errorf("challenge expired")

	case errors.Is(err, pow.ErrFutureTimestamp):
		errCode = protocol.ErrCodeChallengeExpired
		metrics.ChallengeExpired()
		// Future timestamp treated as expired
		if sendErr := h.sendError(conn, errCode); sendErr != nil {
			return true, sendErr
		}
		return true, fmt.Errorf("future timestamp")

	case errors.Is(err, pow.ErrInsufficientWork):
		errCode = protocol.ErrCodeInvalidSolution
		metrics.ChallengeFailed()

		// Check if max attempts exceeded
		if cc.attempts >= h.config.MaxAttempts {
			cc.logger.Info("max attempts exceeded",
				slog.Int("attempts", cc.attempts),
			)
			if sendErr := h.sendError(conn, protocol.ErrCodeTooManyAttempts); sendErr != nil {
				return true, sendErr
			}
			return true, fmt.Errorf("max attempts exceeded")
		}

		// Allow retry
		if sendErr := h.sendError(conn, errCode); sendErr != nil {
			return true, sendErr
		}
		return false, nil // Continue - allow retry

	default:
		// Unknown error - treat as invalid message
		errCode = protocol.ErrCodeInvalidMessage
		metrics.ChallengeFailed()
		if sendErr := h.sendError(conn, errCode); sendErr != nil {
			return true, sendErr
		}
		return true, fmt.Errorf("verification error: %w", err)
	}
}

// handleAuthorized processes messages in the AUTHORIZED state.
func (h *Handler) handleAuthorized(ctx context.Context, conn net.Conn, cc *connectionContext, msg *protocol.Message) (bool, error) {
	// In AUTHORIZED state, quote should already have been sent
	// Any message here is unexpected
	cc.logger.Debug("unexpected message in authorized state",
		slog.String("type", msg.Type.String()),
	)
	return true, nil
}

// sendQuote sends a wisdom quote to the client.
func (h *Handler) sendQuote(conn net.Conn, cc *connectionContext) (bool, error) {
	quote := h.quotes.Random()
	quoteMsg := protocol.NewQuote(quote)

	if err := h.sendMessage(conn, quoteMsg); err != nil {
		return true, err
	}

	// Record quote served metric
	metrics.QuoteServed()

	cc.state = stateCompleted
	cc.logger.Info("quote sent",
		slog.String("state", cc.state.String()),
		slog.Int("quote_length", len(quote)),
	)

	return true, nil
}

// sendMessage sends a protocol message with write timeout.
func (h *Handler) sendMessage(conn net.Conn, msg *protocol.Message) error {
	if err := conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}

	if err := protocol.WriteMessage(conn, msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// sendError sends an error message to the client.
func (h *Handler) sendError(conn net.Conn, code protocol.ErrorCode) error {
	errMsg := protocol.NewError(code)
	return h.sendMessage(conn, errMsg)
}
