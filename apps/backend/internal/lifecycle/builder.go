package lifecycle

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// prefixSlugRE matches the characters that are safe inside the managed
// rule ID's optional "-<prefix-slug>" tail. Anything outside this set
// is collapsed to '-' by slugifyPrefix; the result is then trimmed to
// avoid the ID ending in a delimiter.
var prefixSlugRE = regexp.MustCompile(`[^a-z0-9.]+`)

// MaxRuleIDLen is the upper bound on a generated rule ID. MinIO's
// lifecycle spec caps rule IDs at 255 chars; we stay well under so the
// "harbormaster-expire-" prefix plus a long-but-not-pathological prefix
// slug still fits comfortably.
const MaxRuleIDLen = 200

// generateRuleID returns the deterministic ID a managed expiration rule
// must use: "harbormaster-expire-<days>d" with an optional
// "-<prefix-slug>" tail when prefix is non-empty. The slug is the
// prefix lower-cased, restricted to [a-z0-9.-] (other chars collapsed
// to '-'), with surrounding delimiters trimmed.
//
// The deterministic format is the *only* signal the classifier uses to
// recognise our rules later (paired with a shape check), so the
// formatting MUST stay in lock-step with managedIDRE in classifier.go.
func generateRuleID(days int, prefix string) string {
	id := fmt.Sprintf("harbormaster-expire-%dd", days)
	if prefix == "" {
		return id
	}
	slug := slugifyPrefix(prefix)
	if slug == "" {
		return id
	}
	full := id + "-" + slug
	if len(full) > MaxRuleIDLen {
		full = full[:MaxRuleIDLen]
		// A trailing '-' or '.' looks ugly and trips the strict regex;
		// trim it after truncation.
		full = strings.TrimRight(full, "-.")
	}
	return full
}

// slugifyPrefix lower-cases prefix, collapses any non-[a-z0-9.] run to
// a single '-', and trims leading/trailing delimiters. The result is
// safe to splice into the managed rule ID without breaking
// managedIDRE.
func slugifyPrefix(prefix string) string {
	s := strings.ToLower(prefix)
	s = prefixSlugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	return s
}

// validateDays enforces the operator-facing contract on the "days"
// field: must be a positive integer. MinIO itself accepts much larger
// values, but allowing zero/negative here would silently produce a
// rule that expires every object immediately (or never, depending on
// the server's interpretation), neither of which is what the operator
// asked for.
func validateDays(days int) error {
	if days <= 0 {
		return errors.New("days must be > 0")
	}
	return nil
}
