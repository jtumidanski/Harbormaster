package lifecycle

// The lifecycle domain has NO local persistence: rule state lives in
// MinIO. There is therefore no GORM entity struct here, and no Make /
// ToEntity pair like internal/connection has — the seven-file pattern
// is preserved for readability even though entity.go is intentionally
// empty of types.
//
// If a future task needs a sidecar (e.g. a per-bucket "managed-by"
// override the operator can toggle without writing the rule into
// MinIO), define the GORM-tagged struct in this file to keep the
// domain layout consistent across packages.
