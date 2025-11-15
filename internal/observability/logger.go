package observability

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for structured logging
type Logger struct {
	*zap.Logger
}

// NewLogger creates a new logger with the specified level
func NewLogger(level string) (*Logger, error) {
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

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{Logger: logger}, nil
}

// WithPeerID adds peer_id to the logger context
func (l *Logger) WithPeerID(peerID string) *Logger {
	return &Logger{Logger: l.Logger.With(zap.String("peer_id", peerID))}
}

// WithOperationID adds operation_id to the logger context
func (l *Logger) WithOperationID(operationID string) *Logger {
	return &Logger{Logger: l.Logger.With(zap.String("operation_id", operationID))}
}

// WithTraceID adds trace_id to the logger context
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{Logger: l.Logger.With(zap.String("trace_id", traceID))}
}


