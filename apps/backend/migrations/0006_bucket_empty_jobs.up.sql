CREATE TABLE bucket_empty_jobs (
  id                TEXT PRIMARY KEY,
  bucket_name       TEXT NOT NULL,
  started_at        TEXT NOT NULL,
  last_progress_at  TEXT NOT NULL,
  deleted_count     INTEGER NOT NULL DEFAULT 0,
  estimated_total   INTEGER,
  state             TEXT NOT NULL,
  error_message     TEXT,
  finished_at       TEXT,
  purge_versions    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX bucket_empty_jobs_active_per_bucket
  ON bucket_empty_jobs(bucket_name)
  WHERE state = 'running';
CREATE INDEX bucket_empty_jobs_bucket_idx ON bucket_empty_jobs(bucket_name, started_at);
