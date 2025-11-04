package db

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type GormLogger struct {
	log      *log.Helper
	logLevel logger.LogLevel
}

func NewGormLogger(l log.Logger, level logger.LogLevel) *GormLogger {
	return &GormLogger{
		log:      log.NewHelper(l),
		logLevel: level,
	}
}

func (gl *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *gl
	newlogger.logLevel = level
	return &newlogger
}

func (gl *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if gl.logLevel >= logger.Info {
		gl.log.Infof(msg, data...)
	}
}

func (gl *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if gl.logLevel >= logger.Warn {
		gl.log.Warnf(msg, data...)
	}
}

func (gl *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if gl.logLevel >= logger.Error {
		gl.log.Errorf(msg, data...)
	}
}

func (gl *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if gl.logLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil:
		// Record not found is a normal case, don't log it as error
		if err == gorm.ErrRecordNotFound {
			// Skip logging record not found errors as they are expected in normal operations
			// Only log at debug level if log level is Info or higher
			if gl.logLevel >= logger.Info {
				gl.log.Debugf("SQL: %s | rows: %d | err: record not found | took: %v", sql, rows, elapsed)
			}
			// Otherwise, just skip logging
			return
		}
		// Other errors are logged at error level
		if gl.logLevel >= logger.Error {
			gl.log.Errorf("SQL Error: %s | rows: %d | err: %v | took: %v", sql, rows, err, elapsed)
		}
	case gl.logLevel >= logger.Info:
		gl.log.Infof("SQL: %s | rows: %d | took: %v", sql, rows, elapsed)
	}
}
