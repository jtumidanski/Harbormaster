package main

import (
	"context"

	madmin "github.com/minio/madmin-go/v3"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

// bucketEmptyAuditAdapter translates the bucketempty.AuditRecorder shape
// (action/target/outcome/payload/err positional args, error-less return)
// into the audit.Processor.Record(ctx, audit.Event) call. Boot wiring uses
// it both for OrphanRunningAtStartup and as the Service's audit hook.
//
// Failures to persist the event are deliberately swallowed: the worker has
// no recovery path, and the surrounding context (process boot or a
// completed empty job) has already moved on by the time the row would land.
type bucketEmptyAuditAdapter struct {
	p *audit.Processor
}

// Record implements bucketempty.AuditRecorder. The bucket name maps to the
// audit Event's TargetID; the static "bucket" string is the TargetType so
// the audit log groups orphan/terminal entries with the rest of the bucket
// domain's events.
func (a bucketEmptyAuditAdapter) Record(ctx context.Context, action, target, outcome string,
	payload map[string]any, errMsg string,
) {
	if a.p == nil {
		return
	}
	_ = a.p.Record(ctx, audit.Event{
		Action:         action,
		TargetType:     "bucket",
		TargetID:       target,
		Outcome:        outcome,
		ErrorMessage:   errMsg,
		PayloadSummary: payload,
	})
}

// bucketAdminAdapter wraps a live *madmin.AdminClient and supplies the
// BucketUsageInfo method the buckets package's adminAPI interface requires
// but the upstream SDK does not expose directly.
//
// MinIO's per-bucket usage row lives inside the DataUsageInfo blob, which
// the scanner refreshes on its own cadence. We re-issue the (cheap) admin
// RPC on every call rather than caching: per-bucket detail fetches fan out
// under an errgroup capped at 10, so the upper bound on concurrent calls
// is tiny and a one-process-wide cache would add invalidation complexity
// for negligible benefit on a homelab-scale cluster.
type bucketAdminAdapter struct {
	*madmin.AdminClient
}

// BucketUsageInfo returns the usage row for bucket. A missing bucket
// surfaces as the zero value plus nil error so the processor's tolerant
// usage-fetch path treats it as "scanner has not seen this bucket yet".
func (a bucketAdminAdapter) BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error) {
	info, err := a.AdminClient.DataUsageInfo(ctx)
	if err != nil {
		return madmin.BucketUsageInfo{}, err
	}
	return info.BucketsUsage[bucket], nil
}

// newBucketClientGetter returns a buckets.ClientGetter bound to the live
// MinIO pool. Each call resolves the current client pair (Get is O(1)
// under the pool's RWMutex), wraps the admin client in the BucketUsageInfo
// adapter, and hands the pair to buckets.NewClientGetter which adapts the
// public AdminClient / S3Client interfaces onto the unexported pair the
// processor consumes.
func newBucketClientGetter(pool *hmminio.Pool) buckets.ClientGetter {
	return buckets.NewClientGetter(func(ctx context.Context) (buckets.AdminClient, buckets.S3Client, error) {
		madm, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, nil, err
		}
		return bucketAdminAdapter{AdminClient: madm}, mc, nil
	})
}

// _ assertion keeps the audit adapter type-correct against the
// bucketempty contract; a future signature drift fails the build here
// instead of at runtime.
var _ bucketempty.AuditRecorder = bucketEmptyAuditAdapter{}
