package objects

import (
	"errors"
	"strings"
)

// ValidateObjectKey runs the minimal sanity checks an object-key field
// must satisfy before we round-trip to MinIO. S3 object keys are
// permissive (almost any UTF-8 sequence up to 1024 bytes is allowed) but
// Harbormaster rejects empty and oversized keys at the edge so the UI
// gets a typed envelope rather than a raw 400 from MinIO.
//
// The 1024-byte ceiling matches the S3 protocol limit on key length.
func ValidateObjectKey(key string) error {
	if key == "" {
		return errors.New("object key is required")
	}
	if len(key) > 1024 {
		return errors.New("object key must be at most 1024 bytes")
	}
	// MinIO accepts keys containing NULs at the wire level, but they
	// confuse downstream tooling (and most filesystems can't represent
	// them). Reject up front.
	if strings.ContainsRune(key, '\x00') {
		return errors.New("object key must not contain NUL bytes")
	}
	return nil
}
