package audit

import "time"

// AuditEventAttrs is the JSON:API attributes shape for an audit event.
// source_ip is deliberately omitted from PayloadSummary — it is stored in
// the database for operator forensics but never re-emitted via the API.
// Full marshaling and HTTP handler are wired in M5.
type AuditEventAttrs struct {
	OccurredAt     time.Time      `json:"occurred_at"`
	Actor          string         `json:"actor"`
	Action         string         `json:"action"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id,omitempty"`
	Outcome        string         `json:"outcome"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	PayloadSummary map[string]any `json:"payload_summary,omitempty"`
	// source_ip is intentionally absent — it MUST NOT appear in API responses.
}

// ToAttrs converts an Event to its JSON:API attributes representation.
// PayloadSummary in the response is the sanitised form stored in the DB;
// source_ip is never included.
func ToAttrs(e Event) AuditEventAttrs {
	return AuditEventAttrs{
		OccurredAt:     e.OccurredAt,
		Actor:          e.Actor,
		Action:         e.Action,
		TargetType:     e.TargetType,
		TargetID:       e.TargetID,
		Outcome:        e.Outcome,
		ErrorMessage:   e.ErrorMessage,
		PayloadSummary: e.PayloadSummary,
	}
}
