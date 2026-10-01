-- 2026-10-01 rename sweep: stored series names drop the vendor prefix so
-- they match the collector's objectstore_* keys.
UPDATE metrics_samples SET metric = 'objectstore_' || substr(metric, 7) WHERE metric LIKE 'minio\_%' ESCAPE '\';
