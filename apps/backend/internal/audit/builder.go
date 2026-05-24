package audit

import "time"

// Builder is a fluent helper for constructing an Event before passing it to
// Processor.Record. This minimal implementation satisfies M1 compilation;
// richer construction helpers will be added when M5 wires up HTTP handlers.
type Builder struct {
	e Event
}

// NewEvent starts a new Builder with the given action.
func NewEvent(action string) *Builder {
	return &Builder{e: Event{Action: action}}
}

// Actor sets the actor field (e.g. "local-admin").
func (b *Builder) Actor(actor string) *Builder {
	b.e.Actor = actor
	return b
}

// SourceIP sets the source IP.
func (b *Builder) SourceIP(ip string) *Builder {
	b.e.SourceIP = ip
	return b
}

// Target sets target_type and target_id.
func (b *Builder) Target(targetType, targetID string) *Builder {
	b.e.TargetType = targetType
	b.e.TargetID = targetID
	return b
}

// Outcome sets the outcome (OutcomeSuccess or OutcomeFailure).
func (b *Builder) Outcome(outcome string) *Builder {
	b.e.Outcome = outcome
	return b
}

// Error attaches an error message and sets the outcome to OutcomeFailure.
func (b *Builder) Error(msg string) *Builder {
	b.e.ErrorMessage = msg
	b.e.Outcome = OutcomeFailure
	return b
}

// Payload sets the payload summary map (will be sanitised by Processor.Record).
func (b *Builder) Payload(m map[string]any) *Builder {
	b.e.PayloadSummary = m
	return b
}

// OccurredAt overrides the timestamp (defaults to time.Now() in Record).
func (b *Builder) OccurredAt(t time.Time) *Builder {
	b.e.OccurredAt = t
	return b
}

// Build returns the constructed Event.
func (b *Builder) Build() Event {
	return b.e
}
