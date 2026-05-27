package audit

import "time"

// EventAttrs is the JSON:API attributes shape for an audit event.
//
// Per docs/tasks/task-001-harbormaster-mvp-v1/api-contracts.md
// §GET /api/v1/audit-events, source_ip IS exposed on the M5 list
// endpoint so operators can correlate failures with the originating
// client (the same field is omitted from PayloadSummary by Sanitize so
// payload bodies cannot duplicate the value through a renamed key).
type EventAttrs struct {
	OccurredAt     time.Time      `json:"occurred_at"`
	Actor          string         `json:"actor"`
	SourceIP       string         `json:"source_ip,omitempty"`
	Action         string         `json:"action"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id,omitempty"`
	Outcome        string         `json:"outcome"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	PayloadSummary map[string]any `json:"payload_summary,omitempty"`
}

// ToAttrs converts an Event to its JSON:API attributes representation.
// PayloadSummary in the response is the sanitised form stored in the DB.
func ToAttrs(e Event) EventAttrs {
	return EventAttrs{
		OccurredAt:     e.OccurredAt,
		Actor:          e.Actor,
		SourceIP:       e.SourceIP,
		Action:         e.Action,
		TargetType:     e.TargetType,
		TargetID:       e.TargetID,
		Outcome:        e.Outcome,
		ErrorMessage:   e.ErrorMessage,
		PayloadSummary: e.PayloadSummary,
	}
}
