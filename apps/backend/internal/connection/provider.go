package connection

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrNoConnection is returned by getSingleton when the minio_connections
// table is empty. Processor.Get maps this onto the appropriate apierror.
var ErrNoConnection = errors.New("connection: no row persisted")

// getSingleton returns a curried lookup for the lone minio_connections row.
// The closure expects a context-scoped *gorm.DB from the processor and
// returns the raw entity; the processor is responsible for decryption via
// Make so that this layer remains side-effect-free and crypto-agnostic.
func getSingleton() func(*gorm.DB) (connectionEntity, error) {
	return func(db *gorm.DB) (connectionEntity, error) {
		var e connectionEntity
		err := db.Where("singleton_guard = ?", 1).First(&e).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return connectionEntity{}, ErrNoConnection
			}
			return connectionEntity{}, fmt.Errorf("connection.getSingleton: %w", err)
		}
		return e, nil
	}
}
