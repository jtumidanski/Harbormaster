// Package connection owns the singleton MinIO connection record:
// persistence, validation probe, and the action-style HTTP surface for
// reading and rotating the endpoint configuration.
//
// The plaintext access key, secret key, and custom CA PEM are encrypted at
// rest via internal/crypto.Cipher and never leave the package as anything
// but a masked / *Present view. The cached MinIO client pair in
// internal/minio.Pool is rebuilt after every successful Update.
package connection

// Connection is the immutable read view of the singleton minio_connections
// row. Plaintext secrets are deliberately absent: GET /api/v1/connection
// returns only a masked access key prefix and presence flags for the
// secret key and the optional custom CA PEM.
type Connection struct {
	id                  uint
	endpointURL         string
	tlsSkipVerify       bool
	accessKeyMasked     string
	secretKeyPresent    bool
	customCAPEMPresent  bool
}

// ID returns the database primary key. Always 1 in v1 (singleton).
func (c Connection) ID() uint { return c.id }

// EndpointURL returns the MinIO endpoint URL (http(s)://host:port).
func (c Connection) EndpointURL() string { return c.endpointURL }

// TLSSkipVerify reports whether the operator chose to skip TLS verification.
func (c Connection) TLSSkipVerify() bool { return c.tlsSkipVerify }

// AccessKeyMasked is the first four characters of the access key followed
// by "***". Empty when no access key has been persisted yet.
func (c Connection) AccessKeyMasked() string { return c.accessKeyMasked }

// SecretKeyPresent reports whether a secret key ciphertext is on file.
func (c Connection) SecretKeyPresent() bool { return c.secretKeyPresent }

// CustomCAPEMPresent reports whether a custom CA PEM ciphertext is on file.
func (c Connection) CustomCAPEMPresent() bool { return c.customCAPEMPresent }
