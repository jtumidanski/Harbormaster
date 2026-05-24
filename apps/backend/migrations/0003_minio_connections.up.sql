CREATE TABLE minio_connections (
  id                          INTEGER PRIMARY KEY AUTOINCREMENT,
  singleton_guard             INTEGER NOT NULL DEFAULT 1,
  endpoint_url                TEXT NOT NULL,
  tls_skip_verify             INTEGER NOT NULL DEFAULT 0,
  access_key_ciphertext       TEXT NOT NULL,
  secret_key_ciphertext       TEXT NOT NULL,
  custom_ca_pem_ciphertext    TEXT,
  created_at                  TEXT NOT NULL,
  updated_at                  TEXT NOT NULL
);
CREATE UNIQUE INDEX minio_connections_singleton ON minio_connections(singleton_guard);
