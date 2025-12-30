package pow

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

const (
	// MinSecretLength is the minimum required length for HMAC secret.
	MinSecretLength = 32
)

var (
	ErrSecretTooShort = errors.New("HMAC secret must be at least 32 bytes")
)

// Generator creates PoW challenges.
type Generator interface {
	// Generate creates a new challenge with the specified difficulty.
	Generate(difficulty uint8) (*Challenge, error)
}

// generatorImpl implements the Generator interface.
type generatorImpl struct {
	secret []byte
}

// NewGenerator creates a new challenge generator with the given HMAC secret.
// Returns error if secret is less than 32 bytes.
func NewGenerator(secret []byte) (Generator, error) {
	if len(secret) < MinSecretLength {
		return nil, fmt.Errorf("%w: got %d bytes", ErrSecretTooShort, len(secret))
	}
	return &generatorImpl{
		secret: secret,
	}, nil
}

// Generate creates a new challenge with cryptographically random nonce.
// Returns error if difficulty is outside valid range [16, 24].
func (g *generatorImpl) Generate(difficulty uint8) (*Challenge, error) {
	// Validate difficulty bounds
	if difficulty < MinDifficulty || difficulty > MaxDifficulty {
		return nil, fmt.Errorf("%w: got %d, want %d-%d", ErrInvalidDifficulty, difficulty, MinDifficulty, MaxDifficulty)
	}

	c := &Challenge{
		Version:    Version,
		Difficulty: difficulty,
		Timestamp:  time.Now().Unix(),
	}

	// Generate random resource (server identifier)
	if _, err := rand.Read(c.Resource[:]); err != nil {
		return nil, fmt.Errorf("failed to generate resource: %w", err)
	}

	// Generate random nonce
	if _, err := rand.Read(c.Nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Sign the challenge with HMAC-SHA256
	g.sign(c)

	return c, nil
}

// sign computes the HMAC-SHA256 signature over the unsigned challenge fields.
func (g *generatorImpl) sign(c *Challenge) {
	data := c.MarshalUnsigned()
	mac := hmac.New(sha256.New, g.secret)
	mac.Write(data)
	copy(c.Signature[:], mac.Sum(nil))
}
