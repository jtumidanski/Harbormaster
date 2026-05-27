package setup

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
)

// Processor coordinates first-run setup. It composes the connection probe,
// the password hasher, and the app_settings flip in a single transaction so
// a partial failure leaves no half-initialised state.
type Processor struct {
	DB       *gorm.DB
	Cipher   *crypto.Cipher
	AuthProc *auth.Processor
	ConnProc *connection.Processor
	McPath   string
}

// ErrAlreadyInitialized is returned by Submit when the setup_completed flag
// is already set in app_settings. HTTP layer maps to 409.
var ErrAlreadyInitialized = errors.New("setup already completed")

// ErrMcAliasNotFound is returned when from_mc_alias references an unknown
// or unreadable alias. HTTP layer maps to 422.
var ErrMcAliasNotFound = errors.New("mc_alias_not_found")

// Request carries the form fields submitted to POST /api/v1/setup.
type Request struct {
	Admin struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"admin"`
	MinIO connection.SubmitInput `json:"minio"`
}

// Submit performs the first-run sequence and is idempotent — it returns
// ErrAlreadyInitialized on the second call. The sourceIP is accepted for
// the audit hook that will be wired once internal/audit is reachable here.
func (p *Processor) Submit(ctx context.Context, req Request, sourceIP string) error {
	if p.isInitialized(ctx) {
		return ErrAlreadyInitialized
	}
	if req.MinIO.FromMcAlias != "" {
		secret, err := ReadMcAliasSecret(p.McPath, req.MinIO.FromMcAlias)
		if err != nil {
			return ErrMcAliasNotFound
		}
		req.MinIO.SecretKey = secret
		aliases, _, _ := ReadMcAliases(p.McPath)
		for _, a := range aliases {
			if a.Name == req.MinIO.FromMcAlias {
				req.MinIO.EndpointURL = a.Endpoint
				req.MinIO.AccessKey = a.AccessKey
				if req.MinIO.TLSSkipVerify == nil {
					v := a.TLSSkipVerify
					req.MinIO.TLSSkipVerify = &v
				}
				break
			}
		}
	}
	if err := p.ConnProc.Validate(ctx, req.MinIO); err != nil {
		return err
	}
	hash, err := auth.HashPassword(req.Admin.Password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO admin_users (username, password_hash, created_at, updated_at) VALUES (?,?,?,?)`,
			req.Admin.Username, hash, now, now).Error; err != nil {
			return err
		}
		if err := p.ConnProc.PersistInTx(ctx, tx, req.MinIO); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES (?,?,?)`,
			"setup_completed", "true", now).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	_ = sourceIP // audit hook added once audit.Processor is wired here
	return nil
}

// isInitialized reads the setup_completed flag from app_settings.
func (p *Processor) isInitialized(ctx context.Context) bool {
	var v string
	p.DB.WithContext(ctx).Raw(`SELECT value FROM app_settings WHERE key = ?`, "setup_completed").Scan(&v)
	return v == "true"
}

// Status returns whether setup has been completed.
func (p *Processor) Status(ctx context.Context) bool {
	return p.isInitialized(ctx)
}
