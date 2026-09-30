DROP INDEX IF EXISTS connections_singleton;
ALTER TABLE connections RENAME TO minio_connections;
CREATE UNIQUE INDEX minio_connections_singleton ON minio_connections(singleton_guard);
