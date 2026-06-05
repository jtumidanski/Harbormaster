package policies

import "strings"

// builtinPolicies is the set of MinIO server built-in canned policy names.
// Keys use the canonical MinIO casing (e.g. "consoleAdmin"). All lookups
// are done case-insensitively via strings.EqualFold so that a caller cannot
// bypass the guard by varying case (e.g. "ConsoleAdmin", "READONLY").
var builtinPolicies = map[string]struct{}{
	"readonly":     {},
	"readwrite":    {},
	"writeonly":    {},
	"consoleAdmin": {},
	"diagnostics":  {},
}

// isBuiltinName reports whether name matches any built-in policy name,
// ignoring case.
func isBuiltinName(name string) bool {
	for k := range builtinPolicies {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

// isTemplateName treats the whole "harbormaster-" prefix as template-owned so
// operators cannot shadow a template name with a custom policy.
// The comparison is case-insensitive to prevent bypass via "Harbormaster-foo".
func isTemplateName(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "harbormaster-")
}

// OriginFor classifies a canned policy name by provenance: MinIO built-ins
// first, then the Harbormaster-template prefix, else custom.
func OriginFor(name string) string {
	if isBuiltinName(name) {
		return OriginBuiltin
	}
	if isTemplateName(name) {
		return OriginTemplate
	}
	return OriginCustom
}

// EditableFor reports whether a policy may be edited/deleted through
// Harbormaster (custom-origin only).
func EditableFor(name string) bool {
	return OriginFor(name) == OriginCustom
}

// IsBuiltin reports whether name is a MinIO server built-in policy.
func IsBuiltin(name string) bool {
	return isBuiltinName(name)
}
