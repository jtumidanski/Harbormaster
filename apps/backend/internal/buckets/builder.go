package buckets

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// BucketBuilder accumulates the inputs required to materialize a Bucket
// from a validated name. Today only the name participates in validation;
// the builder shape leaves room for future fields (region, lock mode) to be
// folded in without changing call sites.
type BucketBuilder struct {
	name string
}

// NewBucketBuilder returns an empty builder. Use the chainable setters and
// terminate with Build() to obtain a validated Bucket value (auxiliary
// fields zeroed) or an error describing the first invariant violation.
func NewBucketBuilder() *BucketBuilder { return &BucketBuilder{} }

// Name records the bucket name to be validated on Build.
func (b *BucketBuilder) Name(s string) *BucketBuilder {
	b.name = s
	return b
}

// Build runs MinIO bucket-name validation (a strict subset of the S3 rules
// MinIO enforces server-side; we mirror them here so the wizard can reject
// invalid input before any network round-trip) and returns a Bucket whose
// only populated field is Name. Auxiliary attributes are filled in by the
// provider/administrator helpers from live MinIO state.
func (b *BucketBuilder) Build() (Bucket, error) {
	if err := ValidateBucketName(b.name); err != nil {
		return Bucket{}, err
	}
	return Bucket{Name: b.name}, nil
}

// ValidateBucketName is exported so handlers can reject obvious garbage in
// request decoders without spinning up a full builder.
//
// Rules (per MinIO server's strict mode, which Harbormaster enforces
// universally — the relaxed S3 rules permit hostnames that confuse virtual-
// hosted-style addressing):
//
//   - 3..63 characters
//   - lowercase letters, digits, hyphen (-), and dot (.) only
//   - must not start or end with a hyphen or dot
//   - must not contain two adjacent dots
//   - must not be formatted as an IPv4 / IPv6 address
func ValidateBucketName(name string) error {
	if name == "" {
		return errors.New("bucket name is required")
	}
	if n := len(name); n < 3 || n > 63 {
		return fmt.Errorf("bucket name must be 3-63 characters (got %d)", n)
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") {
		return errors.New("bucket name must not start with '.' or '-'")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, "-") {
		return errors.New("bucket name must not end with '.' or '-'")
	}
	if strings.Contains(name, "..") {
		return errors.New("bucket name must not contain adjacent dots")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '.':
		default:
			return fmt.Errorf("bucket name contains invalid character %q (lowercase alnum, '.', '-' only)", c)
		}
	}
	if ip := net.ParseIP(name); ip != nil {
		return errors.New("bucket name must not be formatted as an IP address")
	}
	return nil
}
