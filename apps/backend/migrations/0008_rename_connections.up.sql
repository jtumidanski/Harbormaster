-- 2026-09 rename sweep: the connection row describes any S3 object store
-- (RustFS in production, MinIO in tests), so the table drops the vendor name.
-- SQLite keeps indexes across RENAME; recreate the singleton index under the
-- new name so the schema reads consistently.
ALTER TABLE minio_connections RENAME TO connections;
DROP INDEX IF EXISTS minio_connections_singleton;
CREATE UNIQUE INDEX connections_singleton ON connections(singleton_guard);
