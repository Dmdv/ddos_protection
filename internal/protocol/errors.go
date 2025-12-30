package protocol

import (
	"errors"
	"fmt"
)

// ProtocolError represents an error received from the server.
type ProtocolError struct {
	Code ErrorCode
}

// Error implements the error interface.
func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error: %s - %s", e.Code, e.Code.Description())
}

// Is implements errors.Is for ProtocolError.
func (e *ProtocolError) Is(target error) bool {
	var pe *ProtocolError
	if errors.As(target, &pe) {
		return e.Code == pe.Code
	}
	return false
}

// Sentinel protocol errors for common cases
var (
	ErrInvalidMessage   = &ProtocolError{Code: ErrCodeInvalidMessage}
	ErrInvalidState     = &ProtocolError{Code: ErrCodeInvalidState}
	ErrChallengeExpired = &ProtocolError{Code: ErrCodeChallengeExpired}
	ErrInvalidSignature = &ProtocolError{Code: ErrCodeInvalidSignature}
	ErrInvalidSolution  = &ProtocolError{Code: ErrCodeInvalidSolution}
	ErrTooManyAttempts  = &ProtocolError{Code: ErrCodeTooManyAttempts}
	ErrRateLimited      = &ProtocolError{Code: ErrCodeRateLimited}
	ErrServerBusy       = &ProtocolError{Code: ErrCodeServerBusy}
)

// NewProtocolError creates a ProtocolError from an error code.
func NewProtocolError(code ErrorCode) *ProtocolError {
	return &ProtocolError{Code: code}
}

// ErrorFromCode returns the sentinel error for a given error code.
func ErrorFromCode(code ErrorCode) error {
	switch code {
	case ErrCodeInvalidMessage:
		return ErrInvalidMessage
	case ErrCodeInvalidState:
		return ErrInvalidState
	case ErrCodeChallengeExpired:
		return ErrChallengeExpired
	case ErrCodeInvalidSignature:
		return ErrInvalidSignature
	case ErrCodeInvalidSolution:
		return ErrInvalidSolution
	case ErrCodeTooManyAttempts:
		return ErrTooManyAttempts
	case ErrCodeRateLimited:
		return ErrRateLimited
	case ErrCodeServerBusy:
		return ErrServerBusy
	default:
		return NewProtocolError(code)
	}
}

// IsRetryableError returns true if the error is transient and the client should retry.
func IsRetryableError(err error) bool {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		return pe.Code.IsRetryable()
	}
	return false
}

// ShouldReconnect returns true if the client should reconnect for this error.
// Some errors require getting a new challenge.
func ShouldReconnect(err error) bool {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		switch pe.Code {
		case ErrCodeInvalidMessage,
			ErrCodeInvalidState,
			ErrCodeChallengeExpired,
			ErrCodeInvalidSignature,
			ErrCodeTooManyAttempts:
			return true
		default:
			return false
		}
	}
	return false
}

// ParseErrorMessage extracts a ProtocolError from an ERROR message.
// Returns nil if the message is not an ERROR message or has invalid format.
func ParseErrorMessage(m *Message) error {
	if m.Type != TypeError {
		return nil
	}
	if len(m.Payload) < 1 {
		return ErrInvalidMessage
	}
	code := ErrorCode(m.Payload[0])
	return ErrorFromCode(code)
}
