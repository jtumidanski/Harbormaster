CREATE TABLE metrics_samples (
  id           TEXT PRIMARY KEY,
  collected_at TEXT NOT NULL,
  metric       TEXT NOT NULL,
  value        REAL NOT NULL
);
CREATE INDEX metrics_samples_metric_time_idx ON metrics_samples(metric, collected_at);
CREATE INDEX metrics_samples_collected_at_idx ON metrics_samples(collected_at);
