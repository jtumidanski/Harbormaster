package objects

import (
	"context"

	miniogo "github.com/minio/minio-go/v7"
)

const (
	// bulkDeleteCeiling is the maximum exact object count the dry-run
	// preview reports. Once the running count reaches this, counting stops
	// and Truncated is set so the UI can render "10,000+". The delete path
	// is NOT capped by this value.
	bulkDeleteCeiling = 10000

	// bulkRemoveBatchN is the max number of keys handed to a single
	// RemoveObjects call. Mirrors MinIO's server-side max-keys cap of 1000.
	bulkRemoveBatchN = 1000
)

// countExpansion streams a recursive listing of each prefix and returns
// the number of objects under them plus len(keys), capped at
// bulkDeleteCeiling. De-duplication of overlapping keys/prefixes is not
// performed (best-effort per the PRD); the count may slightly over-report
// on overlap.
//
// A cancelable context is derived from ctx and cancelled on every return
// path (including the early break at the ceiling) so the minio-go producer
// goroutine never blocks forever on a channel send.
func countExpansion(ctx context.Context, s3 s3API, bucket string, keys, prefixes []string) (int, bool, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	count := len(keys)
	if count >= bulkDeleteCeiling {
		return bulkDeleteCeiling, true, nil
	}
	for _, prefix := range prefixes {
		ch := s3.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{Prefix: prefix, Recursive: true})
		for obj := range ch {
			if obj.Err != nil {
				return 0, false, obj.Err
			}
			count++
			if count >= bulkDeleteCeiling {
				return bulkDeleteCeiling, true, nil
			}
		}
	}
	return count, false, nil
}

// deleteExpansion streams the explicit keys plus every key under each
// prefix into RemoveObjects in batches of <= bulkRemoveBatchN, collecting
// per-key failures without aborting. Deletes carry no version ID
// (RemoveObjectsOptions{} + bare ObjectInfo{Key}), matching the
// single-object Delete semantics: a delete marker on a versioned bucket,
// a permanent removal on an unversioned bucket.
//
// A listing error aborts the whole operation and is returned. The producer
// goroutine is torn down via a cancelable context on every return path.
// DeletedCount = (keys submitted) - (per-key failures).
func deleteExpansion(ctx context.Context, s3 s3API, bucket string, keys, prefixes []string) (int, []BulkDeleteFailure, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var failures []BulkDeleteFailure
	submitted := 0
	batch := make([]miniogo.ObjectInfo, 0, bulkRemoveBatchN)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		objCh := make(chan miniogo.ObjectInfo, len(batch))
		for _, o := range batch {
			objCh <- o
		}
		close(objCh)
		errCh := s3.RemoveObjects(cctx, bucket, objCh, miniogo.RemoveObjectsOptions{})
		for e := range errCh {
			if e.Err != nil {
				failures = append(failures, BulkDeleteFailure{Key: e.ObjectName, Error: e.Err.Error()})
			}
		}
		submitted += len(batch)
		batch = batch[:0]
	}

	add := func(key string) {
		batch = append(batch, miniogo.ObjectInfo{Key: key})
		if len(batch) >= bulkRemoveBatchN {
			flush()
		}
	}

	for _, k := range keys {
		add(k)
	}
	for _, prefix := range prefixes {
		ch := s3.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{Prefix: prefix, Recursive: true})
		for obj := range ch {
			if obj.Err != nil {
				return 0, nil, obj.Err
			}
			add(obj.Key)
		}
	}
	flush()

	return submitted - len(failures), failures, nil
}
