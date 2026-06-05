package main

import (
	"context"
	"io"
	"strings"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/dashboard"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/lifecycle"
	"github.com/jtumidanski/Harbormaster/internal/metrics"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
	"github.com/jtumidanski/Harbormaster/internal/objects"
	"github.com/jtumidanski/Harbormaster/internal/policies"
	"github.com/jtumidanski/Harbormaster/internal/users"
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
	info, err := a.DataUsageInfo(ctx)
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

// objectS3Adapter wraps a *miniogo.Client so it satisfies the
// objects.S3Client interface. ListObjectsV2 lives on miniogo.Core (which
// embeds *Client), so we synthesise a Core value on demand and route the
// call through it; every other method delegates to the embedded Client.
//
// A fresh Core literal per call is cheap — it's just a struct holding a
// pointer — and avoids needing the Pool to retain a separate Core handle.
type objectS3Adapter struct {
	*miniogo.Client
}

// ListObjectsV2 routes through miniogo.Core because the high-level
// Client.ListObjects API returns a channel and hides the continuation
// token the paginated object-list UI needs.
func (a objectS3Adapter) ListObjectsV2(bucket, prefix, startAfter, continuationToken, delimiter string, maxKeys int) (miniogo.ListBucketV2Result, error) {
	core := miniogo.Core{Client: a.Client}
	return core.ListObjectsV2(bucket, prefix, startAfter, continuationToken, delimiter, maxKeys)
}

// GetObject narrows miniogo.Client.GetObject's *miniogo.Object return
// to the io.ReadCloser the objects.S3Client interface expects.
// *miniogo.Object already satisfies io.ReadCloser; we just type-erase
// it here so the package surface doesn't need to reach into miniogo.
func (a objectS3Adapter) GetObject(ctx context.Context, bucket, object string, opts miniogo.GetObjectOptions) (io.ReadCloser, error) {
	return a.Client.GetObject(ctx, bucket, object, opts)
}

// ListObjectVersions drains the high-level Client.ListObjects channel
// (WithVersions=true) into a slice, capping at maxScan to bound
// pathological keys. The bool return is "truncated" — true when the scan
// hit maxScan before the channel closed.
//
// A cancelable context is derived from the caller's ctx and cancelled via
// defer so the minio-go producer goroutine is torn down on every return
// path — including early truncation and error returns — preventing the
// goroutine from blocking on a channel send forever.
func (a objectS3Adapter) ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := a.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{
		Prefix:       key,
		WithVersions: true,
	})
	out := make([]miniogo.ObjectInfo, 0, 16)
	truncated := false
	for info := range ch {
		if info.Err != nil {
			return nil, false, info.Err
		}
		if len(out) >= maxScan {
			truncated = true
			break
		}
		out = append(out, info)
	}
	return out, truncated, nil
}

// newObjectClientGetter returns an objects.ClientGetter bound to the live
// MinIO pool. Each call resolves the current client, wraps it in
// objectS3Adapter so ListObjectsV2 routes through miniogo.Core, and hands
// the wrapper to objects.NewClientGetter which adapts the exported
// S3Client interface onto the unexported s3API the processor consumes.
func newObjectClientGetter(pool *hmminio.Pool) objects.ClientGetter {
	return objects.NewClientGetter(func(ctx context.Context) (objects.S3Client, error) {
		_, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return objectS3Adapter{Client: mc}, nil
	})
}

// lifecycleS3Adapter wraps a *miniogo.Client so it satisfies the
// lifecycle.S3Client interface. The live client already exposes the
// Get/Set/PutBucketLifecycle methods the lifecycle processor needs, so
// this is a thin pass-through that pins the integration point at the
// HTTP wiring layer (the lifecycle package never imports the pool type).
type lifecycleS3Adapter struct {
	*miniogo.Client
}

// newLifecycleClientGetter returns a lifecycle.ClientGetter bound to the
// live MinIO pool. Each call resolves the current client and hands it to
// lifecycle.NewClientGetter which adapts the exported S3Client interface
// onto the unexported s3API used inside the package.
func newLifecycleClientGetter(pool *hmminio.Pool) lifecycle.ClientGetter {
	return lifecycle.NewClientGetter(func(ctx context.Context) (lifecycle.S3Client, error) {
		_, mc, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return lifecycleS3Adapter{Client: mc}, nil
	})
}

// bucketLifecycleAdapter satisfies buckets.LifecycleCreator by wrapping
// the live lifecycle.Processor. The bucket processor only needs success
// vs. failure (the audit row is built inside the lifecycle processor
// itself), so the *lifecycle.Rule return value is intentionally
// discarded. Actor / sourceIP for the auto-applied template are left
// empty here — the bucket.create audit row already carries the
// operator's identity, and the auto-applied lifecycle_rule.create row is
// a side-effect attributed to the same logical action.
type bucketLifecycleAdapter struct {
	lc *lifecycle.Processor
}

// Create implements buckets.LifecycleCreator.
func (a bucketLifecycleAdapter) Create(ctx context.Context, bucket string, days int, prefix string) error {
	if a.lc == nil {
		return nil
	}
	_, err := a.lc.Create(ctx, bucket, days, prefix, "", "")
	return err
}

// newUsersClientGetter returns a users.ClientGetter bound to the live
// MinIO pool. The live *madmin.AdminClient satisfies users.AdminClient by
// structural typing, so no per-method adapter is needed.
func newUsersClientGetter(pool *hmminio.Pool) users.ClientGetter {
	return users.NewClientGetter(func(ctx context.Context) (users.AdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}

// newSAClientGetter returns a users.SAClientGetter bound to the live
// MinIO pool. As above, the live *madmin.AdminClient satisfies
// users.SAAdminClient directly.
func newSAClientGetter(pool *hmminio.Pool) users.SAClientGetter {
	return users.NewSAClientGetter(func(ctx context.Context) (users.SAAdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}

// newPoliciesClientGetter returns a policies.ClientGetter bound to the live
// MinIO pool. The live *madmin.AdminClient satisfies policies.AdminClient by
// structural typing, so no per-method adapter is needed.
func newPoliciesClientGetter(pool *hmminio.Pool) policies.ClientGetter {
	return policies.NewClientGetter(func(ctx context.Context) (policies.AdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}

// Compile-time anchors keeping the adapter types in sync with their
// destination interfaces; a future signature drift fails the build here.
var (
	_ objects.S3Client         = objectS3Adapter{}
	_ lifecycle.S3Client       = lifecycleS3Adapter{}
	_ buckets.LifecycleCreator = bucketLifecycleAdapter{}
	_ dashboard.PoolGetter     = dashboardPoolAdapter{}
	_ policies.AdminClient     = (*madmin.AdminClient)(nil)
)

// dashboardPoolAdapter satisfies dashboard.PoolGetter by translating the
// live MinIO pool's madmin.ServerInfo response into the small dashboard
// view types. It owns the policy decisions for what counts as a
// "warning" so the dashboard processor stays storage-agnostic.
type dashboardPoolAdapter struct {
	pool *hmminio.Pool
}

// ServerInfo issues a single madmin.ServerInfo RPC, then translates the
// InfoMessage into the dashboard ServerInfo / NodeStatus / warnings
// triple. The wire field names (Mode, Version, Uptime, Servers[]) are
// from madmin v3 — see info-commands.go in the upstream module.
//
// Warning policy: any per-server state that is not exactly "online" is
// surfaced as a warning string so the dashboard widget can render the
// "M of N nodes online" headline without re-walking the nodes slice. The
// failures widget (a separate fan-out) covers operator-action failures;
// node warnings are purely about MinIO's own self-reported posture.
func (a dashboardPoolAdapter) ServerInfo(ctx context.Context) (dashboard.ServerInfo, []dashboard.NodeStatus, []string, error) {
	madm, _, err := a.pool.Get(ctx)
	if err != nil {
		return dashboard.ServerInfo{}, nil, nil, err
	}
	info, err := madm.ServerInfo(ctx)
	if err != nil {
		return dashboard.ServerInfo{}, nil, nil, err
	}

	// MinIO returns one Version per server; the cluster-level field is
	// the first server's value (homelab clusters are typically single
	// version anyway). Uptime is the highest-uptime server so a recent
	// node-restart doesn't make the cluster look fresh.
	var version string
	var uptime int64
	for _, srv := range info.Servers {
		if version == "" {
			version = srv.Version
		}
		if srv.Uptime > uptime {
			uptime = srv.Uptime
		}
	}

	nodes := make([]dashboard.NodeStatus, 0, len(info.Servers))
	warnings := make([]string, 0)
	for _, srv := range info.Servers {
		drives := dashboard.DriveCount{Total: len(srv.Disks)}
		for _, d := range srv.Disks {
			if strings.EqualFold(d.State, "ok") {
				drives.Healthy++
			} else {
				drives.Unhealthy++
			}
		}
		nodes = append(nodes, dashboard.NodeStatus{
			Endpoint: srv.Endpoint,
			State:    srv.State,
			Drives:   drives,
		})
		if !strings.EqualFold(srv.State, "online") && srv.State != "" {
			warnings = append(warnings,
				"node "+srv.Endpoint+" reported state "+srv.State)
		}
		if drives.Unhealthy > 0 {
			warnings = append(warnings,
				"node "+srv.Endpoint+" has unhealthy drives")
		}
	}

	return dashboard.ServerInfo{
		Version:        version,
		DeploymentMode: info.Mode,
		UptimeSeconds:  uptime,
	}, nodes, warnings, nil
}

// newDashboardPoolGetter returns a dashboard.PoolGetter bound to the
// live MinIO pool. The adapter type owns the per-call ServerInfo RPC and
// the warning-policy translation; the dashboard processor only sees the
// small dashboard view types.
func newDashboardPoolGetter(pool *hmminio.Pool) dashboard.PoolGetter {
	return dashboardPoolAdapter{pool: pool}
}

// newMetricsSourceGetter returns a metrics.SourceGetter bound to the live
// pool. Each call builds a fresh madmin MetricsClient (cheap; re-reads creds
// + transport) so credential rotations are picked up automatically.
func newMetricsSourceGetter(pool *hmminio.Pool) metrics.SourceGetter {
	return func(ctx context.Context) (metrics.MetricsSource, error) {
		return pool.NewMetricsClient(ctx)
	}
}
