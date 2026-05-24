package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// registerBusyRetry installs GORM callbacks that retry SQLITE_BUSY on
// write operations with exponential backoff (max 5 attempts, 1s ceiling).
// Each operation type (create/update/delete/raw) gets its own after-hook
// registered on the correct callback processor.
func registerBusyRetry(gdb *gorm.DB) error {
	hook := func(name string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			if tx.Error == nil {
				return
			}
			if !isBusy(tx.Error) {
				return
			}
			backoff := 25 * time.Millisecond
			for i := 0; i < 4; i++ {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > time.Second {
					backoff = time.Second
				}
				tx.Error = nil
				tx.Statement.SQL.Reset()
				tx.Statement.Vars = nil
				switch name {
				case "create":
					tx.Callback().Create().Execute(tx)
				case "update":
					tx.Callback().Update().Execute(tx)
				case "delete":
					tx.Callback().Delete().Execute(tx)
				case "raw":
					tx.Callback().Raw().Execute(tx)
				}
				if tx.Error == nil || !isBusy(tx.Error) {
					return
				}
			}
		}
	}

	if err := gdb.Callback().Create().After("gorm:create").Register("hm:busy_retry_create", hook("create")); err != nil {
		return err
	}
	if err := gdb.Callback().Update().After("gorm:update").Register("hm:busy_retry_update", hook("update")); err != nil {
		return err
	}
	if err := gdb.Callback().Delete().After("gorm:delete").Register("hm:busy_retry_delete", hook("delete")); err != nil {
		return err
	}
	if err := gdb.Callback().Raw().After("gorm:raw").Register("hm:busy_retry_raw", hook("raw")); err != nil {
		return err
	}
	return nil
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked") {
		return true
	}
	return errors.Is(err, gorm.ErrInvalidTransaction)
}
