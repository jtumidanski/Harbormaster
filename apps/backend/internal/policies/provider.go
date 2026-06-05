package policies

import (
	"encoding/json"
	"fmt"
)

// summaryDoc is the minimal shape parsed for statement summarisation.
// Action and Resource accept both a plain string and a []string via the
// stringOrSlice helper type.
type summaryDoc struct {
	Statement []summaryStmt `json:"Statement"`
}

type summaryStmt struct {
	Effect   string        `json:"Effect"`
	Action   stringOrSlice `json:"Action"`
	Resource stringOrSlice `json:"Resource"`
}

// stringOrSlice unmarshals a JSON value that is either a bare string or an
// array of strings into a flat []string. MinIO policy documents use both
// forms interchangeably (e.g. "s3:GetObject" vs ["s3:GetObject","s3:PutObject"]).
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// Try array first.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// Try bare string.
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	if str != "" {
		*s = []string{str}
	}
	return nil
}

// first returns the first element, or "" when the slice is empty.
func (s stringOrSlice) first() string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// statementSummary derives a human-readable one-liner from the first
// statement in a policy document. It returns:
//   - "" when the document is unparseable or has no statements.
//   - "<Effect> (no action) [on <resource>]" when the first statement carries no action.
//   - "<Effect> <action> [on <resource>] [+N more]" otherwise.
//
// "on <resource>" is omitted when the Resource field is empty/missing.
// "(+N more)" is appended when the document has more than one statement.
func statementSummary(doc json.RawMessage) string {
	var d summaryDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return ""
	}
	if len(d.Statement) == 0 {
		return ""
	}
	first := d.Statement[0]
	action := first.Action.first()
	resource := first.Resource.first()

	var summary string
	if action == "" {
		summary = fmt.Sprintf("%s (no action)", first.Effect)
	} else {
		summary = fmt.Sprintf("%s %s", first.Effect, action)
	}

	if resource != "" {
		summary = fmt.Sprintf("%s on %s", summary, resource)
	}

	extra := len(d.Statement) - 1
	if extra > 0 {
		summary = fmt.Sprintf("%s (+%d more)", summary, extra)
	}

	return summary
}

// policyFromEntry builds a Policy from a raw ListCannedPolicies entry.
// It classifies the name via OriginFor/EditableFor and computes the
// StatementSummary from the raw document.
func policyFromEntry(name string, doc json.RawMessage) Policy {
	return Policy{
		Name:             name,
		Origin:           OriginFor(name),
		Editable:         EditableFor(name),
		StatementSummary: statementSummary(doc),
	}
}
