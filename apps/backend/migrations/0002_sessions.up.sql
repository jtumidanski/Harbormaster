CREATE TABLE sessions (
  id              TEXT PRIMARY KEY,
  admin_user_id   INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL,
  expires_at      TEXT NOT NULL,
  last_active_at  TEXT NOT NULL,
  source_ip       TEXT,
  user_agent      TEXT
);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
CREATE INDEX sessions_admin_user_id_idx ON sessions(admin_user_id);
