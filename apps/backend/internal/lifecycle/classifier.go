package lifecycle

import (
	"fmt"
	"regexp"
	"strings"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
)

// Managed-ID regexes, one per family. A rule's ID must match exactly one
// of these as a precondition of the managed path; the per-family shape
// check below gates the actual Managed verdict.
var (
	expireIDRE     = regexp.MustCompile(`^harbormaster-expire-\d+d(-[a-z0-9.-]+)?$`)
	noncurrentIDRE = regexp.MustCompile(`^harbormaster-noncurrent-[a-z0-9.-]+-\d+d$`)
	abortMPUIDRE   = regexp.MustCompile(`^harbormaster-abortmpu-[a-z0-9.-]+-\d+d$`)
)

// classify maps an upstream lifecycle.Rule into the domain Rule shape. A
// rule is "managed" iff its ID matches exactly one managed family AND its
// server-side config is exactly that family's action with nothing foreign
// (no other actions, no tag filters). Any drift flips it to "unmanaged".
func classify(r mlifecycle.Rule) Rule {
	switch {
	case expireIDRE.MatchString(r.ID) && isExpirationShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindExpiration,
			Days: int(r.Expiration.Days), Prefix: r.RuleFilter.Prefix,
		}
	case noncurrentIDRE.MatchString(r.ID) && isNoncurrentShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindNoncurrentExpiration,
			NoncurrentDays:          int(r.NoncurrentVersionExpiration.NoncurrentDays),
			NewerNoncurrentVersions: int(r.NoncurrentVersionExpiration.NewerNoncurrentVersions),
			Prefix:                  r.RuleFilter.Prefix,
		}
	case abortMPUIDRE.MatchString(r.ID) && isAbortMPUShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindAbortIncompleteMPU,
			DaysAfterInitiation: int(r.AbortIncompleteMultipartUpload.DaysAfterInitiation),
			Prefix:              r.RuleFilter.Prefix,
		}
	}
	return Rule{ID: r.ID, Managed: false, Summary: summarize(r)}
}

// isExpirationShaped is true iff r carries exactly one Expiration action
// (positive days), no other actions, and no tag filters.
func isExpirationShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.NoncurrentVersionExpiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.NewerNoncurrentVersions == 0 &&
		r.AbortIncompleteMultipartUpload.IsDaysNull() &&
		r.DelMarkerExpiration.IsNull() &&
		!r.Expiration.IsDaysNull() && int(r.Expiration.Days) > 0 &&
		hasNoTagFilters(r)
}

// isNoncurrentShaped is true iff r carries exactly one
// NoncurrentVersionExpiration action (positive NoncurrentDays), no other
// actions, and no tag filters.
func isNoncurrentShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.Expiration.IsDaysNull() &&
		r.AbortIncompleteMultipartUpload.IsDaysNull() &&
		r.DelMarkerExpiration.IsNull() &&
		!r.NoncurrentVersionExpiration.IsDaysNull() &&
		int(r.NoncurrentVersionExpiration.NoncurrentDays) > 0 &&
		hasNoTagFilters(r)
}

// isAbortMPUShaped is true iff r carries exactly one
// AbortIncompleteMultipartUpload action (positive DaysAfterInitiation), no
// other actions, and no tag filters.
func isAbortMPUShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.Expiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.NewerNoncurrentVersions == 0 &&
		r.DelMarkerExpiration.IsNull() &&
		!r.AbortIncompleteMultipartUpload.IsDaysNull() &&
		hasNoTagFilters(r)
}

// summarize composes the human-readable, value-free description an
// unmanaged rule surfaces through the JSON:API attributes shape. The
// summary lists the *kinds* of actions present (Expiration, Transition,
// AbortIncompleteMultipart, NoncurrentVersionExpiration, …) plus a
// count of tag filters scoping the rule — NEVER the tag keys or values
// themselves, which may be sensitive.
//
// The format intentionally mirrors api-contracts.md §lifecycle-rules
// (e.g. "Unmanaged rule (created outside Harbormaster) — 2 actions:
// Transition, AbortIncompleteMultipart; scoped to 1 tag filter(s)") so
// the SPA can display the string verbatim with no further parsing.
func summarize(r mlifecycle.Rule) string {
	var parts []string
	if !r.Expiration.IsDaysNull() && int(r.Expiration.Days) > 0 {
		parts = append(parts, "Expiration")
	}
	if !r.Transition.IsNull() {
		parts = append(parts, "Transition")
	}
	if r.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
		parts = append(parts, "AbortIncompleteMultipart")
	}
	if !r.NoncurrentVersionExpiration.IsDaysNull() {
		parts = append(parts, "NoncurrentVersionExpiration")
	}
	if r.NoncurrentVersionTransition.StorageClass != "" {
		parts = append(parts, "NoncurrentVersionTransition")
	}
	if r.DelMarkerExpiration.Days > 0 {
		parts = append(parts, "DelMarkerExpiration")
	}

	summary := fmt.Sprintf("Unmanaged rule (created outside Harbormaster) — %d actions: %s",
		len(parts), strings.Join(parts, ", "))
	if n := countTagFilters(r); n > 0 {
		summary += fmt.Sprintf("; scoped to %d tag filter(s)", n)
	}
	return summary
}

// hasNoTagFilters returns true when the rule's RuleFilter does not pin
// any tags. A managed rule must have no tag filters because
// Harbormaster's create form intentionally does not surface them
// (api-contracts.md §lifecycle-rules POST), so a rule with tags cannot
// have been produced by us.
func hasNoTagFilters(r mlifecycle.Rule) bool {
	return countTagFilters(r) == 0
}

// countTagFilters returns the number of tag filters that scope this
// rule. lifecycle.Rule supports two tag-shapes:
//   - RuleFilter.Tag: a single Tag struct (empty when unused; IsEmpty()
//     checks both key and value are blank).
//   - RuleFilter.And.Tags: a multi-tag conjunction nested under an <And>
//     element. The slice is the authoritative count.
//
// The caller never sees the tag keys/values — only the count is used
// (in summarize / hasNoTagFilters) so unmanaged-rule tag contents
// never reach the JSON:API surface.
func countTagFilters(r mlifecycle.Rule) int {
	n := 0
	if !r.RuleFilter.Tag.IsEmpty() {
		n++
	}
	n += len(r.RuleFilter.And.Tags)
	return n
}
