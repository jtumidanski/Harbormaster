package users

import (
	"strings"

	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// templatePrefix is the canonical prefix every Harbormaster-materialised
// policy carries. The reverse-mapper uses it to discriminate
// Harbormaster-attached policies (which collapse to TemplateRef entries
// on the read model) from operator-installed policies (which surface as
// OtherPolicies strings).
const templatePrefix = "harbormaster-"

// parsePolicyName reverse-maps a MinIO policy name back into a TemplateRef
// when the name follows the Harbormaster canonical scheme. Returns
// ok=false for any name that does not start with the template prefix —
// those policies belong in User.OtherPolicies so the SPA can render them
// read-only.
//
// Supported shapes:
//   - harbormaster-read-only            → TemplateRef{Name: "read-only"}
//   - harbormaster-read-write           → TemplateRef{Name: "read-write"}
//   - harbormaster-backup-target-<b>    → TemplateRef{Name: "backup-target", Params: {"bucket": b}}
//
// Unknown harbormaster-prefixed names (e.g. a future template the running
// version does not know yet) are returned as TemplateRef{Name: "<rest>"}
// without params so the UI still treats them as managed but the operator
// sees the raw template token.
func parsePolicyName(name string) (TemplateRef, bool) {
	if !strings.HasPrefix(name, templatePrefix) {
		return TemplateRef{}, false
	}
	rest := strings.TrimPrefix(name, templatePrefix)
	if rest == "" {
		return TemplateRef{}, false
	}
	// backup-target carries a bucket suffix. Match the longest fixed
	// prefix first so "backup-target-foo" does not accidentally fall into
	// the parameterless branch.
	if strings.HasPrefix(rest, "backup-target-") {
		bucket := strings.TrimPrefix(rest, "backup-target-")
		if bucket == "" {
			return TemplateRef{}, false
		}
		return TemplateRef{Name: "backup-target", Params: map[string]string{"bucket": bucket}}, true
	}
	switch rest {
	case "read-only", "read-write":
		return TemplateRef{Name: rest}, true
	}
	// Unknown harbormaster-prefixed name — return as parameterless so the
	// UI can still flag it as "managed" without losing track of the raw
	// token.
	return TemplateRef{Name: rest}, true
}

// splitPolicyList turns MinIO's comma-separated UserInfo.PolicyName into a
// trimmed []string. Empty entries are dropped so a trailing comma does
// not leak a "" policy into the reverse-mapper.
func splitPolicyList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// templateKey produces a stable key for a TemplateRef so the diff in
// UpdatePolicies can compare current vs requested without slice ordering
// noise. The format intentionally matches MaterializedName's output so a
// future refactor can deduplicate.
func templateKey(ref TemplateRef) string {
	switch ref.Name {
	case "backup-target":
		return "harbormaster-backup-target-" + ref.Params["bucket"]
	default:
		return "harbormaster-" + ref.Name
	}
}

// classifyAttachments partitions a flat list of MinIO policy names into three
// buckets:
//   - templates: names that parse as a Harbormaster template ref (harbormaster-* prefix)
//   - customOwned: names that are custom-origin AND present in deploymentCustom
//     (i.e. Harbormaster owns the attachment and may diff it)
//   - other: everything else (built-ins like consoleAdmin, foreign custom policies
//     not in the deployment set) — never touched by Harbormaster
func classifyAttachments(names []string, deploymentCustom map[string]struct{}) (templates []TemplateRef, customOwned, other []string) {
	for _, n := range names {
		if ref, ok := parsePolicyName(n); ok {
			templates = append(templates, ref)
			continue
		}
		if policies.OriginFor(n) == policies.OriginCustom {
			if _, inDeployment := deploymentCustom[n]; inDeployment {
				customOwned = append(customOwned, n)
				continue
			}
		}
		other = append(other, n)
	}
	return templates, customOwned, other
}
