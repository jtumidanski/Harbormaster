package audit

import "regexp"

// sensitiveKeyRE matches key names that must never be persisted.
// "url" is intentionally included so presigned URLs, share URLs, etc. are
// always dropped regardless of how the caller names the key.
var sensitiveKeyRE = regexp.MustCompile(`(?i)(secret|password|token|csrf|signature|presigned|url)`)

// Sanitize returns a shallow copy of m with every key matching sensitiveKeyRE
// removed. Nested map[string]any values are recursively sanitized. Nil input
// returns nil. The original map is never modified.
func Sanitize(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sensitiveKeyRE.MatchString(k) {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = Sanitize(nested)
			continue
		}
		out[k] = v
	}
	return out
}
