package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents a logging level.
type Level = slog.Level

// Log levels.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config holds logger configuration.
type Config struct {
	// Level is the minimum log level (debug, info, warn, error)
	Level string

	// Format is the output format (json, text)
	Format string

	// Output is the writer for log output (defaults to os.Stdout)
	Output io.Writer

	// AddSource adds source file and line to log entries
	AddSource bool
}

// DefaultConfig returns default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:     "info",
		Format:    "json",
		Output:    os.Stdout,
		AddSource: false,
	}
}

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
	level  *slog.LevelVar
	config Config
}

// New creates a new Logger with the given configuration.
func New(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := &slog.LevelVar{}
	level.Set(parseLevel(cfg.Level))

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "text":
		handler = slog.NewTextHandler(cfg.Output, opts)
	default:
		handler = slog.NewJSONHandler(cfg.Output, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
		level:  level,
		config: cfg,
	}
}

// parseLevel parses a string level to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetLevel dynamically sets the log level.
func (l *Logger) SetLevel(level string) {
	l.level.Set(parseLevel(level))
}

// SetLevelValue dynamically sets the log level using slog.Level.
func (l *Logger) SetLevelValue(level slog.Level) {
	l.level.Set(level)
}

// GetLevel returns the current log level.
func (l *Logger) GetLevel() slog.Level {
	return l.level.Level()
}

// With returns a new Logger with the given attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
		level:  l.level,
		config: l.config,
	}
}

// WithGroup returns a new Logger with the given group name.
func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		Logger: l.Logger.WithGroup(name),
		level:  l.level,
		config: l.config,
	}
}

// Context key for logger.
type contextKey struct{}

// WithContext returns a new context with the logger.
func WithContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the logger from the context.
// Returns the default logger if none is found.
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(contextKey{}).(*Logger); ok {
		return logger
	}
	return defaultLogger
}

// Global default logger.
var defaultLogger = New(DefaultConfig())

// Default returns the default logger.
func Default() *Logger {
	return defaultLogger
}

// SetDefault sets the default logger.
func SetDefault(logger *Logger) {
	defaultLogger = logger
	slog.SetDefault(logger.Logger)
}

// Global convenience functions.

// Debug logs at debug level using the default logger.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Info logs at info level using the default logger.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs at warn level using the default logger.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs at error level using the default logger.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// DebugContext logs at debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).DebugContext(ctx, msg, args...)
}

// InfoContext logs at info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).InfoContext(ctx, msg, args...)
}

// WarnContext logs at warn level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).WarnContext(ctx, msg, args...)
}

// ErrorContext logs at error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).ErrorContext(ctx, msg, args...)
}

// Common log field keys.
const (
	KeyError      = "error"
	KeyRemoteAddr = "remote_addr"
	KeyDuration   = "duration"
	KeyDifficulty = "difficulty"
	KeyChallenge  = "challenge_id"
	KeyConnID     = "conn_id"
)

// Err returns an error attribute for logging.
func Err(err error) slog.Attr {
	return slog.Any(KeyError, err)
}

// RemoteAddr returns a remote address attribute.
func RemoteAddr(addr string) slog.Attr {
	return slog.String(KeyRemoteAddr, addr)
}

// Duration returns a duration attribute.
func Duration(name string, d any) slog.Attr {
	return slog.Any(name, d)
}

// ConnID returns a connection ID attribute.
func ConnID(id uint64) slog.Attr {
	return slog.Uint64(KeyConnID, id)
}
