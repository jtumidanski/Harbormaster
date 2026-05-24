package policies

import (
	"encoding/json"
	"fmt"
)

// Template represents a bundled Harbormaster IAM policy template that the
// REST layer can attach to a MinIO user without the operator having to author
// raw policy JSON. Each template has a stable Name, a human Description, an
// optional ParamsSchema describing required Render inputs, and a Render
// function that materialises the canonical AWS-IAM JSON document.
//
// All bundled templates avoid any "Effect: Deny" or admin-level actions —
// administrative MinIO privileges (`consoleAdmin`, `diagnostics`) must be
// granted outside Harbormaster so the UI never appears to escalate.
type Template struct {
	Name         string
	Description  string
	Render       func(params map[string]string) (string, error)
	ParamsSchema json.RawMessage
}

// All returns the bundled v1 templates in deterministic order. The order is
// surfaced verbatim to the SPA on GET /policy-templates so a stable wire
// shape is part of the contract.
func All() []Template {
	return []Template{readOnly(), readWrite(), backupTarget()}
}

// Find returns the named template or ok=false when no template with that
// name is registered. Administrative-style names (e.g. "administrator") are
// intentionally absent: the lookup misses so the REST layer can return a
// typed 422 "unknown_template" envelope rather than silently materialising a
// privilege the UI has no UX for revoking.
func Find(name string) (Template, bool) {
	for _, t := range All() {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

func readOnly() Template {
	return Template{
		Name:        "read-only",
		Description: "Read-only across all buckets",
		Render: func(_ map[string]string) (string, error) {
			return renderDoc(
				[]string{"s3:GetObject", "s3:ListBucket"},
				[]string{"arn:aws:s3:::*", "arn:aws:s3:::*/*"})
		},
	}
}

func readWrite() Template {
	return Template{
		Name:        "read-write",
		Description: "Read/write across all buckets, no admin operations",
		Render: func(_ map[string]string) (string, error) {
			return renderDoc(
				[]string{"s3:GetObject", "s3:ListBucket", "s3:PutObject", "s3:DeleteObject"},
				[]string{"arn:aws:s3:::*", "arn:aws:s3:::*/*"})
		},
	}
}

func backupTarget() Template {
	schema := json.RawMessage(`{"type":"object","required":["bucket"],"properties":{"bucket":{"type":"string","minLength":3,"maxLength":63}}}`)
	return Template{
		Name:         "backup-target",
		Description:  "Read/write/delete in a specific bucket",
		ParamsSchema: schema,
		Render: func(p map[string]string) (string, error) {
			b := p["bucket"]
			if b == "" {
				return "", fmt.Errorf("backup-target requires param 'bucket'")
			}
			return renderDoc(
				[]string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"},
				[]string{"arn:aws:s3:::" + b, "arn:aws:s3:::" + b + "/*"})
		},
	}
}

func renderDoc(actions, resources []string) (string, error) {
	type stmt struct {
		Effect   string   `json:"Effect"`
		Action   []string `json:"Action"`
		Resource []string `json:"Resource"`
	}
	type doc struct {
		Version   string `json:"Version"`
		Statement []stmt `json:"Statement"`
	}
	out, err := json.Marshal(doc{
		Version:   "2012-10-17",
		Statement: []stmt{{Effect: "Allow", Action: actions, Resource: resources}},
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MaterializedName returns the deterministic policy name Harbormaster uses
// on the MinIO side when attaching this template. Determinism is the
// invariant the materializer relies on for idempotent EnsurePolicy: the
// same (template, params) tuple always produces the same MinIO policy name
// so repeat calls collapse to a single named record.
//
// For parameterised templates the parameter values are included so two
// backup-target attachments scoped to different buckets do not collide on
// one canonical policy.
func MaterializedName(template string, params map[string]string) string {
	switch template {
	case "backup-target":
		return fmt.Sprintf("harbormaster-%s-%s", template, params["bucket"])
	default:
		return fmt.Sprintf("harbormaster-%s", template)
	}
}
