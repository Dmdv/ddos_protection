package pow

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	// contextCheckInterval defines how often to check for context cancellation.
	// Checking every 10K iterations balances responsiveness with performance.
	contextCheckInterval = 10000

	// DefaultMaxIterations is the default maximum number of iterations before giving up.
	// For difficulty 24, expected iterations are ~16.7M, so 1B gives plenty of margin.
	DefaultMaxIterations = 1_000_000_000
)

var (
	ErrMaxIterationsExceeded = errors.New("maximum iterations exceeded without finding solution")
)

// Solver solves PoW challenges by finding a valid counter.
type Solver interface {
	// Solve finds a counter that makes SHA256(challenge||counter) have the required leading zeros.
	// Returns the solution or an error if the context is cancelled or max iterations exceeded.
	Solve(ctx context.Context, challenge *Challenge) (*Solution, error)
}

// SolverConfig configures the solver behavior.
type SolverConfig struct {
	// MaxIterations is the maximum number of iterations before giving up.
	// Zero means use DefaultMaxIterations.
	MaxIterations uint64
}

// solverImpl implements the Solver interface.
type solverImpl struct {
	maxIterations uint64
}

// NewSolver creates a new PoW solver with default configuration.
func NewSolver() Solver {
	return NewSolverWithConfig(SolverConfig{})
}

// NewSolverWithConfig creates a new PoW solver with custom configuration.
func NewSolverWithConfig(cfg SolverConfig) Solver {
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = DefaultMaxIterations
	}
	return &solverImpl{
		maxIterations: maxIter,
	}
}

// Solve performs brute-force search to find a valid counter.
// It checks for context cancellation every 10K iterations for responsiveness.
func (s *solverImpl) Solve(ctx context.Context, challenge *Challenge) (*Solution, error) {
	// Pre-compute the challenge bytes
	challengeBytes := challenge.Marshal()

	// Prepare buffer for hashing: challenge (90 bytes) + counter (8 bytes)
	data := make([]byte, ChallengeSize+CounterSize)
	copy(data, challengeBytes)

	difficulty := int(challenge.Difficulty)
	var counter uint64

	for counter < s.maxIterations {
		// Check for context cancellation periodically
		if counter%contextCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Update counter in the data buffer
		binary.BigEndian.PutUint64(data[ChallengeSize:], counter)

		// Compute hash
		hash := sha256.Sum256(data)

		// Check if hash meets difficulty requirement
		if hasLeadingZeros(hash[:], difficulty) {
			return &Solution{
				Challenge: *challenge,
				Counter:   counter,
			}, nil
		}

		counter++
	}

	return nil, ErrMaxIterationsExceeded
}

// ProgressCallback is called periodically to report solving progress.
type ProgressCallback func(iterations uint64)

// SolverWithProgress provides progress reporting during solving.
type SolverWithProgress interface {
	Solver
	// SolveWithProgress finds a solution and reports progress.
	SolveWithProgress(ctx context.Context, challenge *Challenge, callback ProgressCallback) (*Solution, error)
}

// progressSolverImpl implements SolverWithProgress.
type progressSolverImpl struct {
	maxIterations  uint64
	reportInterval uint64
}

// NewSolverWithProgress creates a solver that reports progress.
func NewSolverWithProgress(reportInterval uint64) SolverWithProgress {
	return NewSolverWithProgressConfig(reportInterval, SolverConfig{})
}

// NewSolverWithProgressConfig creates a solver with progress reporting and custom config.
func NewSolverWithProgressConfig(reportInterval uint64, cfg SolverConfig) SolverWithProgress {
	if reportInterval == 0 {
		reportInterval = 100000
	}
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = DefaultMaxIterations
	}
	return &progressSolverImpl{
		maxIterations:  maxIter,
		reportInterval: reportInterval,
	}
}

// Solve implements the Solver interface.
func (s *progressSolverImpl) Solve(ctx context.Context, challenge *Challenge) (*Solution, error) {
	return s.SolveWithProgress(ctx, challenge, nil)
}

// SolveWithProgress finds a solution and reports progress.
func (s *progressSolverImpl) SolveWithProgress(ctx context.Context, challenge *Challenge, callback ProgressCallback) (*Solution, error) {
	challengeBytes := challenge.Marshal()

	data := make([]byte, ChallengeSize+CounterSize)
	copy(data, challengeBytes)

	difficulty := int(challenge.Difficulty)
	var counter uint64

	for counter < s.maxIterations {
		// Check for context cancellation periodically
		if counter%contextCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Report progress if callback is provided
		if callback != nil && counter > 0 && counter%s.reportInterval == 0 {
			callback(counter)
		}

		binary.BigEndian.PutUint64(data[ChallengeSize:], counter)
		hash := sha256.Sum256(data)

		if hasLeadingZeros(hash[:], difficulty) {
			return &Solution{
				Challenge: *challenge,
				Counter:   counter,
			}, nil
		}

		counter++
	}

	return nil, ErrMaxIterationsExceeded
}
