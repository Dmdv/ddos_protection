package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the complete server configuration.
type Config struct {
	// Server
	ServerAddress    string        `env:"SERVER_ADDRESS"`
	MetricsAddress   string        `env:"METRICS_ADDRESS"`
	MaxConnections   int           `env:"MAX_CONNECTIONS"`
	ReadTimeout      time.Duration `env:"READ_TIMEOUT"`
	WriteTimeout     time.Duration `env:"WRITE_TIMEOUT"`
	IdleTimeout      time.Duration `env:"IDLE_TIMEOUT"`
	ChallengeTimeout time.Duration `env:"CHALLENGE_TIMEOUT"`
	GracefulTimeout  time.Duration `env:"GRACEFUL_TIMEOUT"`

	// PoW
	PowSecret          []byte        `env:"POW_SECRET"`
	PowDifficulty      int           `env:"POW_DIFFICULTY"`
	PowMinDifficulty   int           `env:"POW_MIN_DIFFICULTY"`
	PowMaxDifficulty   int           `env:"POW_MAX_DIFFICULTY"`
	ClockSkewTolerance time.Duration `env:"CLOCK_SKEW_TOLERANCE"`

	// Rate Limiting
	RateLimitRPS        float64 `env:"RATE_LIMIT_RPS"`
	RateLimitBurst      int     `env:"RATE_LIMIT_BURST"`
	MaxConnectionsPerIP int     `env:"MAX_CONNECTIONS_PER_IP"`
	MaxAttemptsPerIP    int     `env:"MAX_ATTEMPTS_PER_IP"`

	// Logging
	LogLevel  string `env:"LOG_LEVEL"`
	LogFormat string `env:"LOG_FORMAT"`

	// Quotes
	QuotesFile string `env:"QUOTES_FILE"`
}

// Defaults returns a configuration with sensible defaults.
func Defaults() *Config {
	return &Config{
		ServerAddress:    ":8080",
		MetricsAddress:   ":9090",
		MaxConnections:   10000,
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      5 * time.Second,
		ChallengeTimeout: 60 * time.Second,
		GracefulTimeout:  30 * time.Second,

		PowDifficulty:      20,
		PowMinDifficulty:   16,
		PowMaxDifficulty:   24,
		ClockSkewTolerance: 30 * time.Second,

		RateLimitRPS:        10,
		RateLimitBurst:      20,
		MaxConnectionsPerIP: 100,
		MaxAttemptsPerIP:    50,

		LogLevel:  "info",
		LogFormat: "json",

		QuotesFile: "quotes.txt",
	}
}

// Load loads configuration from environment variables.
// It starts with defaults and overrides with environment values.
func Load() (*Config, error) {
	cfg := Defaults()

	// Server
	if v := os.Getenv("SERVER_ADDRESS"); v != "" {
		cfg.ServerAddress = v
	}
	if v := os.Getenv("METRICS_ADDRESS"); v != "" {
		cfg.MetricsAddress = v
	}
	if v := os.Getenv("MAX_CONNECTIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_CONNECTIONS: %w", err)
		}
		cfg.MaxConnections = n
	}
	if v := os.Getenv("READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid READ_TIMEOUT: %w", err)
		}
		cfg.ReadTimeout = d
	}
	if v := os.Getenv("WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WRITE_TIMEOUT: %w", err)
		}
		cfg.WriteTimeout = d
	}
	if v := os.Getenv("IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid IDLE_TIMEOUT: %w", err)
		}
		cfg.IdleTimeout = d
	}
	if v := os.Getenv("CHALLENGE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CHALLENGE_TIMEOUT: %w", err)
		}
		cfg.ChallengeTimeout = d
	}
	if v := os.Getenv("GRACEFUL_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid GRACEFUL_TIMEOUT: %w", err)
		}
		cfg.GracefulTimeout = d
	}

	// PoW - POW_SECRET is required
	if v := os.Getenv("POW_SECRET"); v != "" {
		cfg.PowSecret = []byte(v)
	}
	if v := os.Getenv("POW_DIFFICULTY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POW_DIFFICULTY: %w", err)
		}
		cfg.PowDifficulty = n
	}
	if v := os.Getenv("POW_MIN_DIFFICULTY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POW_MIN_DIFFICULTY: %w", err)
		}
		cfg.PowMinDifficulty = n
	}
	if v := os.Getenv("POW_MAX_DIFFICULTY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POW_MAX_DIFFICULTY: %w", err)
		}
		cfg.PowMaxDifficulty = n
	}
	if v := os.Getenv("CLOCK_SKEW_TOLERANCE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CLOCK_SKEW_TOLERANCE: %w", err)
		}
		cfg.ClockSkewTolerance = d
	}

	// Rate Limiting
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid RATE_LIMIT_RPS: %w", err)
		}
		cfg.RateLimitRPS = f
	}
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid RATE_LIMIT_BURST: %w", err)
		}
		cfg.RateLimitBurst = n
	}
	if v := os.Getenv("MAX_CONNECTIONS_PER_IP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_CONNECTIONS_PER_IP: %w", err)
		}
		cfg.MaxConnectionsPerIP = n
	}
	if v := os.Getenv("MAX_ATTEMPTS_PER_IP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_ATTEMPTS_PER_IP: %w", err)
		}
		cfg.MaxAttemptsPerIP = n
	}

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}

	// Quotes
	if v := os.Getenv("QUOTES_FILE"); v != "" {
		cfg.QuotesFile = v
	}

	return cfg, nil
}

// Validation errors.
var (
	ErrMissingPowSecret       = errors.New("POW_SECRET is required")
	ErrPowSecretTooShort      = errors.New("POW_SECRET must be at least 32 bytes")
	ErrInvalidDifficultyRange = errors.New("POW_MIN_DIFFICULTY must be <= POW_MAX_DIFFICULTY")
	ErrDifficultyOutOfRange   = errors.New("POW_DIFFICULTY must be between POW_MIN_DIFFICULTY and POW_MAX_DIFFICULTY")
	ErrNegativeTimeout        = errors.New("timeout values must be positive")
	ErrInvalidMaxConnections  = errors.New("MAX_CONNECTIONS must be positive")
	ErrInvalidRateLimit       = errors.New("rate limit values must be positive")
	ErrInvalidLogLevel        = errors.New("LOG_LEVEL must be one of: debug, info, warn, error")
	ErrInvalidLogFormat       = errors.New("LOG_FORMAT must be one of: json, text")
)

// Validate checks the configuration for errors.
// It returns an error if the configuration is invalid.
func (c *Config) Validate() error {
	// POW_SECRET validation
	if len(c.PowSecret) == 0 {
		return ErrMissingPowSecret
	}
	if len(c.PowSecret) < 32 {
		return ErrPowSecretTooShort
	}

	// Difficulty range validation
	if c.PowMinDifficulty > c.PowMaxDifficulty {
		return ErrInvalidDifficultyRange
	}
	if c.PowDifficulty < c.PowMinDifficulty || c.PowDifficulty > c.PowMaxDifficulty {
		return ErrDifficultyOutOfRange
	}

	// Timeout validation
	if c.ReadTimeout <= 0 || c.WriteTimeout <= 0 || c.IdleTimeout <= 0 ||
		c.ChallengeTimeout <= 0 || c.GracefulTimeout <= 0 {
		return ErrNegativeTimeout
	}

	// Connection limits
	if c.MaxConnections <= 0 {
		return ErrInvalidMaxConnections
	}

	// Rate limit validation
	if c.RateLimitRPS <= 0 || c.RateLimitBurst <= 0 {
		return ErrInvalidRateLimit
	}

	// Log level validation
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// Valid
	default:
		return ErrInvalidLogLevel
	}

	// Log format validation
	switch c.LogFormat {
	case "json", "text":
		// Valid
	default:
		return ErrInvalidLogFormat
	}

	return nil
}

// MustLoad loads and validates configuration, panicking on error.
// This is useful for main() where failure should terminate the program.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("config load error: %v", err))
	}
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("config validation error: %v", err))
	}
	return cfg
}
