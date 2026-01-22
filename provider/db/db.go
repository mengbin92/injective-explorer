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

const (
	// Database connection retry constants
	DBMaxRetries      = 10             // Maximum number of connection retry attempts
	DBInitialRetryDelay = 2 * time.Second // Initial delay before retry

	// Database connection pool settings
	// Optimize connection pool for high TPS:
	// - MaxIdleConns: Keep more idle connections to reduce connection overhead
	// - MaxOpenConns: Allow more concurrent connections for high load
	//   IMPORTANT: Must be 60-70% of MySQL's max_connections to avoid "Too many connections"
	//   Default MySQL max_connections is 151, so 100 is safe (66% of 151)
	// - ConnMaxLifetime: Recycle connections periodically to prevent stale connections
	// - ConnMaxIdleTime: Close idle connections after a period to free resources
	DBMaxIdleConns      = 30            // 30% of MaxOpenConns for efficiency (100 * 0.3)
	DBMaxOpenConns      = 100           // Increased from 50 to 100 for better concurrency
	DBConnMaxLifetime   = 5 * time.Minute  // Reduced from 1 hour to 5 minutes for faster recycling
	DBConnMaxIdleTime   = 3 * time.Minute  // Close idle connections after 3 minutes to free resources
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
		// Retry connection with exponential backoff
		retryDelay := DBInitialRetryDelay

		for attempt := 0; attempt < DBMaxRetries; attempt++ {
			switch cfg.Driver {
			case "postgre":
				gdb, initErr = postgres.InitDB(cfg.Source, gormLogger)
			case "sqlite":
				gdb, initErr = sqlite3.InitDB(cfg.Source, gormLogger)
			default:
				// MySQL is the default driver
				gdb, initErr = mysql.InitDB(cfg.Source, gormLogger)
			}

			if initErr == nil {
				// Connection successful
				break
			}

			// Log retry attempt
			logHelper := log.NewHelper(logKratos)
			if attempt < DBMaxRetries-1 {
				logHelper.Warnf("database connection attempt %d/%d failed: %v, retrying in %v...",
					attempt+1, DBMaxRetries, initErr, retryDelay)
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
			}
		}
	})

	if initErr != nil {
		return errors.Wrap(initErr, "connect to db error")
	}

	if gdb == nil {
		return errors.New("database instance is nil after initialization")
	}

	// Configure connection pool settings for better performance
	sqlDB, err := gdb.DB()
	if err != nil {
		return errors.Wrap(err, "get sql db error")
	}

	// Optimize connection pool for high TPS
	// CRITICAL: If you see "Too many connections" error, reduce MaxOpenConns to 60% of MySQL max_connections
	// Example: If MySQL max_connections = 151, set MaxOpenConns = 90 (151 * 0.6)
	//
	// Performance optimization: Increased pool size for better concurrency
	// Ensure MySQL max_connections >= 150 (recommended: 200)
	sqlDB.SetMaxIdleConns(DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(DBConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(DBConnMaxIdleTime)

	// Test the connection
	if err := sqlDB.PingContext(ctx); err != nil {
		return errors.Wrap(err, "ping database error")
	}

	// Register custom Sonic JSON serializer for high-performance JSON operations
	// This replaces the default JSON serializer to reduce CPU usage
	RegisterSonicJSONSerializer()

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
