// Package logger provides a zap-based logger implementation for the kratos framework.
package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/go-kratos/kratos/v2/log"

	"injective-explorer/internal/conf"
)

var _ log.Logger = (*ZapLogger)(nil)

// ZapLogger implements the kratos Logger interface using zap.
// It provides structured logging capabilities with configurable output formats and levels.
type ZapLogger struct {
	log  *zap.Logger
	Sync func() error
}

// NewZapLogger creates a new ZapLogger instance from the provided configuration.
//
// Parameters:
//   - logConf: The logging configuration containing format and level settings
//
// Returns:
//   - *ZapLogger: A new logger instance ready for use
func NewZapLogger(logConf *conf.Log) *ZapLogger {
	if logConf == nil {
		// Use default configuration if none provided
		logConf = &conf.Log{
			Format: "json",
			Level:  0,
		}
	}
	zapLogger := DefaultLogger(logConf)
	return &ZapLogger{log: zapLogger, Sync: zapLogger.Sync}
}

// Log implements the kratos Logger interface.
// It accepts a log level and key-value pairs and writes them using zap.
//
// Parameters:
//   - level: The log level (Debug, Info, Warn, Error)
//   - keyvals: A variadic list of key-value pairs (must be even number of arguments)
//
// Returns:
//   - error: Always returns nil; errors in logging are handled internally
func (l *ZapLogger) Log(level log.Level, keyvals ...interface{}) error {
	if len(keyvals) == 0 || len(keyvals)%2 != 0 {
		l.log.Warn("Keyvalues must appear in pairs", zap.Any("keyvals", keyvals))
		return nil
	}

	// Convert key-value pairs to zap fields
	var fields []zap.Field
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			fields = append(fields, zap.Any(fmt.Sprint(keyvals[i]), keyvals[i+1]))
		}
	}

	// Log at the appropriate level
	switch level {
	case log.LevelDebug:
		l.log.Debug("", fields...)
	case log.LevelInfo:
		l.log.Info("", fields...)
	case log.LevelWarn:
		l.log.Warn("", fields...)
	case log.LevelError:
		l.log.Error("", fields...)
	}
	return nil
}

// With creates a new logger instance with additional fields.
//
// Parameters:
//   - fields: Additional zap fields to include in all log messages
//
// Returns:
//   - *ZapLogger: A new logger instance with the additional fields
func (l *ZapLogger) With(fields ...zap.Field) *ZapLogger {
	return &ZapLogger{
		log:  l.log.With(fields...),
		Sync: l.Sync,
	}
}

// DefaultLogger creates a default zap logger configuration.
// It supports both JSON and console output formats and configurable log levels.
//
// Parameters:
//   - logConf: The logging configuration
//
// Returns:
//   - *zap.Logger: A configured zap logger instance
func DefaultLogger(logConf *conf.Log) *zap.Logger {
	if logConf == nil {
		logConf = &conf.Log{
			Format: "json",
			Level:  0,
		}
	}

	var cores []zapcore.Core

	// Configure encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var encoder zapcore.Encoder
	if logConf.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Define log level enablers
	highPriority := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level >= zapcore.ErrorLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level < zapcore.ErrorLevel && level >= zapcore.DebugLevel
	})

	// Create cores for different priority levels
	infoCore := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		lowPriority,
	)
	errorCore := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		highPriority,
	)

	cores = append(cores, infoCore, errorCore)

	// Create logger with cores and set caller info
	logger := zap.New(
		zapcore.NewTee(cores...),
		zap.AddCaller(),
		zap.AddCallerSkip(2),
	)

	return setLogLevel(logger, logConf.GetLevel())
}

// setLogLevel configures the minimum log level for the logger.
//
// Parameters:
//   - logger: The zap logger to configure
//   - level: The minimum log level (-1=Debug, 0=Info, 1=Warn, 3=DPanic, 4=Panic, 5=Fatal, default=Error)
//
// Returns:
//   - *zap.Logger: The logger with the configured level
func setLogLevel(logger *zap.Logger, level int32) *zap.Logger {
	var zapLevel zapcore.Level

	switch level {
	case -1:
		zapLevel = zapcore.DebugLevel
	case 0:
		zapLevel = zapcore.InfoLevel
	case 1:
		zapLevel = zapcore.WarnLevel
	case 3:
		zapLevel = zapcore.DPanicLevel
	case 4:
		zapLevel = zapcore.PanicLevel
	case 5:
		zapLevel = zapcore.FatalLevel
	default:
		zapLevel = zapcore.ErrorLevel
	}

	return logger.WithOptions(zap.IncreaseLevel(zapLevel))
}
