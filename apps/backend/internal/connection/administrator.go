package connection

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// upsertSingleton writes the singleton minio_connections row. If a row
// already exists (singleton_guard = 1), every mutable column is updated
// in place and CreatedAt is preserved from the prior row; otherwise the
// row in e (CreatedAt == UpdatedAt) is inserted as-is.
//
// The function is intentionally GORM-flavoured rather than a raw INSERT …
// ON CONFLICT so it works against both SQLite (production) and SQLite-in-
// memory (tests) without driver-specific SQL.
func upsertSingleton(db *gorm.DB, e connectionEntity) error {
	var existing connectionEntity
	err := db.Where("singleton_guard = ?", 1).First(&existing).Error
	switch {
	case err == nil:
		// Preserve the original CreatedAt; bump UpdatedAt to e.UpdatedAt
		// (or to now if the caller left it zero, which would be a bug).
		updated := e.UpdatedAt
		if updated == "" {
			updated = time.Now().UTC().Format(time.RFC3339Nano)
		}
		res := db.Model(&connectionEntity{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"endpoint_url":             e.EndpointURL,
				"tls_skip_verify":          e.TLSSkipVerify,
				"access_key_ciphertext":    e.AccessKeyCiphertext,
				"secret_key_ciphertext":    e.SecretKeyCiphertext,
				"custom_ca_pem_ciphertext": e.CustomCAPEMCiphertext,
				"updated_at":               updated,
			})
		if res.Error != nil {
			return fmt.Errorf("connection.upsertSingleton.update: %w", res.Error)
		}
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := db.Create(&e).Error; err != nil {
			return fmt.Errorf("connection.upsertSingleton.create: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("connection.upsertSingleton.lookup: %w", err)
	}
}
