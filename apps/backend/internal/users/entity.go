package users

// The users domain has NO local persistence: every read goes to MinIO via
// madmin and every write is forwarded to the same. There is therefore no
// GORM entity struct here, and no Make / ToEntity pair like
// internal/connection has — the seven-file pattern is preserved for
// readability even though entity.go is intentionally empty of types.
//
// If a future task needs a local cache or sidecar metadata (e.g. an audit
// label per user, or a "created via Harbormaster" flag) define the
// GORM-tagged struct in this file to keep the domain layout consistent
// across packages.
