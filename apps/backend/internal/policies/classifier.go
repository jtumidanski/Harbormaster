package policies

import "strings"

// builtinPolicies is the set of MinIO server built-in canned policy names.
var builtinPolicies = map[string]struct{}{
	"readonly":     {},
	"readwrite":    {},
	"writeonly":    {},
	"consoleAdmin": {},
	"diagnostics":  {},
}

// isTemplateName treats the whole "harbormaster-" prefix as template-owned so
// operators cannot shadow a template name with a custom policy.
func isTemplateName(name string) bool {
	return strings.HasPrefix(name, "harbormaster-")
}

// OriginFor classifies a canned policy name by provenance: MinIO built-ins
// first, then the Harbormaster-template prefix, else custom.
func OriginFor(name string) string {
	if _, ok := builtinPolicies[name]; ok {
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
	_, ok := builtinPolicies[name]
	return ok
}
