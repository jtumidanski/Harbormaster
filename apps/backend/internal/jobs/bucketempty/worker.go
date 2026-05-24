package bucketempty

import (
	"context"
	"fmt"

	miniogo "github.com/minio/minio-go/v7"
)

// s3Iface is the subset of *miniogo.Client the worker uses. Production code
// passes the real client; tests substitute a fake. Defined here so unit tests
// in this package can construct a stub without a running MinIO.
type s3Iface interface {
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
	RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
	GetBucketVersioning(ctx context.Context, bucket string) (miniogo.BucketVersioningConfiguration, error)
}

// drainObjects iterates ListObjects and bulk-deletes in batches of batchN.
// onBatch is invoked with the number of keys submitted to RemoveObjects on
// each batch (NOT the number of confirmed successes, since RemoveObjects
// signals success by silence on the error channel).
//
// Exposed as a package-level variable so tests can substitute a deterministic
// stub without standing up a real S3 endpoint.
var drainObjects = func(ctx context.Context, mc s3Iface, bucket string, batchN int, onBatch func(int64)) error {
	listOpts := miniogo.ListObjectsOptions{Recursive: true}
	return drainWithOpts(ctx, mc, bucket, batchN, listOpts, miniogo.RemoveObjectsOptions{}, onBatch)
}

// drainVersions enumerates every version (including delete markers) and bulk-
// removes them. The VersionID on each ObjectInfo is preserved through the
// channel hand-off so RemoveObjects targets the correct version.
var drainVersions = func(ctx context.Context, mc s3Iface, bucket string, batchN int, onBatch func(int64)) error {
	listOpts := miniogo.ListObjectsOptions{Recursive: true, WithVersions: true}
	return drainWithOpts(ctx, mc, bucket, batchN, listOpts, miniogo.RemoveObjectsOptions{GovernanceBypass: false}, onBatch)
}

// drainWithOpts is the shared batching loop. It reads up to batchN items from
// the list channel, hands them to RemoveObjects via a buffered channel, drains
// the RemoveObjects error channel, and returns the first error encountered.
//
// The current batch is fed via a fresh chan ObjectInfo per RemoveObjects call
// because RemoveObjects loops until its objectsCh is closed; passing the raw
// list channel would never terminate.
func drainWithOpts(
	ctx context.Context,
	mc s3Iface,
	bucket string,
	batchN int,
	listOpts miniogo.ListObjectsOptions,
	rmOpts miniogo.RemoveObjectsOptions,
	onBatch func(int64),
) error {
	if batchN <= 0 {
		batchN = 1000
	}
	listCh := mc.ListObjects(ctx, bucket, listOpts)
	batch := make([]miniogo.ObjectInfo, 0, batchN)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		objCh := make(chan miniogo.ObjectInfo, len(batch))
		for _, o := range batch {
			objCh <- o
		}
		close(objCh)
		errCh := mc.RemoveObjects(ctx, bucket, objCh, rmOpts)
		var firstErr error
		for e := range errCh {
			if e.Err != nil && firstErr == nil {
				firstErr = fmt.Errorf("remove %q version=%q: %w", e.ObjectName, e.VersionID, e.Err)
			}
		}
		if firstErr != nil {
			return firstErr
		}
		onBatch(int64(len(batch)))
		batch = batch[:0]
		return nil
	}

	for obj := range listCh {
		if obj.Err != nil {
			return fmt.Errorf("list objects: %w", obj.Err)
		}
		batch = append(batch, obj)
		if len(batch) >= batchN {
			if err := flush(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return flush()
}

// isVersioned returns true when the bucket's versioning status is "Enabled".
// A Suspended bucket is treated as not-versioned for purge purposes; only
// genuinely Enabled buckets need the version-aware drain path.
func isVersioned(ctx context.Context, mc s3Iface, bucket string) (bool, error) {
	cfg, err := mc.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return false, fmt.Errorf("get bucket versioning: %w", err)
	}
	return cfg.Status == miniogo.Enabled, nil
}
