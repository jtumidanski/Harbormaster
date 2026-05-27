package setup_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/setup"
)

// validRequest returns a Request carrying explicit MinIO credentials.
// Helper used to keep the per-test body short.
func validRequest() setup.Request {
	var req setup.Request
	req.Admin.Username = "admin"
	req.Admin.Password = "correct horse battery staple"
	req.MinIO = connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIAEXAMPLE",
		SecretKey:   "topsecretvalue",
	}
	return req
}

// TestStatus_FalseThenTrueAfterSubmit verifies that Status flips from false
// to true once Submit succeeds.
func TestStatus_FalseThenTrueAfterSubmit(t *testing.T) {
	p, _ := newProcessor(t, "/nonexistent")
	ctx := context.Background()

	require.False(t, p.Status(ctx))
	require.NoError(t, p.Submit(ctx, validRequest(), "127.0.0.1"))
	require.True(t, p.Status(ctx))
}

// TestSubmit_PersistsAdminAndConnection verifies the happy path writes both
// the admin_users row (with a non-plaintext password hash) and the
// minio_connections row, and flips the setup_completed flag.
func TestSubmit_PersistsAdminAndConnection(t *testing.T) {
	p, gdb := newProcessor(t, "/nonexistent")
	ctx := context.Background()
	req := validRequest()

	require.NoError(t, p.Submit(ctx, req, "127.0.0.1"))

	// admin_users row exists and stores an argon2id hash, not plaintext.
	type adminRow struct {
		Username     string `gorm:"column:username"`
		PasswordHash string `gorm:"column:password_hash"`
	}
	var au adminRow
	require.NoError(t, gdb.Table("admin_users").
		Select("username, password_hash").
		Where("username = ?", req.Admin.Username).
		Scan(&au).Error)
	require.Equal(t, "admin", au.Username)
	require.NotEmpty(t, au.PasswordHash)
	require.NotEqual(t, req.Admin.Password, au.PasswordHash, "password must be hashed")
	require.NoError(t, auth.VerifyPassword(au.PasswordHash, req.Admin.Password),
		"stored hash must verify against the submitted password")

	// minio_connections row exists with the expected endpoint.
	var endpoint string
	require.NoError(t, gdb.Table("minio_connections").
		Select("endpoint_url").
		Where("singleton_guard = 1").
		Scan(&endpoint).Error)
	require.Equal(t, "https://minio.lan:9000", endpoint)

	// app_settings flag set.
	var v string
	require.NoError(t, gdb.Table("app_settings").
		Select("value").
		Where("key = ?", "setup_completed").
		Scan(&v).Error)
	require.Equal(t, "true", v)
}

// TestSubmit_FromMcAlias verifies that when the request references an alias,
// the secret key is fetched from the mc-config file and the endpoint and
// access key are overridden from the alias entry.
func TestSubmit_FromMcAlias(t *testing.T) {
	dir := t.TempDir()
	mcPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(mcPath, []byte(`{
		"version": "10",
		"aliases": {
			"myminio": {"url": "https://alias.lan:9000", "accessKey": "AKIAFROMALIAS", "secretKey": "secretfromalias", "insecure": true}
		}
	}`), 0o600))

	p, gdb := newProcessor(t, mcPath)
	ctx := context.Background()

	var req setup.Request
	req.Admin.Username = "admin"
	req.Admin.Password = "x" + "correct horse battery staple"
	req.MinIO = connection.SubmitInput{
		// EndpointURL/AccessKey deliberately empty; they must be filled
		// from the alias entry by Submit.
		FromMcAlias: "myminio",
	}

	require.NoError(t, p.Submit(ctx, req, "127.0.0.1"))

	// minio_connections row should reflect the alias entry's endpoint and
	// the tls_skip_verify=true flag from "insecure": true.
	type row struct {
		EndpointURL   string `gorm:"column:endpoint_url"`
		TLSSkipVerify bool   `gorm:"column:tls_skip_verify"`
	}
	var r row
	require.NoError(t, gdb.Table("minio_connections").
		Select("endpoint_url, tls_skip_verify").
		Where("singleton_guard = 1").
		Scan(&r).Error)
	require.Equal(t, "https://alias.lan:9000", r.EndpointURL)
	require.True(t, r.TLSSkipVerify)
}

// TestSubmit_SecondCallReturnsAlreadyInitialized verifies idempotence: a
// second Submit after a successful first attempt returns
// setup.ErrAlreadyInitialized without mutating state.
func TestSubmit_SecondCallReturnsAlreadyInitialized(t *testing.T) {
	p, _ := newProcessor(t, "/nonexistent")
	ctx := context.Background()

	require.NoError(t, p.Submit(ctx, validRequest(), "127.0.0.1"))
	err := p.Submit(ctx, validRequest(), "127.0.0.1")
	require.Error(t, err)
	require.True(t, errors.Is(err, setup.ErrAlreadyInitialized))
}

// TestSubmit_McAliasNotFound verifies that referencing an unknown alias
// returns ErrMcAliasNotFound and does not touch the database.
func TestSubmit_McAliasNotFound(t *testing.T) {
	dir := t.TempDir()
	mcPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(mcPath, []byte(`{
		"version": "10",
		"aliases": {"different": {"url":"u","accessKey":"a","secretKey":"s","insecure":false}}
	}`), 0o600))

	p, gdb := newProcessor(t, mcPath)
	ctx := context.Background()

	var req setup.Request
	req.Admin.Username = "admin"
	req.Admin.Password = "pw"
	req.MinIO.FromMcAlias = "missing"

	err := p.Submit(ctx, req, "127.0.0.1")
	require.Error(t, err)
	require.True(t, errors.Is(err, setup.ErrMcAliasNotFound))

	// admin_users must not have been touched.
	var count int64
	require.NoError(t, gdb.Table("admin_users").Count(&count).Error)
	require.EqualValues(t, 0, count, "no admin row should be written on alias miss")
	require.False(t, p.Status(ctx))
}
