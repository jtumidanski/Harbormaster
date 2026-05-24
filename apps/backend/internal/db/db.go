// Package db opens the SQLite database with PRAGMAs and connection limits
// appropriate for a single-writer workload, and runs forward-only migrations
// at startup.
package db

import (
	"database/sql"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
)

// Open returns a configured *gorm.DB wrapping an open *sql.DB.
// Callers must close the returned *sql.DB on shutdown.
func Open(cfg config.Config) (*gorm.DB, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)",
		cfg.DatabasePath,
	)
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	sdb, err := gdb.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("unwrap sql.DB: %w", err)
	}
	sdb.SetMaxOpenConns(1)
	sdb.SetMaxIdleConns(1)
	sdb.SetConnMaxLifetime(0)

	if err := registerBusyRetry(gdb); err != nil {
		return nil, nil, fmt.Errorf("register busy-retry plugin: %w", err)
	}
	return gdb, sdb, nil
}
