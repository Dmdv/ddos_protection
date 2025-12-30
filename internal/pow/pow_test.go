package pow

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-key-with-32-bytes!!!")

// mustNewGenerator creates a generator or panics (for tests only)
func mustNewGenerator(secret []byte) Generator {
	gen, err := NewGenerator(secret)
	if err != nil {
		panic(err)
	}
	return gen
}

// mustNewVerifier creates a verifier or panics (for tests only)
func mustNewVerifier(cfg VerifierConfig) Verifier {
	v, err := NewVerifier(cfg)
	if err != nil {
		panic(err)
	}
	return v
}

// TestChallenge_Marshal verifies challenge serialization
func TestChallenge_Marshal(t *testing.T) {
	c := &Challenge{
		Version:    1,
		Difficulty: 20,
		Timestamp:  1704067200, // 2024-01-01 00:00:00 UTC
	}
	copy(c.Resource[:], bytes.Repeat([]byte{0xAA}, 32))
	copy(c.Nonce[:], bytes.Repeat([]byte{0xBB}, 16))
	copy(c.Signature[:], bytes.Repeat([]byte{0xCC}, 32))

	data := c.Marshal()

	if len(data) != ChallengeSize {
		t.Errorf("Marshal() length = %d, want %d", len(data), ChallengeSize)
	}

	// Verify fields are at correct offsets
	if data[0] != 1 {
		t.Errorf("Version = %d, want 1", data[0])
	}
	if data[1] != 20 {
		t.Errorf("Difficulty = %d, want 20", data[1])
	}
}

// TestChallenge_Unmarshal verifies challenge deserialization
func TestChallenge_Unmarshal(t *testing.T) {
	original := &Challenge{
		Version:    1,
		Difficulty: 20,
		Timestamp:  1704067200,
	}
	copy(original.Resource[:], bytes.Repeat([]byte{0xAA}, 32))
	copy(original.Nonce[:], bytes.Repeat([]byte{0xBB}, 16))
	copy(original.Signature[:], bytes.Repeat([]byte{0xCC}, 32))

	data := original.Marshal()
	restored, err := UnmarshalChallenge(data)

	if err != nil {
		t.Fatalf("UnmarshalChallenge() error = %v", err)
	}

	if restored.Version != original.Version {
		t.Errorf("Version = %d, want %d", restored.Version, original.Version)
	}
	if restored.Difficulty != original.Difficulty {
		t.Errorf("Difficulty = %d, want %d", restored.Difficulty, original.Difficulty)
	}
	if restored.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %d, want %d", restored.Timestamp, original.Timestamp)
	}
	if !bytes.Equal(restored.Resource[:], original.Resource[:]) {
		t.Error("Resource mismatch")
	}
	if !bytes.Equal(restored.Nonce[:], original.Nonce[:]) {
		t.Error("Nonce mismatch")
	}
	if !bytes.Equal(restored.Signature[:], original.Signature[:]) {
		t.Error("Signature mismatch")
	}
}

// TestChallenge_RoundTrip verifies Marshal/Unmarshal are inverse operations
func TestChallenge_RoundTrip(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	for difficulty := uint8(MinDifficulty); difficulty <= MaxDifficulty; difficulty++ {
		original, err := gen.Generate(difficulty)
		if err != nil {
			t.Fatalf("Generate(%d) error = %v", difficulty, err)
		}

		data := original.Marshal()
		restored, err := UnmarshalChallenge(data)
		if err != nil {
			t.Fatalf("UnmarshalChallenge() error = %v", err)
		}

		if restored.Version != original.Version ||
			restored.Difficulty != original.Difficulty ||
			restored.Timestamp != original.Timestamp ||
			!bytes.Equal(restored.Resource[:], original.Resource[:]) ||
			!bytes.Equal(restored.Nonce[:], original.Nonce[:]) ||
			!bytes.Equal(restored.Signature[:], original.Signature[:]) {
			t.Errorf("Round-trip failed for difficulty %d", difficulty)
		}
	}
}

// TestUnmarshal_InvalidLength verifies error handling for invalid input
func TestUnmarshal_InvalidLength(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", make([]byte, ChallengeSize-1)},
		{"too long", make([]byte, ChallengeSize+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UnmarshalChallenge(tt.data)
			if err == nil {
				t.Error("UnmarshalChallenge() should return error for invalid length")
			}
		})
	}
}

// TestSolution_RoundTrip verifies solution serialization
func TestSolution_RoundTrip(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	challenge, err := gen.Generate(BaseDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	original := &Solution{
		Challenge: *challenge,
		Counter:   12345678,
	}

	data := original.Marshal()
	if len(data) != SolutionSize {
		t.Errorf("Solution.Marshal() length = %d, want %d", len(data), SolutionSize)
	}

	restored, err := UnmarshalSolution(data)
	if err != nil {
		t.Fatalf("UnmarshalSolution() error = %v", err)
	}

	if restored.Counter != original.Counter {
		t.Errorf("Counter = %d, want %d", restored.Counter, original.Counter)
	}
}

// TestGenerator_Generate verifies challenge generation
func TestGenerator_Generate(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	c, err := gen.Generate(BaseDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if c.Version != Version {
		t.Errorf("Version = %d, want %d", c.Version, Version)
	}
	if c.Difficulty != BaseDifficulty {
		t.Errorf("Difficulty = %d, want %d", c.Difficulty, BaseDifficulty)
	}

	// Timestamp should be recent (within 1 second)
	now := time.Now().Unix()
	if c.Timestamp < now-1 || c.Timestamp > now+1 {
		t.Errorf("Timestamp = %d, should be close to %d", c.Timestamp, now)
	}
}

// TestGenerator_Generate_UniqueNonces verifies nonces are unique
func TestGenerator_Generate_UniqueNonces(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	nonces := make(map[[16]byte]bool)

	for i := 0; i < 100; i++ {
		c, err := gen.Generate(BaseDifficulty)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if nonces[c.Nonce] {
			t.Errorf("Duplicate nonce generated")
		}
		nonces[c.Nonce] = true
	}
}

// TestVerifier_Verify_ValidSolution tests happy path verification
func TestVerifier_Verify_ValidSolution(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	solver := NewSolver()

	// Use low difficulty for fast testing
	challenge, err := gen.Generate(MinDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		t.Fatalf("Solve() error = %v", err)
	}

	if err := verifier.Verify(solution); err != nil {
		t.Errorf("Verify() error = %v", err)
	}
}

// TestVerifier_Verify_InvalidSignature tests signature validation
func TestVerifier_Verify_InvalidSignature(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	wrongSecret := []byte("wrong-secret-key-with-32-bytes!!!")
	verifier := mustNewVerifier(VerifierConfig{Secret: wrongSecret})

	challenge, _ := gen.Generate(MinDifficulty)

	solution := &Solution{
		Challenge: *challenge,
		Counter:   0, // Doesn't matter, should fail signature first
	}

	err := verifier.Verify(solution)
	if err != ErrInvalidSignature {
		t.Errorf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}

// TestVerifier_Verify_ExpiredChallenge tests timestamp validation
func TestVerifier_Verify_ExpiredChallenge(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	verifier := mustNewVerifier(VerifierConfig{
		Secret:           testSecret,
		ChallengeTimeout: 60 * time.Second,
		ClockSkew:        30 * time.Second,
	})

	challenge, _ := gen.Generate(MinDifficulty)
	// Make the challenge 2 minutes old (expired)
	challenge.Timestamp = time.Now().Unix() - 120

	// Re-sign with correct secret but old timestamp
	genImpl := gen.(*generatorImpl)
	genImpl.sign(challenge)

	solution := &Solution{
		Challenge: *challenge,
		Counter:   0,
	}

	err := verifier.Verify(solution)
	if err != ErrChallengeExpired {
		t.Errorf("Verify() error = %v, want ErrChallengeExpired", err)
	}
}

// TestVerifier_Verify_FutureTimestamp tests future timestamp rejection
func TestVerifier_Verify_FutureTimestamp(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	verifier := mustNewVerifier(VerifierConfig{
		Secret:    testSecret,
		ClockSkew: 30 * time.Second,
	})

	challenge, _ := gen.Generate(MinDifficulty)
	// Make the challenge 5 minutes in the future
	challenge.Timestamp = time.Now().Unix() + 300

	// Re-sign with correct secret but future timestamp
	genImpl := gen.(*generatorImpl)
	genImpl.sign(challenge)

	solution := &Solution{
		Challenge: *challenge,
		Counter:   0,
	}

	err := verifier.Verify(solution)
	if err != ErrFutureTimestamp {
		t.Errorf("Verify() error = %v, want ErrFutureTimestamp", err)
	}
}

// TestVerifier_Verify_InsufficientZeros tests difficulty validation
func TestVerifier_Verify_InsufficientZeros(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})

	challenge, _ := gen.Generate(BaseDifficulty)

	// Counter 0 is extremely unlikely to be a valid solution
	solution := &Solution{
		Challenge: *challenge,
		Counter:   0,
	}

	err := verifier.Verify(solution)
	if err != ErrInsufficientWork {
		t.Errorf("Verify() error = %v, want ErrInsufficientWork", err)
	}
}

// TestSolver_Solve tests the solver finds valid solutions
func TestSolver_Solve(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	solver := NewSolver()

	// Test with minimum difficulty for speed
	challenge, err := gen.Generate(MinDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		t.Fatalf("Solve() error = %v", err)
	}

	// Verify the solution is valid
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	if err := verifier.Verify(solution); err != nil {
		t.Errorf("Solution verification failed: %v", err)
	}
}

// TestSolver_Solve_Cancellation tests context cancellation
func TestSolver_Solve_Cancellation(t *testing.T) {
	gen := mustNewGenerator(testSecret)
	solver := NewSolver()

	// Use max difficulty to ensure we have time to cancel
	challenge, _ := gen.Generate(MaxDifficulty)

	// Create an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := solver.Solve(ctx, challenge)
	if err != context.Canceled {
		t.Errorf("Solve() error = %v, want context.Canceled", err)
	}
}

// TestHasLeadingZeros tests the leading zeros check
func TestHasLeadingZeros(t *testing.T) {
	tests := []struct {
		hash     []byte
		bits     int
		expected bool
	}{
		{[]byte{0x00, 0x00, 0x00, 0x00}, 0, true},
		{[]byte{0x00, 0x00, 0x00, 0x00}, 16, true},
		{[]byte{0x00, 0x00, 0x00, 0x00}, 32, true},
		{[]byte{0x01, 0x00, 0x00, 0x00}, 0, true},
		{[]byte{0x01, 0x00, 0x00, 0x00}, 7, true},
		{[]byte{0x01, 0x00, 0x00, 0x00}, 8, false},
		{[]byte{0x00, 0x01, 0x00, 0x00}, 15, true},
		{[]byte{0x00, 0x01, 0x00, 0x00}, 16, false},
		{[]byte{0x00, 0x0F, 0x00, 0x00}, 12, true},
		{[]byte{0x00, 0x0F, 0x00, 0x00}, 13, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := hasLeadingZeros(tt.hash, tt.bits)
			if result != tt.expected {
				t.Errorf("hasLeadingZeros(%x, %d) = %v, want %v", tt.hash, tt.bits, result, tt.expected)
			}
		})
	}
}

// TestChallenge_Validate tests field validation
func TestChallenge_Validate(t *testing.T) {
	tests := []struct {
		name       string
		version    uint8
		difficulty uint8
		wantErr    bool
	}{
		{"valid", 1, 20, false},
		{"valid min difficulty", 1, 16, false},
		{"valid max difficulty", 1, 24, false},
		{"invalid version", 2, 20, true},
		{"difficulty too low", 1, 15, true},
		{"difficulty too high", 1, 25, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Challenge{
				Version:    tt.version,
				Difficulty: tt.difficulty,
			}
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// BenchmarkVerifier_Verify measures verification performance
func BenchmarkVerifier_Verify(b *testing.B) {
	gen := mustNewGenerator(testSecret)
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	solver := NewSolver()

	challenge, _ := gen.Generate(MinDifficulty)
	ctx := context.Background()
	solution, _ := solver.Solve(ctx, challenge)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(solution)
	}
}

// BenchmarkSolver_Difficulty16 measures solve time for difficulty 16
func BenchmarkSolver_Difficulty16(b *testing.B) {
	gen := mustNewGenerator(testSecret)
	solver := NewSolver()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		challenge, _ := gen.Generate(16)
		ctx := context.Background()
		_, _ = solver.Solve(ctx, challenge)
	}
}

// BenchmarkSolver_Difficulty20 measures solve time for difficulty 20
func BenchmarkSolver_Difficulty20(b *testing.B) {
	gen := mustNewGenerator(testSecret)
	solver := NewSolver()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		challenge, _ := gen.Generate(20)
		ctx := context.Background()
		_, _ = solver.Solve(ctx, challenge)
	}
}

// BenchmarkHasLeadingZeros measures leading zeros check performance
func BenchmarkHasLeadingZeros(b *testing.B) {
	hash := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		hasLeadingZeros(hash, 20)
	}
}

// TestNewGenerator_SecretValidation tests secret length validation
func TestNewGenerator_SecretValidation(t *testing.T) {
	tests := []struct {
		name      string
		secret    []byte
		wantErr   bool
		errTarget error
	}{
		{"valid 32 bytes", make([]byte, 32), false, nil},
		{"valid 64 bytes", make([]byte, 64), false, nil},
		{"too short 31 bytes", make([]byte, 31), true, ErrSecretTooShort},
		{"too short 16 bytes", make([]byte, 16), true, ErrSecretTooShort},
		{"empty secret", []byte{}, true, ErrSecretTooShort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewGenerator(tt.secret)
			if tt.wantErr {
				if err == nil {
					t.Error("NewGenerator() should return error")
				}
				if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
					t.Errorf("NewGenerator() error = %v, want %v", err, tt.errTarget)
				}
				if gen != nil {
					t.Error("NewGenerator() should return nil generator on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewGenerator() unexpected error = %v", err)
				}
				if gen == nil {
					t.Error("NewGenerator() should return non-nil generator")
				}
			}
		})
	}
}

// TestNewVerifier_SecretValidation tests secret length validation
func TestNewVerifier_SecretValidation(t *testing.T) {
	tests := []struct {
		name      string
		secret    []byte
		wantErr   bool
		errTarget error
	}{
		{"valid 32 bytes", make([]byte, 32), false, nil},
		{"valid 64 bytes", make([]byte, 64), false, nil},
		{"too short 31 bytes", make([]byte, 31), true, ErrSecretTooShort},
		{"too short 16 bytes", make([]byte, 16), true, ErrSecretTooShort},
		{"empty secret", []byte{}, true, ErrSecretTooShort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewVerifier(VerifierConfig{Secret: tt.secret})
			if tt.wantErr {
				if err == nil {
					t.Error("NewVerifier() should return error")
				}
				if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
					t.Errorf("NewVerifier() error = %v, want %v", err, tt.errTarget)
				}
				if v != nil {
					t.Error("NewVerifier() should return nil verifier on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewVerifier() unexpected error = %v", err)
				}
				if v == nil {
					t.Error("NewVerifier() should return non-nil verifier")
				}
			}
		})
	}
}

// TestGenerator_Generate_DifficultyValidation tests difficulty bounds validation
func TestGenerator_Generate_DifficultyValidation(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	tests := []struct {
		name       string
		difficulty uint8
		wantErr    bool
	}{
		{"valid min", MinDifficulty, false},
		{"valid base", BaseDifficulty, false},
		{"valid max", MaxDifficulty, false},
		{"too low", MinDifficulty - 1, true},
		{"too high", MaxDifficulty + 1, true},
		{"zero", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			challenge, err := gen.Generate(tt.difficulty)
			if tt.wantErr {
				if err == nil {
					t.Error("Generate() should return error for invalid difficulty")
				}
				if !errors.Is(err, ErrInvalidDifficulty) {
					t.Errorf("Generate() error = %v, want ErrInvalidDifficulty", err)
				}
				if challenge != nil {
					t.Error("Generate() should return nil challenge on error")
				}
			} else {
				if err != nil {
					t.Errorf("Generate() unexpected error = %v", err)
				}
				if challenge == nil {
					t.Error("Generate() should return non-nil challenge")
				}
			}
		})
	}
}

// TestSolverWithConfig tests custom solver configuration
func TestSolverWithConfig(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	tests := []struct {
		name          string
		maxIterations uint64
	}{
		{"default", 0},
		{"custom", 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			solver := NewSolverWithConfig(SolverConfig{MaxIterations: tt.maxIterations})
			if solver == nil {
				t.Fatal("NewSolverWithConfig() returned nil")
			}

			challenge, err := gen.Generate(MinDifficulty)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			solution, err := solver.Solve(ctx, challenge)
			if err != nil {
				t.Fatalf("Solve() error = %v", err)
			}

			verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
			if err := verifier.Verify(solution); err != nil {
				t.Errorf("Solution verification failed: %v", err)
			}
		})
	}
}

// TestSolverWithProgress tests progress reporting solver
func TestSolverWithProgress(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	solver := NewSolverWithProgress(10000)
	if solver == nil {
		t.Fatal("NewSolverWithProgress() returned nil")
	}

	challenge, err := gen.Generate(MinDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var progressCalls int
	var lastProgress uint64

	solution, err := solver.SolveWithProgress(ctx, challenge, func(iterations uint64) {
		progressCalls++
		lastProgress = iterations
	})

	if err != nil {
		t.Fatalf("SolveWithProgress() error = %v", err)
	}

	// Verify solution is valid
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	if err := verifier.Verify(solution); err != nil {
		t.Errorf("Solution verification failed: %v", err)
	}

	// If solution took more than reportInterval iterations, we should have progress calls
	if solution.Counter > 10000 && progressCalls == 0 {
		t.Logf("Expected progress calls for counter %d", solution.Counter)
	}

	t.Logf("Progress calls: %d, last progress: %d, counter: %d", progressCalls, lastProgress, solution.Counter)
}

// TestSolverWithProgress_Solve tests the Solver interface implementation
func TestSolverWithProgress_Solve(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	solver := NewSolverWithProgress(10000)
	challenge, err := gen.Generate(MinDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test that Solve() works (calls SolveWithProgress with nil callback)
	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		t.Fatalf("Solve() error = %v", err)
	}

	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	if err := verifier.Verify(solution); err != nil {
		t.Errorf("Solution verification failed: %v", err)
	}
}

// TestSolverWithProgress_Cancellation tests context cancellation
func TestSolverWithProgress_Cancellation(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	solver := NewSolverWithProgress(10000)
	challenge, _ := gen.Generate(MaxDifficulty)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := solver.SolveWithProgress(ctx, challenge, nil)
	if err != context.Canceled {
		t.Errorf("SolveWithProgress() error = %v, want context.Canceled", err)
	}
}

// TestSolverWithProgressConfig tests custom config with progress solver
func TestSolverWithProgressConfig(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	solver := NewSolverWithProgressConfig(50000, SolverConfig{MaxIterations: 10000000})
	if solver == nil {
		t.Fatal("NewSolverWithProgressConfig() returned nil")
	}

	challenge, err := gen.Generate(MinDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		t.Fatalf("Solve() error = %v", err)
	}

	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})
	if err := verifier.Verify(solution); err != nil {
		t.Errorf("Solution verification failed: %v", err)
	}
}

// TestSolver_MaxIterationsExceeded tests iteration limit enforcement
func TestSolver_MaxIterationsExceeded(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	// Create solver with very low max iterations
	solver := NewSolverWithConfig(SolverConfig{MaxIterations: 10})

	challenge, err := gen.Generate(MaxDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx := context.Background()
	_, err = solver.Solve(ctx, challenge)

	if !errors.Is(err, ErrMaxIterationsExceeded) {
		t.Errorf("Solve() error = %v, want ErrMaxIterationsExceeded", err)
	}
}

// TestSolverWithProgress_MaxIterationsExceeded tests iteration limit in progress solver
func TestSolverWithProgress_MaxIterationsExceeded(t *testing.T) {
	gen := mustNewGenerator(testSecret)

	solver := NewSolverWithProgressConfig(5, SolverConfig{MaxIterations: 10})

	challenge, err := gen.Generate(MaxDifficulty)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	ctx := context.Background()
	_, err = solver.SolveWithProgress(ctx, challenge, nil)

	if !errors.Is(err, ErrMaxIterationsExceeded) {
		t.Errorf("SolveWithProgress() error = %v, want ErrMaxIterationsExceeded", err)
	}
}

// TestVerifier_InvalidDifficulty tests verifier handling of invalid difficulty
func TestVerifier_InvalidDifficulty(t *testing.T) {
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})

	// Create a challenge with valid signature but invalid difficulty
	gen := mustNewGenerator(testSecret)
	challenge, _ := gen.Generate(MinDifficulty)

	// Manually set invalid difficulty (bypassing generator validation)
	challenge.Difficulty = MinDifficulty - 1

	// Re-sign to make signature valid
	genImpl := gen.(*generatorImpl)
	genImpl.sign(challenge)

	solution := &Solution{
		Challenge: *challenge,
		Counter:   0,
	}

	err := verifier.Verify(solution)
	if !errors.Is(err, ErrInvalidDifficulty) {
		t.Errorf("Verify() error = %v, want ErrInvalidDifficulty", err)
	}
}

// TestVerifier_InvalidVersion tests verifier handling of invalid version
func TestVerifier_InvalidVersion(t *testing.T) {
	verifier := mustNewVerifier(VerifierConfig{Secret: testSecret})

	gen := mustNewGenerator(testSecret)
	challenge, _ := gen.Generate(MinDifficulty)

	// Set invalid version
	challenge.Version = 99

	// Re-sign
	genImpl := gen.(*generatorImpl)
	genImpl.sign(challenge)

	solution := &Solution{
		Challenge: *challenge,
		Counter:   0,
	}

	err := verifier.Verify(solution)
	if !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("Verify() error = %v, want ErrInvalidVersion", err)
	}
}

// TestUnmarshalSolution_InvalidLength tests solution unmarshaling error handling
func TestUnmarshalSolution_InvalidLength(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", make([]byte, SolutionSize-1)},
		{"too long", make([]byte, SolutionSize+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UnmarshalSolution(tt.data)
			if err == nil {
				t.Error("UnmarshalSolution() should return error for invalid length")
			}
		})
	}
}
