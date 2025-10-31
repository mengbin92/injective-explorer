// Package postgres provides PostgreSQL database connection initialization.
package postgres

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB initializes a PostgreSQL database connection using GORM.
//
// Parameters:
//   - source: The PostgreSQL data source name (DSN), e.g., "host=localhost user=gorm dbname=gorm password=mypassword port=9920 sslmode=disable TimeZone=Asia/Shanghai"
//   - logger: The GORM logger interface for logging database operations
//
// Returns:
//   - *gorm.DB: A GORM database instance connected to PostgreSQL
//   - error: Error if connection fails
func InitDB(source string, logger logger.Interface) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(source),
		&gorm.Config{
			SkipDefaultTransaction: true,
			Logger:                 logger,
		})
}
