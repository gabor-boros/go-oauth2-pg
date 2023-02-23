package pgStore

import (
	"context"
	"fmt"
)

const (
	LogLevelDebug = LogLevel("DEBUG") // debug level
	LogLevelInfo  = LogLevel("INFO")  // info level
	LogLevelWarn  = LogLevel("WARN")  // warn level
	LogLevelError = LogLevel("ERROR") // error level
)

var (
	// ErrNoTable is returned when no table was provided.
	ErrNoTable = fmt.Errorf("no table provided")
	// ErrNoConnPool is returned when no database was provided.
	ErrNoConnPool = fmt.Errorf("no connection pool provided")
	// ErrNoLogger is returned when no logger was provided.
	ErrNoLogger = fmt.Errorf("no logger provided")
)

// LogLevel is a log level.
type LogLevel string

// Logger wraps a logger to log messages.
type Logger interface {
	// Log logs a message.
	Log(ctx context.Context, level LogLevel, msg string, args ...any)
}

// NoopLogger is a logger that does nothing.
type NoopLogger struct{}

// Log logs a message.
func (l *NoopLogger) Log(ctx context.Context, level LogLevel, msg string, args ...any) {}
