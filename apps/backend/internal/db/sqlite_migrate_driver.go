package db

// sqliteMigrateDriver is an inline copy of github.com/golang-migrate/migrate/v4/database/sqlite
// without the `_ "modernc.org/sqlite"` blank import, which would cause a
// duplicate-driver registration panic because glebarez/go-sqlite already
// registers the "sqlite" driver name.

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/golang-migrate/migrate/v4/database"
)

const defaultMigrationsTable = "schema_migrations"

var errNilConfig = fmt.Errorf("no config")

type sqliteMigrateConfig struct {
	MigrationsTable string
	NoTxWrap        bool
}

type sqliteMigrateDriver struct {
	db       *sql.DB
	isLocked atomic.Bool
	config   *sqliteMigrateConfig
}

func newSQLiteMigrateDriver(instance *sql.DB) (database.Driver, error) {
	cfg := &sqliteMigrateConfig{MigrationsTable: defaultMigrationsTable}
	if instance == nil {
		return nil, errNilConfig
	}
	if err := instance.Ping(); err != nil {
		return nil, err
	}
	drv := &sqliteMigrateDriver{db: instance, config: cfg}
	if err := drv.ensureVersionTable(); err != nil {
		return nil, err
	}
	return drv, nil
}

func (m *sqliteMigrateDriver) ensureVersionTable() (err error) {
	if err = m.Lock(); err != nil {
		return err
	}
	defer func() {
		if e := m.Unlock(); e != nil {
			err = errors.Join(err, e)
		}
	}()
	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (version uint64, dirty bool);
	CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON %s (version);
	`, m.config.MigrationsTable, m.config.MigrationsTable)
	if _, err := m.db.Exec(query); err != nil {
		return err
	}
	return nil
}

func (m *sqliteMigrateDriver) Open(_ string) (database.Driver, error) {
	return nil, fmt.Errorf("sqliteMigrateDriver.Open not supported; use newSQLiteMigrateDriver")
}

func (m *sqliteMigrateDriver) Close() error { return nil } // caller owns the *sql.DB

func (m *sqliteMigrateDriver) Drop() (err error) {
	query := `SELECT name FROM sqlite_master WHERE type = 'table';`
	tables, err := m.db.Query(query)
	if err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	defer func() {
		if errClose := tables.Close(); errClose != nil {
			err = errors.Join(err, errClose)
		}
	}()
	var tableNames []string
	for tables.Next() {
		var t string
		if err := tables.Scan(&t); err != nil {
			return err
		}
		if t != "" {
			tableNames = append(tableNames, t)
		}
	}
	if err := tables.Err(); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	for _, t := range tableNames {
		q := "DROP TABLE " + t
		if err := m.executeQuery(q); err != nil {
			return &database.Error{OrigErr: err, Query: []byte(q)}
		}
	}
	if len(tableNames) > 0 {
		if _, err := m.db.Query("VACUUM"); err != nil {
			return &database.Error{OrigErr: err, Query: []byte("VACUUM")}
		}
	}
	return nil
}

func (m *sqliteMigrateDriver) Lock() error {
	if !m.isLocked.CompareAndSwap(false, true) {
		return database.ErrLocked
	}
	return nil
}

func (m *sqliteMigrateDriver) Unlock() error {
	if !m.isLocked.CompareAndSwap(true, false) {
		return database.ErrNotLocked
	}
	return nil
}

func (m *sqliteMigrateDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return err
	}
	if m.config.NoTxWrap {
		return m.executeQueryNoTx(string(migr))
	}
	return m.executeQuery(string(migr))
}

func (m *sqliteMigrateDriver) executeQuery(query string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}
	if _, err := tx.Exec(query); err != nil {
		if errRollback := tx.Rollback(); errRollback != nil {
			err = errors.Join(err, errRollback)
		}
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}
	return nil
}

func (m *sqliteMigrateDriver) executeQueryNoTx(query string) error {
	if _, err := m.db.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	return nil
}

func (m *sqliteMigrateDriver) SetVersion(version int, dirty bool) error {
	tx, err := m.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}
	query := "DELETE FROM " + m.config.MigrationsTable
	if _, err := tx.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	if version >= 0 || (version == database.NilVersion && dirty) {
		q := fmt.Sprintf(`INSERT INTO %s (version, dirty) VALUES (?, ?)`, m.config.MigrationsTable)
		if _, err := tx.Exec(q, version, dirty); err != nil {
			if errRollback := tx.Rollback(); errRollback != nil {
				err = errors.Join(err, errRollback)
			}
			return &database.Error{OrigErr: err, Query: []byte(q)}
		}
	}
	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}
	return nil
}

func (m *sqliteMigrateDriver) Version() (version int, dirty bool, err error) {
	query := "SELECT version, dirty FROM " + m.config.MigrationsTable + " LIMIT 1"
	err = m.db.QueryRow(query).Scan(&version, &dirty)
	if err != nil {
		return database.NilVersion, false, nil
	}
	return version, dirty, nil
}

// Ensure sqliteMigrateDriver satisfies the interface at compile time.
var _ database.Driver = (*sqliteMigrateDriver)(nil)
