package users

import (
	"errors"
	"fmt"
	"strings"
)

// UserBuilder accumulates the inputs required to materialize a User from a
// validated access key. Today only the access key participates in
// validation; the builder shape leaves room for future fields to be folded
// in without changing call sites.
type UserBuilder struct {
	accessKey string
}

// NewUserBuilder returns an empty builder. Use the chainable setters and
// terminate with Build() to obtain a validated User value with status
// defaulted to "enabled" and zero attached policies, or an error
// describing the first invariant violation.
func NewUserBuilder() *UserBuilder { return &UserBuilder{} }

// AccessKey records the IAM access key to be validated on Build.
func (b *UserBuilder) AccessKey(s string) *UserBuilder {
	b.accessKey = s
	return b
}

// Build runs MinIO access-key validation and returns a User whose only
// populated fields are AccessKey and Status. Attachment fields are filled
// in later by the processor from live MinIO state.
func (b *UserBuilder) Build() (User, error) {
	if err := ValidateAccessKey(b.accessKey); err != nil {
		return User{}, err
	}
	return User{AccessKey: b.accessKey, Status: "enabled"}, nil
}

// ValidateAccessKey is exported so handlers can reject obvious garbage in
// request decoders without spinning up a full builder.
//
// MinIO's IAM access-key rules (matching the upstream `mc admin user add`
// validation): 3..128 characters, alphanumerics plus a small punctuation
// set; no whitespace; non-empty.
func ValidateAccessKey(s string) error {
	if s == "" {
		return errors.New("access key is required")
	}
	if n := len(s); n < 3 || n > 128 {
		return fmt.Errorf("access key must be 3-128 characters (got %d)", n)
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return errors.New("access key must not contain whitespace")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == '+' || c == '=' || c == '@':
		default:
			return fmt.Errorf("access key contains invalid character %q", c)
		}
	}
	return nil
}
