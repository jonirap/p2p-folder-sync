package observability_test

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/p2p-folder-sync/p2p-sync/internal/observability"
)

// createTestLogger creates a logger that writes to a buffer for testing
func createTestLogger(level string) (*observability.Logger, *bytes.Buffer, error) {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	buffer := &bytes.Buffer{}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(buffer),
		zapLevel,
	)

	logger := &observability.Logger{
		Logger: zap.New(core),
	}

	return logger, buffer, nil
}

func TestLogger(t *testing.T) {
	// Test logger creation with different levels
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			logger, err := observability.NewLogger(level)
			if err != nil {
				t.Fatalf("Failed to create logger with level %s: %v", level, err)
			}
			if logger == nil {
				t.Errorf("Expected logger to be created for level %s", level)
			}
		})
	}

	// Test invalid level defaults to info
	logger, err := observability.NewLogger("invalid")
	if err != nil {
		t.Fatalf("Failed to create logger with invalid level: %v", err)
	}
	if logger == nil {
		t.Fatal("Expected logger to be created with invalid level")
	}
}

func TestLoggerOutput(t *testing.T) {
	// Test info level logging
	t.Run("info_level", func(t *testing.T) {
		logger, buffer, err := createTestLogger("info")
		if err != nil {
			t.Fatalf("Failed to create test logger: %v", err)
		}

		testMessage := "Test info message"
		logger.Info(testMessage)

		output := buffer.String()
		if !strings.Contains(output, testMessage) {
			t.Errorf("Expected log output to contain message '%s', got: %s", testMessage, output)
		}
		if !strings.Contains(output, `"level":"info"`) {
			t.Errorf("Expected log output to contain level 'info', got: %s", output)
		}
	})

	// Test error level logging
	t.Run("error_level", func(t *testing.T) {
		logger, buffer, err := createTestLogger("info")
		if err != nil {
			t.Fatalf("Failed to create test logger: %v", err)
		}

		testMessage := "Test error message"
		logger.Error(testMessage)

		output := buffer.String()
		if !strings.Contains(output, testMessage) {
			t.Errorf("Expected log output to contain message '%s', got: %s", testMessage, output)
		}
		if !strings.Contains(output, `"level":"error"`) {
			t.Errorf("Expected log output to contain level 'error', got: %s", output)
		}
	})
}

func TestLoggerLevelFiltering(t *testing.T) {
	// Test info level filtering (should show info, warn, error)
	t.Run("info_level_filtering", func(t *testing.T) {
		logger, buffer, err := createTestLogger("info")
		if err != nil {
			t.Fatalf("Failed to create test logger: %v", err)
		}

		logger.Debug("Debug message - should be filtered")
		logger.Info("Info message - should appear")
		logger.Warn("Warn message - should appear")
		logger.Error("Error message - should appear")

		output := buffer.String()

		// Should contain info, warn, error
		if !strings.Contains(output, "Info message") {
			t.Error("Expected info level message to appear")
		}
		if !strings.Contains(output, "Warn message") {
			t.Error("Expected warn level message to appear")
		}
		if !strings.Contains(output, "Error message") {
			t.Error("Expected error level message to appear")
		}

		// Should NOT contain debug
		if strings.Contains(output, "Debug message") {
			t.Error("Expected debug level message to be filtered out")
		}
	})

	// Test error level filtering (should only show error)
	t.Run("error_level_filtering", func(t *testing.T) {
		logger, buffer, err := createTestLogger("error")
		if err != nil {
			t.Fatalf("Failed to create test logger: %v", err)
		}

		logger.Debug("Debug message - should be filtered")
		logger.Info("Info message - should be filtered")
		logger.Warn("Warn message - should be filtered")
		logger.Error("Error message - should appear")

		output := buffer.String()

		// Should contain only error
		if !strings.Contains(output, "Error message") {
			t.Error("Expected error level message to appear")
		}

		// Should NOT contain debug, info, warn
		if strings.Contains(output, "Debug message") {
			t.Error("Expected debug level message to be filtered out")
		}
		if strings.Contains(output, "Info message") {
			t.Error("Expected info level message to be filtered out")
		}
		if strings.Contains(output, "Warn message") {
			t.Error("Expected warn level message to be filtered out")
		}
	})

	// Test debug level filtering (should show all levels)
	t.Run("debug_level_filtering", func(t *testing.T) {
		logger, buffer, err := createTestLogger("debug")
		if err != nil {
			t.Fatalf("Failed to create test logger: %v", err)
		}

		logger.Debug("Debug message - should appear")
		logger.Info("Info message - should appear")
		logger.Warn("Warn message - should appear")
		logger.Error("Error message - should appear")

		output := buffer.String()

		// Should contain all levels
		if !strings.Contains(output, "Debug message") {
			t.Error("Expected debug level message to appear")
		}
		if !strings.Contains(output, "Info message") {
			t.Error("Expected info level message to appear")
		}
		if !strings.Contains(output, "Warn message") {
			t.Error("Expected warn level message to appear")
		}
		if !strings.Contains(output, "Error message") {
			t.Error("Expected error level message to appear")
		}
	})
}

func TestLoggerContext(t *testing.T) {
	logger, buffer, err := createTestLogger("info")
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}

	// Test WithPeerID
	loggerWithPeer := logger.WithPeerID("test-peer-123")
	loggerWithPeer.Info("Message with peer ID")

	output := buffer.String()
	if !strings.Contains(output, `"peer_id":"test-peer-123"`) {
		t.Errorf("Expected log output to contain peer_id field, got: %s", output)
	}

	// Test WithOperationID
	buffer.Reset()
	loggerWithOp := logger.WithOperationID("op-456")
	loggerWithOp.Info("Message with operation ID")

	output = buffer.String()
	if !strings.Contains(output, `"operation_id":"op-456"`) {
		t.Errorf("Expected log output to contain operation_id field, got: %s", output)
	}

	// Test WithTraceID
	buffer.Reset()
	loggerWithTrace := logger.WithTraceID("trace-789")
	loggerWithTrace.Info("Message with trace ID")

	output = buffer.String()
	if !strings.Contains(output, `"trace_id":"trace-789"`) {
		t.Errorf("Expected log output to contain trace_id field, got: %s", output)
	}
}
