package audit_test

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// presignedURLRE matches a string that starts with an http(s) scheme AND
// carries the AWS SigV4 X-Amz-Signature query parameter — the exact shape
// of a presigned object URL. Plain endpoint URLs (no signature) do not
// match and are permitted in audit payloads (e.g. connection.update
// records the connection's endpoint URL).
//
// The invariant this regex enforces is R17 (PRD §13): "Audit payloads
// MUST NOT contain presigned URLs."
var presignedURLRE = regexp.MustCompile(`^https?://.*X-Amz-Signature=`)

// TestNoPresignedURLInAuditPayloads enumerates every audit action
// constant, records an event whose payload deliberately contains a
// well-formed presigned-URL string in several positions (top-level url,
// nested url, repurposed-key url), then walks every persisted
// payload_summary_json value and asserts no string leaf matches the
// presigned-URL pattern.
//
// This is the structural backstop for R17: even if a future caller
// invents a new payload key (e.g. "download_link") and forgets that
// Sanitize only drops keys whose names match the sensitive regex, the
// stored value must still not look like a presigned URL.
func TestNoPresignedURLInAuditPayloads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const presigned = "https://example.com/foo?X-Amz-Signature=deadbeef&X-Amz-Algorithm=AWS4-HMAC-SHA256"
	const plainEndpoint = "https://minio.example.com" // permitted: no signature

	for _, a := range audit.AllActions() {
		a := a // capture
		t.Run(a, func(t *testing.T) {
			t.Parallel()
			p := newTestProcessor(t)

			err := p.Record(ctx, audit.Event{
				OccurredAt: time.Now().UTC(),
				Actor:      "local-admin",
				Action:     a,
				TargetType: "bucket",
				TargetID:   "x",
				Outcome:    audit.OutcomeSuccess,
				PayloadSummary: map[string]any{
					"url":           presigned, // sanitizer drops this key
					"presigned_url": presigned, // sanitizer drops this key
					"endpoint":      plainEndpoint,
					"nested": map[string]any{
						"url":     presigned,
						"comment": "nested-safe",
					},
					"safe_field": "ok",
				},
			})
			require.NoError(t, err)

			raw := loadLatest(t, p, a)
			require.NotEmpty(t, raw, "no payload persisted for action %s", a)

			// JSON-walk every string leaf and assert none match the
			// presigned-URL pattern. The walker reports the path of any
			// offending leaf so a regression is easy to triage.
			var parsed any
			require.NoError(t, json.Unmarshal([]byte(raw), &parsed),
				"payload_summary_json for action %s is not valid JSON: %s", a, raw)
			walkStrings(t, a, "", parsed)
		})
	}
}

// walkStrings recursively visits every string leaf in v and fails the
// test if any matches presignedURLRE. path is the JSON-pointer-ish
// breadcrumb used in the failure message.
func walkStrings(t *testing.T, action, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case string:
		if presignedURLRE.MatchString(x) {
			t.Errorf("action %s: presigned URL leaked at %s = %q (R17 violation)",
				action, path, x)
		}
	case map[string]any:
		for k, child := range x {
			walkStrings(t, action, path+"/"+k, child)
		}
	case []any:
		for i, child := range x {
			walkStrings(t, action, path+"/"+itoa(i), child)
		}
	}
}

// itoa avoids pulling in strconv for the single-use call.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
