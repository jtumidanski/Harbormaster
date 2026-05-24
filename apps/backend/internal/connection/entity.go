package connection

import (
	"fmt"
	"time"

	"github.com/jtumidanski/Harbormaster/internal/crypto"
)

// connectionEntity is the GORM persistence struct for minio_connections.
// Ciphertext columns hold base64(nonce || ct || tag) envelopes produced by
// crypto.Cipher.Encrypt. They never leave the package as plaintext except
// via the in-process plainCreds value built from Make.
type connectionEntity struct {
	ID                     uint    `gorm:"column:id;primaryKey;autoIncrement"`
	SingletonGuard         int     `gorm:"column:singleton_guard;not null;default:1;uniqueIndex"`
	EndpointURL            string  `gorm:"column:endpoint_url;not null"`
	TLSSkipVerify          bool    `gorm:"column:tls_skip_verify;not null;default:false"`
	AccessKeyCiphertext    string  `gorm:"column:access_key_ciphertext;not null"`
	SecretKeyCiphertext    string  `gorm:"column:secret_key_ciphertext;not null"`
	CustomCAPEMCiphertext  *string `gorm:"column:custom_ca_pem_ciphertext"`
	CreatedAt              string  `gorm:"column:created_at;not null"`
	UpdatedAt              string  `gorm:"column:updated_at;not null"`
}

// TableName satisfies gorm.Tabler.
func (connectionEntity) TableName() string { return "minio_connections" }

// plainCreds carries the decrypted credential trio alongside the masked
// view. It is package-private; only Processor.Update hands the values to
// minio.Pool.Rebuild before letting them fall out of scope.
type plainCreds struct {
	AccessKey       string
	SecretKey       string
	CustomCAPEMText string
}

// Make decrypts e using cipher and returns (Connection, plainCreds). The
// Connection is the safe read view (masked access key, *Present flags);
// plainCreds holds the values needed to rebuild the live MinIO client pair.
//
// Errors are returned as soon as any column fails to decrypt — partial
// returns would mask a key-mismatch from the caller.
func Make(e connectionEntity, cipher *crypto.Cipher) (Connection, plainCreds, error) {
	akPlain, err := cipher.Decrypt(e.AccessKeyCiphertext)
	if err != nil {
		return Connection{}, plainCreds{}, fmt.Errorf("connection.Make: access_key: %w", err)
	}
	skPlain, err := cipher.Decrypt(e.SecretKeyCiphertext)
	if err != nil {
		return Connection{}, plainCreds{}, fmt.Errorf("connection.Make: secret_key: %w", err)
	}
	var caPlain string
	customPresent := false
	if e.CustomCAPEMCiphertext != nil && *e.CustomCAPEMCiphertext != "" {
		raw, err := cipher.Decrypt(*e.CustomCAPEMCiphertext)
		if err != nil {
			return Connection{}, plainCreds{}, fmt.Errorf("connection.Make: custom_ca: %w", err)
		}
		caPlain = string(raw)
		customPresent = true
	}
	creds := plainCreds{
		AccessKey:       string(akPlain),
		SecretKey:       string(skPlain),
		CustomCAPEMText: caPlain,
	}
	view := Connection{
		id:                 e.ID,
		endpointURL:        e.EndpointURL,
		tlsSkipVerify:      e.TLSSkipVerify,
		accessKeyMasked:    maskAccessKey(creds.AccessKey),
		secretKeyPresent:   creds.SecretKey != "",
		customCAPEMPresent: customPresent,
	}
	return view, creds, nil
}

// ToEntity encrypts the secrets carried in in and returns a fresh
// connectionEntity ready to be UPSERTed. CreatedAt is set on first write
// (caller may overwrite by reading the prior row); UpdatedAt is always now.
func ToEntity(in SubmitInput, cipher *crypto.Cipher, now time.Time) (connectionEntity, error) {
	skipVerify := false
	if in.TLSSkipVerify != nil {
		skipVerify = *in.TLSSkipVerify
	}
	ak, err := cipher.Encrypt([]byte(in.AccessKey))
	if err != nil {
		return connectionEntity{}, fmt.Errorf("connection.ToEntity: access_key encrypt: %w", err)
	}
	sk, err := cipher.Encrypt([]byte(in.SecretKey))
	if err != nil {
		return connectionEntity{}, fmt.Errorf("connection.ToEntity: secret_key encrypt: %w", err)
	}
	var caPtr *string
	if in.CustomCAPEM != "" {
		ca, err := cipher.Encrypt([]byte(in.CustomCAPEM))
		if err != nil {
			return connectionEntity{}, fmt.Errorf("connection.ToEntity: custom_ca encrypt: %w", err)
		}
		caPtr = &ca
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	return connectionEntity{
		SingletonGuard:        1,
		EndpointURL:           in.EndpointURL,
		TLSSkipVerify:         skipVerify,
		AccessKeyCiphertext:   ak,
		SecretKeyCiphertext:   sk,
		CustomCAPEMCiphertext: caPtr,
		CreatedAt:             stamp,
		UpdatedAt:             stamp,
	}, nil
}

// maskAccessKey returns the first four characters of key followed by "***".
// Keys shorter than four characters are returned verbatim followed by "***"
// so the *Present flag is never the only signal that something is on file.
func maskAccessKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return key + "***"
	}
	return key[:4] + "***"
}
