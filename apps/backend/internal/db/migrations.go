package db

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	migrations "github.com/jtumidanski/Harbormaster/migrations"
)

// Migrate runs all up-migrations against the given gorm.DB.
//
// The golang-migrate database/sqlite package is NOT imported directly because
// it carries a blank `_ "modernc.org/sqlite"` import that conflicts with
// glebarez/go-sqlite, which has already registered the "sqlite" driver name.
// Instead we use an inline driver shim (sqlite_migrate_driver.go) that is
// functionally identical but avoids the double-registration panic.
func Migrate(gdb *gorm.DB) error {
	sdb, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("unwrap sql.DB: %w", err)
	}
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations source: %w", err)
	}
	driver, err := newSQLiteMigrateDriver(sdb)
	if err != nil {
		return fmt.Errorf("init sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
