package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != "info" {
		t.Errorf("expected Level=info, got %s", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected Format=json, got %s", cfg.Format)
	}
	if cfg.Output == nil {
		t.Error("expected Output to not be nil")
	}
	if cfg.AddSource {
		t.Error("expected AddSource=false")
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["msg"] != "test message" {
		t.Errorf("expected msg=test message, got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("expected key=value, got %v", logEntry["key"])
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected output to contain 'key=value', got %s", output)
	}
}

func TestLogger_Levels(t *testing.T) {
	tests := []struct {
		level    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(Config{
				Level:  tt.level,
				Format: "json",
				Output: &buf,
			})

			if logger.GetLevel() != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, logger.GetLevel())
			}
		})
	}
}

func TestLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Initial level is info
	if logger.GetLevel() != slog.LevelInfo {
		t.Errorf("expected initial level info, got %v", logger.GetLevel())
	}

	// Debug message should not be logged
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("debug message should not be logged at info level")
	}

	// Change level to debug
	logger.SetLevel("debug")
	if logger.GetLevel() != slog.LevelDebug {
		t.Errorf("expected level debug, got %v", logger.GetLevel())
	}

	// Now debug message should be logged
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("debug message should be logged at debug level")
	}
}

func TestLogger_SetLevelValue(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	logger.SetLevelValue(slog.LevelError)

	if logger.GetLevel() != slog.LevelError {
		t.Errorf("expected level error, got %v", logger.GetLevel())
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	childLogger := logger.With("component", "test")
	childLogger.Info("message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if logEntry["component"] != "test" {
		t.Errorf("expected component=test, got %v", logEntry["component"])
	}
}

func TestLogger_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	childLogger := logger.WithGroup("group1")
	childLogger.Info("message", "key", "value")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	group, ok := logEntry["group1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected group1 to be a map, got %T", logEntry["group1"])
	}
	if group["key"] != "value" {
		t.Errorf("expected group1.key=value, got %v", group["key"])
	}
}

func TestLogger_Context(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	ctx := WithContext(context.Background(), logger)
	retrieved := FromContext(ctx)

	if retrieved != logger {
		t.Error("expected same logger from context")
	}

	// Test with empty context
	emptyCtx := context.Background()
	defaultFromCtx := FromContext(emptyCtx)
	if defaultFromCtx == nil {
		t.Error("expected default logger from empty context")
	}
}

func TestGlobalFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})
	SetDefault(logger)

	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 log lines, got %d", len(lines))
	}
}

func TestContextFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})

	ctx := WithContext(context.Background(), logger)

	DebugContext(ctx, "debug")
	InfoContext(ctx, "info")
	WarnContext(ctx, "warn")
	ErrorContext(ctx, "error")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 log lines, got %d", len(lines))
	}
}

func TestErr(t *testing.T) {
	attr := Err(context.Canceled)
	if attr.Key != KeyError {
		t.Errorf("expected key=%s, got %s", KeyError, attr.Key)
	}
}

func TestRemoteAddr(t *testing.T) {
	attr := RemoteAddr("127.0.0.1:8080")
	if attr.Key != KeyRemoteAddr {
		t.Errorf("expected key=%s, got %s", KeyRemoteAddr, attr.Key)
	}
	if attr.Value.String() != "127.0.0.1:8080" {
		t.Errorf("expected value=127.0.0.1:8080, got %s", attr.Value.String())
	}
}

func TestConnID(t *testing.T) {
	attr := ConnID(12345)
	if attr.Key != KeyConnID {
		t.Errorf("expected key=%s, got %s", KeyConnID, attr.Key)
	}
	if attr.Value.Uint64() != 12345 {
		t.Errorf("expected value=12345, got %d", attr.Value.Uint64())
	}
}

func TestDefault(t *testing.T) {
	logger := Default()
	if logger == nil {
		t.Error("default logger should not be nil")
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		configLevel string
		logLevel    string
		shouldLog   bool
	}{
		{"error", "debug", false},
		{"error", "info", false},
		{"error", "warn", false},
		{"error", "error", true},
		{"warn", "debug", false},
		{"warn", "info", false},
		{"warn", "warn", true},
		{"warn", "error", true},
		{"info", "debug", false},
		{"info", "info", true},
		{"info", "warn", true},
		{"info", "error", true},
		{"debug", "debug", true},
		{"debug", "info", true},
		{"debug", "warn", true},
		{"debug", "error", true},
	}

	for _, tt := range tests {
		name := tt.configLevel + "_" + tt.logLevel
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(Config{
				Level:  tt.configLevel,
				Format: "json",
				Output: &buf,
			})

			switch tt.logLevel {
			case "debug":
				logger.Debug("test")
			case "info":
				logger.Info("test")
			case "warn":
				logger.Warn("test")
			case "error":
				logger.Error("test")
			}

			hasOutput := buf.Len() > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("config=%s, log=%s: expected logged=%v, got=%v",
					tt.configLevel, tt.logLevel, tt.shouldLog, hasOutput)
			}
		})
	}
}

func TestNew_NilOutput(t *testing.T) {
	// Should default to stdout
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: nil,
	})

	if logger == nil {
		t.Error("logger should not be nil")
	}
}

func BenchmarkLogger_Info(b *testing.B) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", "key", "value", "number", i)
		buf.Reset()
	}
}

func BenchmarkLogger_InfoFiltered(b *testing.B) {
	var buf bytes.Buffer
	logger := New(Config{
		Level:  "error", // Info will be filtered
		Format: "json",
		Output: &buf,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", "key", "value", "number", i)
	}
}
