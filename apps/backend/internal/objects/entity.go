package objects

// The objects domain has NO local persistence: every read/write goes to
// MinIO via the configured s3 client. There is therefore no GORM entity
// struct here, and no Make / ToEntity pair like internal/connection has —
// the seven-file pattern is preserved for readability even though
// entity.go is intentionally empty of types.
//
// If a future task needs a sidecar (e.g. an object-tag cache or a local
// share-link audit-friendly handle), define the GORM-tagged struct in
// this file to keep the domain layout consistent across packages.
