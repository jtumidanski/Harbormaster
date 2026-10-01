UPDATE metrics_samples SET metric = 'minio_' || substr(metric, 13) WHERE metric LIKE 'objectstore\_%' ESCAPE '\';
