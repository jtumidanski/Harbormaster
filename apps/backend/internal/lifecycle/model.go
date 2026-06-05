// Package lifecycle owns the bucket-level lifecycle-rule view of MinIO:
// listing rules currently attached to a bucket, creating a new managed
// "expire after N days" rule (optionally scoped to a key prefix), and
// deleting any rule by ID. Like buckets/objects, the source of truth is
// MinIO itself — nothing in this package persists local state.
//
// The domain distinguishes "managed" rules (created via Harbormaster's
// form, recognisable by a deterministic ID prefix and a minimal config
// shape) from "unmanaged" rules (anything else, e.g. rules pushed via
// `mc ilm import`). Managed rules surface as a structured Rule the UI
// can show in a form; unmanaged rules surface as a redacted human
// summary so an operator can see them and choose to delete them without
// the form trying to parse a config it cannot represent.
package lifecycle

// Managed lifecycle kinds. A managed Rule's Kind is exactly one of these.
const (
	KindExpiration           = "expiration"
	KindNoncurrentExpiration = "noncurrent-expiration"
	KindAbortIncompleteMPU   = "abort-incomplete-multipart"
)

// Rule is the immutable read view of a single lifecycle rule attached
// to a bucket. The shape is intentionally minimal: managed rules carry
// their structured (days, prefix) details; unmanaged rules carry only
// the rule ID and a redacted Summary string. The wire layer renders
// either flavour into the lifecycle_rules JSON:API resource.
//
// The classifier (classifier.go) is the single producer of Rule values
// from the upstream lifecycle.Rule shape; downstream code must never
// construct a Rule literal outside of tests because the Managed flag
// drives the JSON:API attributes shape and getting it wrong would
// surface unmanaged config as a structured "expiration" rule.
type Rule struct {
	// ID is the rule's MinIO-side identifier (XML <ID> on the wire).
	// For managed rules this is the deterministic
	// "harbormaster-expire-<days>d[-<prefix-slug>]" string; for
	// unmanaged rules it's whatever the source tool wrote.
	ID string

	// Managed is true iff the rule was created by Harbormaster's form
	// (deterministic ID, expiration-only, no tag filters, no transition,
	// no AbortIncompleteMultipart). The wire layer keys off this to
	// decide whether to render the structured (days, prefix) attributes
	// or the redacted Summary.
	Managed bool

	// Kind is one of {KindExpiration, KindNoncurrentExpiration, KindAbortIncompleteMPU} for managed rules.
	Kind string

	// Days is the expiration day count for managed rules. Zero for
	// unmanaged rules.
	Days int

	// Prefix is the optional key prefix filter for managed rules.
	// Empty string means "applies to the whole bucket". Unmanaged.
	Prefix string

	// NoncurrentDays is the age (days) after which a noncurrent version
	// expires. Non-zero only for Kind == KindNoncurrentExpiration.
	NoncurrentDays int

	// NewerNoncurrentVersions optionally retains this many newest
	// noncurrent versions before expiring older ones. Zero means "no
	// retention floor". Only meaningful for KindNoncurrentExpiration.
	NewerNoncurrentVersions int

	// DaysAfterInitiation is the age (days) after which an incomplete
	// multipart upload is aborted. Non-zero only for
	// Kind == KindAbortIncompleteMPU.
	DaysAfterInitiation int

	// Summary is the human-readable, value-free description of an
	// unmanaged rule: action count, action kinds, and tag-filter count
	// (NEVER tag keys/values — those may be sensitive). Empty for
	// managed rules.
	Summary string
}
