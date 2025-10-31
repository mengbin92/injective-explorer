// Package db provides database connection initialization and management.
// It supports multiple database drivers including MySQL, PostgreSQL, and SQLite.
package db

import (
	"injective-explorer/internal/conf"
	"injective-explorer/provider/db/mysql"
	"injective-explorer/provider/db/postgres"
	"injective-explorer/provider/db/sqlite3"
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	// gdb is the global GORM database instance
	gdb *gorm.DB
	// initDBOnce ensures the database is initialized only once (thread-safe)
	initDBOnce sync.Once
)

// Init initializes the database connection.
// It uses sync.Once to ensure the database is initialized only once, even if called multiple times.
//
// Supported drivers:
//   - "postgre": PostgreSQL database
//   - "sqlite": SQLite database
//   - Default: MySQL database
//
// Parameters:
//   - ctx: Context for the initialization operation (currently unused but reserved for future use)
//   - cfg: Database configuration containing driver type and connection source
//   - logKratos: Logger instance for database logging
//
// Returns:
//   - error: Error if initialization or connection fails
func Init(ctx context.Context, cfg *conf.Data_Database, logKratos log.Logger) error {
	if cfg == nil {
		return errors.New("database config cannot be nil")
	}

	var initErr error
	gormLogger := NewGormLogger(logKratos, logger.Error)

	initDBOnce.Do(func() {
		switch cfg.Driver {
		case "postgre":
			gdb, initErr = postgres.InitDB(cfg.Source, gormLogger)
		case "sqlite":
			gdb, initErr = sqlite3.InitDB(cfg.Source, gormLogger)
		default:
			// MySQL is the default driver
			gdb, initErr = mysql.InitDB(cfg.Source, gormLogger)
		}
	})

	if initErr != nil {
		return errors.Wrap(initErr, "connect to db error")
	}

	if gdb == nil {
		return errors.New("database instance is nil after initialization")
	}

	// Configure connection pool settings
	sqlDB, err := gdb.DB()
	if err != nil {
		return errors.Wrap(err, "get sql db error")
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return nil
}

// Get returns the global GORM database instance.
//
// Returns:
//   - *gorm.DB: The GORM database instance
//
// Panics:
//   - If the database has not been initialized (nil instance)
//
// Note: The database should be initialized using Init before calling this function.
func Get() *gorm.DB {
	if gdb == nil {
		panic("database is nil; please initialize it using db.Init()")
	}

	return gdb
}
