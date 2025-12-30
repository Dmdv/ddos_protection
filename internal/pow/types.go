// Package pow implements Proof of Work challenge-response protocol using Hashcash with SHA-256.
package pow

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// Protocol version
	Version = 1

	// Difficulty levels (leading zero bits)
	MinDifficulty  = 16 // Low load, friendly to all clients
	BaseDifficulty = 20 // Normal operation (~1.6s solve time)
	MaxDifficulty  = 24 // Under attack (~25s solve time, max for 60s timeout)

	// Challenge wire format sizes
	ChallengeSize = 90 // Total challenge size in bytes
	SolutionSize  = 98 // Challenge (90) + Counter (8)

	// Field sizes
	VersionSize    = 1
	DifficultySize = 1
	TimestampSize  = 8
	ResourceSize   = 32
	NonceSize      = 16
	SignatureSize  = 32
	CounterSize    = 8
)

var (
	ErrInvalidChallengeLength = errors.New("invalid challenge length")
	ErrInvalidSolutionLength  = errors.New("invalid solution length")
	ErrInvalidVersion         = errors.New("invalid protocol version")
	ErrInvalidDifficulty      = errors.New("invalid difficulty level")
)

// Challenge represents a PoW challenge sent by the server.
// Wire format: Version(1) + Difficulty(1) + Timestamp(8) + Resource(32) + Nonce(16) + Signature(32) = 90 bytes
type Challenge struct {
	Version    uint8    // Protocol version (1)
	Difficulty uint8    // Required leading zero bits (16-24)
	Timestamp  int64    // Unix timestamp (seconds)
	Resource   [32]byte // Server random identifier
	Nonce      [16]byte // Per-challenge random nonce
	Signature  [32]byte // HMAC-SHA256(secret, Version||Difficulty||Timestamp||Resource||Nonce)
}

// Solution represents a client's solution to a challenge.
// Wire format: Challenge(90) + Counter(8) = 98 bytes
type Solution struct {
	Challenge Challenge // Original challenge
	Counter   uint64    // Solution counter found by client
}

// Marshal serializes the challenge to bytes in big-endian format.
func (c *Challenge) Marshal() []byte {
	buf := make([]byte, ChallengeSize)
	offset := 0

	buf[offset] = c.Version
	offset += VersionSize

	buf[offset] = c.Difficulty
	offset += DifficultySize

	binary.BigEndian.PutUint64(buf[offset:], uint64(c.Timestamp))
	offset += TimestampSize

	copy(buf[offset:], c.Resource[:])
	offset += ResourceSize

	copy(buf[offset:], c.Nonce[:])
	offset += NonceSize

	copy(buf[offset:], c.Signature[:])

	return buf
}

// MarshalUnsigned returns the unsigned portion of the challenge for HMAC computation.
// Format: Version||Difficulty||Timestamp||Resource||Nonce (58 bytes)
func (c *Challenge) MarshalUnsigned() []byte {
	size := VersionSize + DifficultySize + TimestampSize + ResourceSize + NonceSize
	buf := make([]byte, size)
	offset := 0

	buf[offset] = c.Version
	offset += VersionSize

	buf[offset] = c.Difficulty
	offset += DifficultySize

	binary.BigEndian.PutUint64(buf[offset:], uint64(c.Timestamp))
	offset += TimestampSize

	copy(buf[offset:], c.Resource[:])
	offset += ResourceSize

	copy(buf[offset:], c.Nonce[:])

	return buf
}

// UnmarshalChallenge deserializes a challenge from bytes.
func UnmarshalChallenge(data []byte) (*Challenge, error) {
	if len(data) != ChallengeSize {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrInvalidChallengeLength, len(data), ChallengeSize)
	}

	c := &Challenge{}
	offset := 0

	c.Version = data[offset]
	offset += VersionSize

	c.Difficulty = data[offset]
	offset += DifficultySize

	c.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += TimestampSize

	copy(c.Resource[:], data[offset:offset+ResourceSize])
	offset += ResourceSize

	copy(c.Nonce[:], data[offset:offset+NonceSize])
	offset += NonceSize

	copy(c.Signature[:], data[offset:offset+SignatureSize])

	return c, nil
}

// Marshal serializes the solution to bytes.
func (s *Solution) Marshal() []byte {
	buf := make([]byte, SolutionSize)
	copy(buf, s.Challenge.Marshal())
	binary.BigEndian.PutUint64(buf[ChallengeSize:], s.Counter)
	return buf
}

// UnmarshalSolution deserializes a solution from bytes.
func UnmarshalSolution(data []byte) (*Solution, error) {
	if len(data) != SolutionSize {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrInvalidSolutionLength, len(data), SolutionSize)
	}

	challenge, err := UnmarshalChallenge(data[:ChallengeSize])
	if err != nil {
		return nil, err
	}

	return &Solution{
		Challenge: *challenge,
		Counter:   binary.BigEndian.Uint64(data[ChallengeSize:]),
	}, nil
}

// Validate checks if the challenge has valid field values.
func (c *Challenge) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("%w: got %d, want %d", ErrInvalidVersion, c.Version, Version)
	}
	if c.Difficulty < MinDifficulty || c.Difficulty > MaxDifficulty {
		return fmt.Errorf("%w: got %d, want %d-%d", ErrInvalidDifficulty, c.Difficulty, MinDifficulty, MaxDifficulty)
	}
	return nil
}
