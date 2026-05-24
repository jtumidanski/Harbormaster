package audit

// AuditEventResource wraps an Event for JSON:API transport.
// Router wiring and full attribute marshaling are implemented in M5.
// M1 just exposes the type and implements the jsonapi.Resource interface.
type AuditEventResource struct {
	Event Event
}

// ResourceType returns the JSON:API type name.
func (r AuditEventResource) ResourceType() string { return "audit-events" }

// ResourceID returns the event's ULID.
func (r AuditEventResource) ResourceID() string { return r.Event.ID }
