package pow

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid challenge signature")
	ErrChallengeExpired = errors.New("challenge expired")
	ErrFutureTimestamp  = errors.New("challenge timestamp is in the future")
	ErrInsufficientWork = errors.New("solution does not meet difficulty requirement")
)

// Verifier verifies PoW solutions.
type Verifier interface {
	// Verify checks if the solution is valid.
	// Returns nil if valid, or an error describing the failure.
	Verify(solution *Solution) error
}

// VerifierConfig holds configuration for the verifier.
type VerifierConfig struct {
	Secret           []byte
	ChallengeTimeout time.Duration // Maximum age of challenge (default: 60s)
	ClockSkew        time.Duration // Tolerance for clock differences (default: 30s)
}

// verifierImpl implements the Verifier interface.
type verifierImpl struct {
	secret           []byte
	challengeTimeout time.Duration
	clockSkew        time.Duration
}

// NewVerifier creates a new solution verifier.
// Returns error if secret is less than 32 bytes.
func NewVerifier(cfg VerifierConfig) (Verifier, error) {
	if len(cfg.Secret) < MinSecretLength {
		return nil, ErrSecretTooShort
	}

	timeout := cfg.ChallengeTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	skew := cfg.ClockSkew
	if skew == 0 {
		skew = 30 * time.Second
	}

	return &verifierImpl{
		secret:           cfg.Secret,
		challengeTimeout: timeout,
		clockSkew:        skew,
	}, nil
}

// Verify performs 3-step validation:
// 1. HMAC signature check
// 2. Timestamp validation
// 3. Leading zeros check
func (v *verifierImpl) Verify(solution *Solution) error {
	challenge := &solution.Challenge

	// Step 1: Validate challenge fields
	if err := challenge.Validate(); err != nil {
		return err
	}

	// Step 2: Verify HMAC signature
	if !v.verifySignature(challenge) {
		return ErrInvalidSignature
	}

	// Step 3: Check timestamp freshness
	if err := v.validateTimestamp(challenge.Timestamp); err != nil {
		return err
	}

	// Step 4: Verify proof of work (leading zeros)
	if !v.hasRequiredLeadingZeros(solution) {
		return ErrInsufficientWork
	}

	return nil
}

// verifySignature checks if the challenge signature is valid.
func (v *verifierImpl) verifySignature(c *Challenge) bool {
	data := c.MarshalUnsigned()
	mac := hmac.New(sha256.New, v.secret)
	mac.Write(data)
	expected := mac.Sum(nil)
	return hmac.Equal(c.Signature[:], expected)
}

// validateTimestamp checks if the challenge is within acceptable time bounds.
func (v *verifierImpl) validateTimestamp(challengeTime int64) error {
	now := time.Now().Unix()
	age := now - challengeTime

	// Too old (expired)
	maxAge := int64(v.challengeTimeout.Seconds()) + int64(v.clockSkew.Seconds())
	if age > maxAge {
		return ErrChallengeExpired
	}

	// Too new (clock skew / potential forgery)
	if age < -int64(v.clockSkew.Seconds()) {
		return ErrFutureTimestamp
	}

	return nil
}

// hasRequiredLeadingZeros checks if SHA256(challenge||counter) has required leading zero bits.
func (v *verifierImpl) hasRequiredLeadingZeros(solution *Solution) bool {
	// Compute hash of challenge + counter
	data := make([]byte, ChallengeSize+CounterSize)
	copy(data, solution.Challenge.Marshal())
	binary.BigEndian.PutUint64(data[ChallengeSize:], solution.Counter)

	hash := sha256.Sum256(data)

	return hasLeadingZeros(hash[:], int(solution.Challenge.Difficulty))
}

// hasLeadingZeros checks if the hash has at least n leading zero bits.
func hasLeadingZeros(hash []byte, n int) bool {
	// Check full bytes
	fullBytes := n / 8
	for i := 0; i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}

	// Check remaining bits
	remainingBits := n % 8
	if remainingBits > 0 {
		mask := byte(0xFF << (8 - remainingBits))
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}

	return true
}
