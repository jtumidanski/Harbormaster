CREATE TABLE audit_events (
  id                   TEXT PRIMARY KEY,
  occurred_at          TEXT NOT NULL,
  actor                TEXT NOT NULL,
  source_ip            TEXT,
  action               TEXT NOT NULL,
  target_type          TEXT NOT NULL,
  target_id            TEXT,
  outcome              TEXT NOT NULL,
  error_message        TEXT,
  payload_summary_json TEXT
);
CREATE INDEX audit_events_occurred_at_idx ON audit_events(occurred_at);
CREATE INDEX audit_events_action_idx      ON audit_events(action, occurred_at);
CREATE INDEX audit_events_target_idx      ON audit_events(target_type, target_id);
