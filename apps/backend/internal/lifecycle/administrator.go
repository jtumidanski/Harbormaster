package lifecycle

// The lifecycle domain's processor calls SetBucketLifecycle and
// GetBucketLifecycle directly through the unexported s3API interface
// — there's no intermediate "administrator" helper file like
// internal/objects has because the call patterns are trivial
// (read-modify-write on a single XML blob, no pagination, no
// concurrency).
//
// The seven-file scaffold is preserved for readability and so a
// future task that needs a richer admin-side surface (e.g. issuing
// `mc ilm reset` or routing through madmin for noncurrent-version
// quirks) has a stable home for those helpers.
