package connection_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

// stubProbeOK is a Prober that always reports success, used to bypass the
// network during PersistInTx / Get round-trip tests.
func stubProbeOK(_ context.Context, _ connection.SubmitInput) (connection.TestResult, *apierror.Error) {
	return connection.TestResult{
		TCPConnect:   "ok",
		ListBuckets:  "ok",
		AdminPing:    "ok",
		MinIOVersion: "RELEASE.2026-01-01T00-00-00Z",
	}, nil
}

// newProcessor builds a Processor wired to a fresh test DB, a deterministic
// cipher, an empty pool, and the success-stub prober.
func newProcessor(t *testing.T) (*connection.Processor, *gorm.DB) {
	t.Helper()
	gdb := newTestDB(t)
	cipher := newTestCipher(t)
	pool := hmminio.NewEmpty()
	p := connection.NewProcessor(gdb, cipher, pool)
	p.Probe = stubProbeOK
	return p, gdb
}

// TestProcessor_PersistInTx_EncryptsAndGetMasks exercises the write/read
// round-trip end-to-end: PersistInTx must store ciphertext (not plaintext)
// for the access/secret keys, and Get must return the masked view with
// SecretKeyPresent=true. CustomCAPEM is left empty here; a sibling case
// covers the present-CA branch.
func TestProcessor_PersistInTx_EncryptsAndGetMasks(t *testing.T) {
	p, gdb := newProcessor(t)
	ctx := context.Background()

	in := connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIA1234567890",
		SecretKey:   "topsecretvalue",
	}
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return p.PersistInTx(ctx, tx, in)
	}))

	// Ciphertext columns must not contain the plaintext.
	type rawRow struct {
		AccessKeyCiphertext   string  `gorm:"column:access_key_ciphertext"`
		SecretKeyCiphertext   string  `gorm:"column:secret_key_ciphertext"`
		CustomCAPEMCiphertext *string `gorm:"column:custom_ca_pem_ciphertext"`
		TLSSkipVerify         bool    `gorm:"column:tls_skip_verify"`
	}
	var raw rawRow
	require.NoError(t, gdb.Table("minio_connections").
		Select("access_key_ciphertext, secret_key_ciphertext, custom_ca_pem_ciphertext, tls_skip_verify").
		Where("singleton_guard = 1").
		Scan(&raw).Error)
	require.NotEmpty(t, raw.AccessKeyCiphertext)
	require.NotEmpty(t, raw.SecretKeyCiphertext)
	require.False(t, strings.Contains(raw.AccessKeyCiphertext, in.AccessKey),
		"access key ciphertext leaked plaintext")
	require.False(t, strings.Contains(raw.SecretKeyCiphertext, in.SecretKey),
		"secret key ciphertext leaked plaintext")
	require.Nil(t, raw.CustomCAPEMCiphertext, "expected nil custom_ca when input omitted it")
	require.False(t, raw.TLSSkipVerify)

	// Get round-trips back through decryption and produces the masked view.
	view, err := p.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://minio.lan:9000", view.EndpointURL())
	require.False(t, view.TLSSkipVerify())
	require.Equal(t, "AKIA***", view.AccessKeyMasked(),
		"access key should be first-4 + ***")
	require.True(t, view.SecretKeyPresent())
	require.False(t, view.CustomCAPEMPresent())
}

// TestProcessor_PersistInTx_StoresCustomCA verifies the optional CA PEM
// branch: when CustomCAPEM is non-empty, ToEntity encrypts it and Get
// reports CustomCAPEMPresent=true.
func TestProcessor_PersistInTx_StoresCustomCA(t *testing.T) {
	p, gdb := newProcessor(t)
	ctx := context.Background()

	tlsTrue := true
	in := connection.SubmitInput{
		EndpointURL:   "https://minio.lan:9000",
		AccessKey:     "AKIA",
		SecretKey:     "sk",
		TLSSkipVerify: &tlsTrue,
		CustomCAPEM:   "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n",
	}
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return p.PersistInTx(ctx, tx, in)
	}))

	type rawRow struct {
		CustomCAPEMCiphertext *string `gorm:"column:custom_ca_pem_ciphertext"`
	}
	var raw rawRow
	require.NoError(t, gdb.Table("minio_connections").
		Select("custom_ca_pem_ciphertext").
		Where("singleton_guard = 1").
		Scan(&raw).Error)
	require.NotNil(t, raw.CustomCAPEMCiphertext)
	require.NotEmpty(t, *raw.CustomCAPEMCiphertext)
	require.False(t, strings.Contains(*raw.CustomCAPEMCiphertext, "BEGIN CERTIFICATE"),
		"custom CA ciphertext leaked PEM markers")

	view, err := p.Get(ctx)
	require.NoError(t, err)
	require.True(t, view.TLSSkipVerify())
	require.True(t, view.CustomCAPEMPresent())
	// Access key shorter than 4 chars should still be masked.
	require.Equal(t, "AKIA***", view.AccessKeyMasked())
}

// TestProcessor_PersistInTx_UpsertsSingleton verifies the second write
// updates the existing row instead of inserting a duplicate (the singleton
// unique index would reject that).
func TestProcessor_PersistInTx_UpsertsSingleton(t *testing.T) {
	p, gdb := newProcessor(t)
	ctx := context.Background()

	first := connection.SubmitInput{
		EndpointURL: "https://a.lan:9000",
		AccessKey:   "AAAA",
		SecretKey:   "s1",
	}
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return p.PersistInTx(ctx, tx, first)
	}))

	second := connection.SubmitInput{
		EndpointURL: "https://b.lan:9000",
		AccessKey:   "BBBB",
		SecretKey:   "s2",
	}
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return p.PersistInTx(ctx, tx, second)
	}))

	var count int64
	require.NoError(t, gdb.Table("minio_connections").Count(&count).Error)
	require.EqualValues(t, 1, count, "expected exactly one singleton row")

	view, err := p.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://b.lan:9000", view.EndpointURL())
	require.Equal(t, "BBBB***", view.AccessKeyMasked())
}

// TestProcessor_Get_WhenEmptyReturnsNotFound verifies the empty-table
// branch maps to a typed apierror.NotFound (404).
func TestProcessor_Get_WhenEmptyReturnsNotFound(t *testing.T) {
	p, _ := newProcessor(t)
	_, err := p.Get(context.Background())
	require.Error(t, err)
	ae, ok := err.(*apierror.Error)
	require.True(t, ok, "expected *apierror.Error, got %T", err)
	require.Equal(t, "not_found", ae.Code)
}
