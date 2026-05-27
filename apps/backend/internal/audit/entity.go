package audit

import (
	"encoding/json"
	"time"
)

// auditEvent is the GORM persistence struct for the audit_events table.
// It is unexported; only the package itself may construct or read it.
type auditEvent struct {
	ID                 string `gorm:"column:id;primaryKey"`
	OccurredAt         string `gorm:"column:occurred_at;not null"`
	Actor              string `gorm:"column:actor;not null"`
	SourceIP           string `gorm:"column:source_ip"`
	Action             string `gorm:"column:action;not null"`
	TargetType         string `gorm:"column:target_type;not null"`
	TargetID           string `gorm:"column:target_id"`
	Outcome            string `gorm:"column:outcome;not null"`
	ErrorMessage       string `gorm:"column:error_message"`
	PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
}

// TableName satisfies gorm.Tabler.
func (auditEvent) TableName() string { return "audit_events" }

// Make converts a domain Event into a persistence entity.
// PayloadSummary is serialised to JSON; a nil map produces an empty string.
// The caller must have already sanitised the payload before calling Make.
func Make(e Event) auditEvent {
	var pJSON string
	if e.PayloadSummary != nil {
		b, _ := json.Marshal(e.PayloadSummary)
		pJSON = string(b)
	}
	return auditEvent{
		ID:                 e.ID,
		OccurredAt:         e.OccurredAt.UTC().Format(time.RFC3339Nano),
		Actor:              e.Actor,
		SourceIP:           e.SourceIP,
		Action:             e.Action,
		TargetType:         e.TargetType,
		TargetID:           e.TargetID,
		Outcome:            e.Outcome,
		ErrorMessage:       e.ErrorMessage,
		PayloadSummaryJSON: pJSON,
	}
}

// ToEvent converts the persistence entity back into a domain Event.
func (ae auditEvent) ToEvent() Event {
	t, _ := time.Parse(time.RFC3339Nano, ae.OccurredAt)
	var payload map[string]any
	if ae.PayloadSummaryJSON != "" {
		_ = json.Unmarshal([]byte(ae.PayloadSummaryJSON), &payload)
	}
	return Event{
		ID:             ae.ID,
		OccurredAt:     t.UTC(),
		Actor:          ae.Actor,
		SourceIP:       ae.SourceIP,
		Action:         ae.Action,
		TargetType:     ae.TargetType,
		TargetID:       ae.TargetID,
		Outcome:        ae.Outcome,
		ErrorMessage:   ae.ErrorMessage,
		PayloadSummary: payload,
	}
}
