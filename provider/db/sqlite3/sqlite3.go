// Package sqlite3 provides SQLite database connection initialization.
package sqlite3

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB initializes a SQLite database connection using GORM.
//
// Parameters:
//   - source: The SQLite database file path, e.g., "gorm.db" or ":memory:" for in-memory database
//   - logger: The GORM logger interface for logging database operations
//
// Returns:
//   - *gorm.DB: A GORM database instance connected to SQLite
//   - error: Error if connection fails
func InitDB(source string, logger logger.Interface) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(source),
		&gorm.Config{
			SkipDefaultTransaction: true,
			Logger:                 logger,
		})
}
