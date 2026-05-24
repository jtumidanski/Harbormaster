package dashboard

import "github.com/jtumidanski/Harbormaster/internal/audit"

// MarshalJSON tags on audit.Event are unexported (it's a domain struct,
// not a wire DTO), so the dashboard payload needs its own
// EventSummary / FailureSummary projection types when M5 frontend
// integration starts requiring stable field names beyond what
// audit.Event currently exposes.
//
// For T5.1 the contract example fields (id / occurred_at / action /
// target_type / target_id / outcome / source_ip / error_message) are
// all present on audit.Event by name; encoding/json's default snake_case
// conversion would NOT match the contract, so we add explicit JSON
// tags here via the projection helpers below. Build returns these via
// the View.RecentActivity and View.RecentFailures fields when the HTTP
// layer is wired in T5.3 — until then keeping these projections as
// thin helpers means future contract drift can be re-pointed without
// touching the processor signature.

// EventSummary is the recent_activity row shape from api-contracts.md.
// We project audit.Event into this struct rather than tagging audit.Event
// directly so the domain model stays transport-agnostic.
type EventSummary struct {
	ID         string `json:"id"`
	OccurredAt string `json:"occurred_at"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
	Outcome    string `json:"outcome"`
}

// FailureSummary is the recent_failures.entries row shape. It widens
// EventSummary with source_ip and error_message — both documented as
// surfaced on the failures widget (api-contracts.md §dashboard).
type FailureSummary struct {
	ID           string `json:"id"`
	OccurredAt   string `json:"occurred_at"`
	Action       string `json:"action"`
	TargetType   string `json:"target_type"`
	TargetID     string `json:"target_id,omitempty"`
	SourceIP     string `json:"source_ip,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// summariseEvent projects a single audit.Event to the contract's
// recent_activity row shape.
func summariseEvent(e audit.Event) EventSummary {
	return EventSummary{
		ID:         e.ID,
		OccurredAt: e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Outcome:    e.Outcome,
	}
}

// summariseFailure projects a single failure-outcome audit.Event to the
// contract's recent_failures.entries row shape.
func summariseFailure(e audit.Event) FailureSummary {
	return FailureSummary{
		ID:           e.ID,
		OccurredAt:   e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		Action:       e.Action,
		TargetType:   e.TargetType,
		TargetID:     e.TargetID,
		SourceIP:     e.SourceIP,
		ErrorMessage: e.ErrorMessage,
	}
}
