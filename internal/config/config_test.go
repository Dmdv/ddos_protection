package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.ServerAddress != ":8080" {
		t.Errorf("expected ServerAddress=:8080, got %s", cfg.ServerAddress)
	}
	if cfg.MetricsAddress != ":9090" {
		t.Errorf("expected MetricsAddress=:9090, got %s", cfg.MetricsAddress)
	}
	if cfg.MaxConnections != 10000 {
		t.Errorf("expected MaxConnections=10000, got %d", cfg.MaxConnections)
	}
	if cfg.PowDifficulty != 20 {
		t.Errorf("expected PowDifficulty=20, got %d", cfg.PowDifficulty)
	}
	if cfg.PowMinDifficulty != 16 {
		t.Errorf("expected PowMinDifficulty=16, got %d", cfg.PowMinDifficulty)
	}
	if cfg.PowMaxDifficulty != 24 {
		t.Errorf("expected PowMaxDifficulty=24, got %d", cfg.PowMaxDifficulty)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=info, got %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("expected LogFormat=json, got %s", cfg.LogFormat)
	}
}

func TestLoad_WithEnvVars(t *testing.T) {
	// Save original env and restore after test
	original := map[string]string{
		"SERVER_ADDRESS":  os.Getenv("SERVER_ADDRESS"),
		"MAX_CONNECTIONS": os.Getenv("MAX_CONNECTIONS"),
		"POW_SECRET":      os.Getenv("POW_SECRET"),
		"POW_DIFFICULTY":  os.Getenv("POW_DIFFICULTY"),
		"READ_TIMEOUT":    os.Getenv("READ_TIMEOUT"),
		"LOG_LEVEL":       os.Getenv("LOG_LEVEL"),
	}
	defer func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	os.Setenv("SERVER_ADDRESS", ":9999")
	os.Setenv("MAX_CONNECTIONS", "5000")
	os.Setenv("POW_SECRET", "test-secret-key-32-bytes-long!!!")
	os.Setenv("POW_DIFFICULTY", "18")
	os.Setenv("READ_TIMEOUT", "5s")
	os.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServerAddress != ":9999" {
		t.Errorf("expected ServerAddress=:9999, got %s", cfg.ServerAddress)
	}
	if cfg.MaxConnections != 5000 {
		t.Errorf("expected MaxConnections=5000, got %d", cfg.MaxConnections)
	}
	if string(cfg.PowSecret) != "test-secret-key-32-bytes-long!!!" {
		t.Errorf("expected PowSecret to match, got %s", string(cfg.PowSecret))
	}
	if cfg.PowDifficulty != 18 {
		t.Errorf("expected PowDifficulty=18, got %d", cfg.PowDifficulty)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("expected ReadTimeout=5s, got %v", cfg.ReadTimeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel=debug, got %s", cfg.LogLevel)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		value   string
		wantErr string
	}{
		{
			name:    "invalid MAX_CONNECTIONS",
			envVar:  "MAX_CONNECTIONS",
			value:   "not-a-number",
			wantErr: "invalid MAX_CONNECTIONS",
		},
		{
			name:    "invalid POW_DIFFICULTY",
			envVar:  "POW_DIFFICULTY",
			value:   "abc",
			wantErr: "invalid POW_DIFFICULTY",
		},
		{
			name:    "invalid READ_TIMEOUT",
			envVar:  "READ_TIMEOUT",
			value:   "not-duration",
			wantErr: "invalid READ_TIMEOUT",
		},
		{
			name:    "invalid RATE_LIMIT_RPS",
			envVar:  "RATE_LIMIT_RPS",
			value:   "xyz",
			wantErr: "invalid RATE_LIMIT_RPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := os.Getenv(tt.envVar)
			defer func() {
				if original == "" {
					os.Unsetenv(tt.envVar)
				} else {
					os.Setenv(tt.envVar, original)
				}
			}()

			os.Setenv(tt.envVar, tt.value)

			_, err := Load()
			if err == nil {
				t.Error("expected error")
				return
			}
			if err.Error()[:len(tt.wantErr)] != tt.wantErr {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestValidate_MissingPowSecret(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = nil

	err := cfg.Validate()
	if err != ErrMissingPowSecret {
		t.Errorf("expected ErrMissingPowSecret, got %v", err)
	}
}

func TestValidate_PowSecretTooShort(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("short")

	err := cfg.Validate()
	if err != ErrPowSecretTooShort {
		t.Errorf("expected ErrPowSecretTooShort, got %v", err)
	}
}

func TestValidate_InvalidDifficultyRange(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
	cfg.PowMinDifficulty = 24
	cfg.PowMaxDifficulty = 16 // Min > Max

	err := cfg.Validate()
	if err != ErrInvalidDifficultyRange {
		t.Errorf("expected ErrInvalidDifficultyRange, got %v", err)
	}
}

func TestValidate_DifficultyOutOfRange(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
	cfg.PowDifficulty = 10 // Below min (16)

	err := cfg.Validate()
	if err != ErrDifficultyOutOfRange {
		t.Errorf("expected ErrDifficultyOutOfRange, got %v", err)
	}

	cfg.PowDifficulty = 30 // Above max (24)
	err = cfg.Validate()
	if err != ErrDifficultyOutOfRange {
		t.Errorf("expected ErrDifficultyOutOfRange, got %v", err)
	}
}

func TestValidate_NegativeTimeout(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")

	cfg.ReadTimeout = 0
	err := cfg.Validate()
	if err != ErrNegativeTimeout {
		t.Errorf("expected ErrNegativeTimeout for ReadTimeout=0, got %v", err)
	}

	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = -1 * time.Second
	err = cfg.Validate()
	if err != ErrNegativeTimeout {
		t.Errorf("expected ErrNegativeTimeout for negative WriteTimeout, got %v", err)
	}
}

func TestValidate_InvalidMaxConnections(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
	cfg.MaxConnections = 0

	err := cfg.Validate()
	if err != ErrInvalidMaxConnections {
		t.Errorf("expected ErrInvalidMaxConnections, got %v", err)
	}
}

func TestValidate_InvalidRateLimit(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")

	cfg.RateLimitRPS = 0
	err := cfg.Validate()
	if err != ErrInvalidRateLimit {
		t.Errorf("expected ErrInvalidRateLimit for RPS=0, got %v", err)
	}

	cfg.RateLimitRPS = 10
	cfg.RateLimitBurst = -1
	err = cfg.Validate()
	if err != ErrInvalidRateLimit {
		t.Errorf("expected ErrInvalidRateLimit for negative burst, got %v", err)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
	cfg.LogLevel = "invalid"

	err := cfg.Validate()
	if err != ErrInvalidLogLevel {
		t.Errorf("expected ErrInvalidLogLevel, got %v", err)
	}
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
	cfg.LogFormat = "xml"

	err := cfg.Validate()
	if err != ErrInvalidLogFormat {
		t.Errorf("expected ErrInvalidLogFormat, got %v", err)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := Defaults()
	cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_AllLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := Defaults()
			cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
			cfg.LogLevel = level

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for log level %q, got %v", level, err)
			}
		})
	}
}

func TestValidate_AllLogFormats(t *testing.T) {
	formats := []string{"json", "text"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			cfg := Defaults()
			cfg.PowSecret = []byte("test-secret-key-32-bytes-long!!!")
			cfg.LogFormat = format

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for log format %q, got %v", format, err)
			}
		})
	}
}

func TestMustLoad_Panics_OnMissingSecret(t *testing.T) {
	// Clear POW_SECRET
	original := os.Getenv("POW_SECRET")
	os.Unsetenv("POW_SECRET")
	defer func() {
		if original != "" {
			os.Setenv("POW_SECRET", original)
		}
	}()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Errorf("expected string panic, got %T", r)
		}
		if msg == "" || msg[:24] != "config validation error:" {
			t.Errorf("expected validation error panic, got %s", msg)
		}
	}()

	MustLoad()
}

func TestLoad_AllDurationFields(t *testing.T) {
	envVars := map[string]string{
		"WRITE_TIMEOUT":        "15s",
		"IDLE_TIMEOUT":         "3s",
		"CHALLENGE_TIMEOUT":    "120s",
		"GRACEFUL_TIMEOUT":     "45s",
		"CLOCK_SKEW_TOLERANCE": "60s",
	}

	// Save and restore
	originals := make(map[string]string)
	for k := range envVars {
		originals[k] = os.Getenv(k)
	}
	defer func() {
		for k, v := range originals {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	for k, v := range envVars {
		os.Setenv(k, v)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WriteTimeout != 15*time.Second {
		t.Errorf("expected WriteTimeout=15s, got %v", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 3*time.Second {
		t.Errorf("expected IdleTimeout=3s, got %v", cfg.IdleTimeout)
	}
	if cfg.ChallengeTimeout != 120*time.Second {
		t.Errorf("expected ChallengeTimeout=120s, got %v", cfg.ChallengeTimeout)
	}
	if cfg.GracefulTimeout != 45*time.Second {
		t.Errorf("expected GracefulTimeout=45s, got %v", cfg.GracefulTimeout)
	}
	if cfg.ClockSkewTolerance != 60*time.Second {
		t.Errorf("expected ClockSkewTolerance=60s, got %v", cfg.ClockSkewTolerance)
	}
}

func TestLoad_AllIntFields(t *testing.T) {
	envVars := map[string]string{
		"POW_MIN_DIFFICULTY":     "18",
		"POW_MAX_DIFFICULTY":     "22",
		"RATE_LIMIT_BURST":       "50",
		"MAX_CONNECTIONS_PER_IP": "200",
		"MAX_ATTEMPTS_PER_IP":    "100",
	}

	// Save and restore
	originals := make(map[string]string)
	for k := range envVars {
		originals[k] = os.Getenv(k)
	}
	defer func() {
		for k, v := range originals {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	for k, v := range envVars {
		os.Setenv(k, v)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PowMinDifficulty != 18 {
		t.Errorf("expected PowMinDifficulty=18, got %d", cfg.PowMinDifficulty)
	}
	if cfg.PowMaxDifficulty != 22 {
		t.Errorf("expected PowMaxDifficulty=22, got %d", cfg.PowMaxDifficulty)
	}
	if cfg.RateLimitBurst != 50 {
		t.Errorf("expected RateLimitBurst=50, got %d", cfg.RateLimitBurst)
	}
	if cfg.MaxConnectionsPerIP != 200 {
		t.Errorf("expected MaxConnectionsPerIP=200, got %d", cfg.MaxConnectionsPerIP)
	}
	if cfg.MaxAttemptsPerIP != 100 {
		t.Errorf("expected MaxAttemptsPerIP=100, got %d", cfg.MaxAttemptsPerIP)
	}
}
