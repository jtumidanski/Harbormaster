package lifecycle

import (
	"fmt"
	"regexp"
	"strings"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
)

// managedIDRE matches the deterministic ID format generateRuleID emits.
// The optional "-<prefix-slug>" tail mirrors slugifyPrefix's allowed
// charset (lowercase alphanumerics, dots, hyphens). A rule's ID must
// match this pattern as a *precondition* of the managed-classification
// path; the full shape check (no transition, no abort, no tag filter,
// expiration only, positive days) gates the actual Managed=true verdict
// below.
var managedIDRE = regexp.MustCompile(`^harbormaster-expire-\d+d(-[a-z0-9.-]+)?$`)

// classify maps an upstream lifecycle.Rule into the domain Rule shape.
// The decision tree is intentionally narrow: a rule is "managed" iff it
// matches the deterministic ID format AND its server-side config matches
// the exact subset Harbormaster's create form produces (expiration-only,
// no transitions, no abort-incomplete-multipart, no tag filters). Any
// drift — even one a future admin tool added to the same rule ID — flips
// the classification to "unmanaged" so the form never tries to render
// config it cannot represent.
//
// Unmanaged rules carry a value-free Summary string (action count, kind
// list, tag-filter count) so the UI can show "something exists here"
// without leaking potentially sensitive tag keys/values.
func classify(r mlifecycle.Rule) Rule {
	if managedIDRE.MatchString(r.ID) &&
		r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.NoncurrentVersionExpiration.IsDaysNull() &&
		r.AbortIncompleteMultipartUpload.DaysAfterInitiation == 0 &&
		r.DelMarkerExpiration.Days == 0 &&
		!r.Expiration.IsDaysNull() &&
		int(r.Expiration.Days) > 0 &&
		hasNoTagFilters(r) {
		return Rule{
			ID:      r.ID,
			Managed: true,
			Kind:    "expiration",
			Days:    int(r.Expiration.Days),
			Prefix:  r.RuleFilter.Prefix,
		}
	}
	return Rule{ID: r.ID, Managed: false, Summary: summarize(r)}
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
