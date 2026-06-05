package policies

import "encoding/json"

// Origin classifies a canned policy by provenance. Only custom policies are
// editable/deletable through Harbormaster.
const (
	OriginBuiltin  = "minio-builtin"
	OriginTemplate = "harbormaster-template"
	OriginCustom   = "custom"
)

// Policy is the immutable read view of a single canned policy. Editable is
// true exactly when Origin == OriginCustom. StatementSummary is a short,
// human-readable précis of the first statement (never the full document).
type Policy struct {
	Name             string
	Origin           string
	Editable         bool
	StatementSummary string
}

// PolicyDetail adds the full IAM document to a Policy (returned by Get only).
type PolicyDetail struct {
	Policy
	Document json.RawMessage
}
