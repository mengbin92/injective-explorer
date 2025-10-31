// Package mysql provides MySQL database connection initialization.
package mysql

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm/logger"

	"gorm.io/gorm"
)

// InitDB initializes a MySQL database connection using GORM.
//
// Parameters:
//   - source: The MySQL data source name (DSN), e.g., "user:password@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"
//   - logger: The GORM logger interface for logging database operations
//
// Returns:
//   - *gorm.DB: A GORM database instance connected to MySQL
//   - error: Error if connection fails
func InitDB(source string, logger logger.Interface) (*gorm.DB, error) {
	return gorm.Open(mysql.Open(source), &gorm.Config{
		SkipDefaultTransaction:                   true,
		AllowGlobalUpdate:                        false,
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger,
	})
}
