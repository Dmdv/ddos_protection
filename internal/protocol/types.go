package protocol

// Protocol constants
const (
	// MaxMessageSize is the maximum allowed message size (header + payload)
	MaxMessageSize = 1024

	// HeaderSize is the size of the message header (length + type)
	HeaderSize = 3

	// LengthSize is the size of the length field
	LengthSize = 2

	// MaxPayloadSize is the maximum payload size
	MaxPayloadSize = MaxMessageSize - HeaderSize
)

// MessageType identifies the type of protocol message.
type MessageType uint8

// Message types as defined in API_REFERENCE.md Section 2.2
const (
	// TypeRequestChallenge is sent by client to request a new PoW challenge.
	TypeRequestChallenge MessageType = 0x01

	// TypeChallenge is sent by server containing the PoW challenge.
	TypeChallenge MessageType = 0x02

	// TypeSolution is sent by client containing the solved challenge.
	TypeSolution MessageType = 0x03

	// TypeQuote is sent by server containing the wisdom quote.
	TypeQuote MessageType = 0x04

	// TypeError is sent by server to report an error condition.
	TypeError MessageType = 0x05
)

// String returns a human-readable name for the message type.
func (t MessageType) String() string {
	switch t {
	case TypeRequestChallenge:
		return "REQUEST_CHALLENGE"
	case TypeChallenge:
		return "CHALLENGE"
	case TypeSolution:
		return "SOLUTION"
	case TypeQuote:
		return "QUOTE"
	case TypeError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// IsValid returns true if the message type is a known type.
func (t MessageType) IsValid() bool {
	return t >= TypeRequestChallenge && t <= TypeError
}

// ErrorCode represents a protocol error code.
type ErrorCode uint8

// Error codes as defined in API_REFERENCE.md Section 4
const (
	// ErrCodeInvalidMessage indicates a malformed message.
	ErrCodeInvalidMessage ErrorCode = 0x01

	// ErrCodeInvalidState indicates wrong message sequence.
	ErrCodeInvalidState ErrorCode = 0x02

	// ErrCodeChallengeExpired indicates the challenge timestamp is too old.
	ErrCodeChallengeExpired ErrorCode = 0x03

	// ErrCodeInvalidSignature indicates HMAC verification failed.
	ErrCodeInvalidSignature ErrorCode = 0x04

	// ErrCodeInvalidSolution indicates hash doesn't meet difficulty.
	ErrCodeInvalidSolution ErrorCode = 0x05

	// ErrCodeTooManyAttempts indicates exceeded 3 solution attempts.
	ErrCodeTooManyAttempts ErrorCode = 0x06

	// ErrCodeRateLimited indicates too many requests from this IP.
	ErrCodeRateLimited ErrorCode = 0x07

	// ErrCodeServerBusy indicates connection limit reached.
	ErrCodeServerBusy ErrorCode = 0x08
)

// String returns a human-readable name for the error code.
func (e ErrorCode) String() string {
	switch e {
	case ErrCodeInvalidMessage:
		return "ERR_INVALID_MESSAGE"
	case ErrCodeInvalidState:
		return "ERR_INVALID_STATE"
	case ErrCodeChallengeExpired:
		return "ERR_CHALLENGE_EXPIRED"
	case ErrCodeInvalidSignature:
		return "ERR_INVALID_SIGNATURE"
	case ErrCodeInvalidSolution:
		return "ERR_INVALID_SOLUTION"
	case ErrCodeTooManyAttempts:
		return "ERR_TOO_MANY_ATTEMPTS"
	case ErrCodeRateLimited:
		return "ERR_RATE_LIMITED"
	case ErrCodeServerBusy:
		return "ERR_SERVER_BUSY"
	default:
		return "ERR_UNKNOWN"
	}
}

// IsValid returns true if the error code is a known code.
func (e ErrorCode) IsValid() bool {
	return e >= ErrCodeInvalidMessage && e <= ErrCodeServerBusy
}

// IsRetryable returns true if the error is transient and client should retry.
func (e ErrorCode) IsRetryable() bool {
	switch e {
	case ErrCodeInvalidSolution: // Can retry up to 3 times
		return true
	case ErrCodeRateLimited: // Wait and retry
		return true
	case ErrCodeServerBusy: // Wait and retry
		return true
	default:
		return false
	}
}

// Description returns a human-readable description of the error.
func (e ErrorCode) Description() string {
	switch e {
	case ErrCodeInvalidMessage:
		return "malformed message"
	case ErrCodeInvalidState:
		return "wrong message sequence"
	case ErrCodeChallengeExpired:
		return "challenge timestamp too old"
	case ErrCodeInvalidSignature:
		return "HMAC verification failed"
	case ErrCodeInvalidSolution:
		return "hash doesn't meet difficulty requirement"
	case ErrCodeTooManyAttempts:
		return "exceeded maximum solution attempts"
	case ErrCodeRateLimited:
		return "too many requests from this IP"
	case ErrCodeServerBusy:
		return "server connection limit reached"
	default:
		return "unknown error"
	}
}
