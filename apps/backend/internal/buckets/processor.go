package buckets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// LifecycleCreator is the narrow contract the bucket processor needs to
// apply a bundled lifecycle template on bucket creation (T3.21). Defining
// it here — rather than importing internal/lifecycle directly — keeps the
// import graph acyclic and avoids leaking the lifecycle.Rule return type
// into the bucket domain. The wire-up site (serve.go) supplies a thin
// adapter around *lifecycle.Processor that swallows the Rule (the bucket
// caller only cares about success vs. failure for the audit row).
type LifecycleCreator interface {
	Create(ctx context.Context, bucket string, days int, prefix string) error
}

// lifecycleTemplate is a bundled (days, prefix) pair keyed by the wire
// name. Only "expire-30d" and "expire-90d" are accepted in v1; unknown
// names surface as a typed 422 envelope from Create.
type lifecycleTemplate struct {
	days   int
	prefix string
}

var lifecycleTemplates = map[string]lifecycleTemplate{
	"expire-30d": {days: 30, prefix: ""},
	"expire-90d": {days: 90, prefix: ""},
}

// fanoutConcurrency caps the number of in-flight per-bucket detail fetches
// during List. Ten is enough to overlap the typical "list buckets + per-
// bucket usage/versioning/policy/quota" RTT pattern on a homelab MinIO
// without pegging the admin server's request queue.
const fanoutConcurrency = 10

// adminAPI is the subset of *madmin.AdminClient the bucket processor uses.
// Defining it as a local interface lets tests substitute an in-memory
// stub without standing up a fake MinIO server.
type adminAPI interface {
	// BucketUsageInfo returns the per-bucket usage row from MinIO's
	// data-usage scanner. The implementation typically caches a single
	// DataUsageInfo() call and indexes into BucketsUsage[bucket]; the
	// interface keeps that policy at the adapter boundary.
	BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error)
	GetBucketQuota(ctx context.Context, bucket string) (madmin.BucketQuota, error)
	SetBucketQuota(ctx context.Context, bucket string, quota *madmin.BucketQuota) error
}

// s3API is the subset of *miniogo.Client the bucket processor uses.
type s3API interface {
	ListBuckets(ctx context.Context) ([]miniogo.BucketInfo, error)
	// BucketExists is a HEAD-bucket call. The Get path uses it as a cheap
	// presence probe so a missing bucket can be mapped to a typed 404
	// without scanning the full ListBuckets response (which would also
	// have to translate any transport error into a typed envelope).
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, opts miniogo.MakeBucketOptions) error
	RemoveBucket(ctx context.Context, bucket string) error
	GetBucketPolicy(ctx context.Context, bucket string) (string, error)
	SetBucketPolicy(ctx context.Context, bucket, policy string) error
	GetBucketVersioning(ctx context.Context, bucket string) (miniogo.BucketVersioningConfiguration, error)
	SetBucketVersioning(ctx context.Context, bucket string, config miniogo.BucketVersioningConfiguration) error
	GetBucketLifecycle(ctx context.Context, bucket string) (*lifecycle.Configuration, error)
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
}

// ClientGetter is the concrete dependency the Processor pulls from on
// every call. The HTTP layer adapts internal/minio.Pool to this shape so
// the package never imports the live pool type; tests inject a getter that
// returns hand-rolled stubs satisfying adminAPI / s3API.
type ClientGetter func(ctx context.Context) (adminAPI, s3API, error)

// AdminClient is the public face of adminAPI. It exists so callers outside
// the package (the HTTP wiring in cmd/harbormaster) can supply a live
// admin-client adapter to NewClientGetter without leaking the unexported
// adminAPI shape into the surrounding code.
type AdminClient interface {
	BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error)
	GetBucketQuota(ctx context.Context, bucket string) (madmin.BucketQuota, error)
	SetBucketQuota(ctx context.Context, bucket string, quota *madmin.BucketQuota) error
}

// S3Client is the public face of s3API. It mirrors the methods the
// processor invokes against an active minio-go client. The live
// *miniogo.Client already satisfies this shape.
type S3Client interface {
	ListBuckets(ctx context.Context) ([]miniogo.BucketInfo, error)
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, opts miniogo.MakeBucketOptions) error
	RemoveBucket(ctx context.Context, bucket string) error
	GetBucketPolicy(ctx context.Context, bucket string) (string, error)
	SetBucketPolicy(ctx context.Context, bucket, policy string) error
	GetBucketVersioning(ctx context.Context, bucket string) (miniogo.BucketVersioningConfiguration, error)
	SetBucketVersioning(ctx context.Context, bucket string, config miniogo.BucketVersioningConfiguration) error
	GetBucketLifecycle(ctx context.Context, bucket string) (*lifecycle.Configuration, error)
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
}

// NewClientGetter adapts a resolver that yields the public AdminClient /
// S3Client pair into a ClientGetter compatible with the unexported
// adminAPI / s3API interfaces used inside the package. This is the
// supported integration point for the HTTP layer; callers should not
// fabricate a ClientGetter literal because the underlying interface types
// are intentionally unexported.
func NewClientGetter(resolve func(ctx context.Context) (AdminClient, S3Client, error)) ClientGetter {
	return func(ctx context.Context) (adminAPI, s3API, error) {
		adm, s3, err := resolve(ctx)
		if err != nil {
			return nil, nil, err
		}
		return adm, s3, nil
	}
}

// CreateOpts captures the optional knobs a single POST /buckets request
// can flip during bucket creation. All zero values are valid — the
// processor only invokes the corresponding helper when the field is set.
type CreateOpts struct {
	VersioningEnabled bool
	PublicAccess      PublicAccess
	Quota             *Quota
	// LifecycleTemplate is one of "transition-30d" | "expire-90d" | "".
	// Wired in T3.21; T3.1 records the value for the audit row but does
	// not call any handler.
	LifecycleTemplate string
}

// Processor is the bucket-domain orchestrator. It depends only on the
// ClientGetter — there is no GORM DB here because the domain has no local
// persistence.
//
// Logger is used to surface best-effort sub-fetch failures (per-bucket
// usage and quota) that intentionally do not fail the parent call. The
// default value is a zerolog.Nop so unit tests need not configure it; the
// HTTP wire-up calls WithLogger to inject the real logger.
//
// Audit is the (optional) audit.Processor handle used to record bucket
// mutations. nil disables audit emission — call WithAudit at the
// composition root to enable it.
//
// Lifecycle is the (optional) LifecycleCreator used by Create to apply
// the bundled "expire-30d" / "expire-90d" templates after MakeBucket
// succeeds. nil disables template application (the field is then a
// validation-only knob for the audit payload).
type Processor struct {
	Clients   ClientGetter
	Logger    zerolog.Logger
	Audit     *audit.Processor
	Lifecycle LifecycleCreator
}

// NewProcessor returns a Processor bound to clients. The logger defaults
// to zerolog.Nop; use WithLogger to attach the real logger.
func NewProcessor(clients ClientGetter) *Processor {
	return &Processor{Clients: clients, Logger: zerolog.Nop()}
}

// WithLogger returns p with the supplied logger attached. Best-effort
// sub-fetch failures (usage/quota) log at warn level via this logger so an
// operator can see partial-result events that a 200 response otherwise
// hides.
func (p *Processor) WithLogger(l zerolog.Logger) *Processor {
	p.Logger = l
	return p
}

// WithAudit returns p with the supplied audit processor attached.
func (p *Processor) WithAudit(a *audit.Processor) *Processor {
	p.Audit = a
	return p
}

// WithLifecycle returns p with the supplied LifecycleCreator attached.
// Required for Create to honour CreateOpts.LifecycleTemplate.
func (p *Processor) WithLifecycle(lc LifecycleCreator) *Processor {
	p.Lifecycle = lc
	return p
}

// recordAudit is a nil-safe helper. Audit writes are best-effort and must
// never surface to the operator's foreground operation.
func (p *Processor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

// List returns every bucket on the configured MinIO with the auxiliary
// settings populated. Per-bucket detail fetches fan out under an
// errgroup with concurrency capped at fanoutConcurrency.
//
// The result is sorted by Name so the UI gets a stable order without
// depending on MinIO's listing order.
func (p *Processor) List(ctx context.Context) ([]Bucket, error) {
	adm, s3, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	infos, err := s3.ListBuckets(ctx)
	if err != nil {
		return nil, mapClientError(err, "failed to list buckets")
	}

	// Each goroutine writes to its own index, so no shared-mutation
	// guard is required.
	out := make([]Bucket, len(infos))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanoutConcurrency)
	for i := range infos {
		i := i
		info := infos[i]
		g.Go(func() error {
			b, err := p.detail(gctx, adm, s3, info)
			if err != nil {
				return err
			}
			out[i] = b
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns the single-bucket view, running the same auxiliary fetches
// as List for one bucket.
//
// Presence is probed via BucketExists (a cheap HEAD) so a missing bucket
// surfaces as a typed 404 rather than the generic 502 a ListBuckets-scan
// fallback would produce when the bucket simply does not exist. A
// transport error from BucketExists is still mapped through mapClientError
// as a 502.
//
// CreationDate is intentionally not fetched here: MinIO has no per-bucket
// info endpoint, and pulling the full ListBuckets response just to scan
// for one entry's CreationDate would defeat the point of the cheap probe.
// The List endpoint remains the source for CreationDate.
func (p *Processor) Get(ctx context.Context, name string) (Bucket, error) {
	if err := ValidateBucketName(name); err != nil {
		return Bucket{}, apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error())
	}
	adm, s3, err := p.clients(ctx)
	if err != nil {
		return Bucket{}, err
	}
	exists, err := s3.BucketExists(ctx, name)
	if err != nil {
		return Bucket{}, mapClientError(err, "failed to look up bucket")
	}
	if !exists {
		return Bucket{}, apierror.NotFound("bucket")
	}
	return p.detail(ctx, adm, s3, miniogo.BucketInfo{Name: name})
}

// detail performs the per-bucket fan-out: usage, versioning, lifecycle,
// policy, and quota. Errors from any sub-call short-circuit the bucket;
// individual sub-fetches that legitimately return "not configured" (e.g.
// no policy, no lifecycle) are normalised to the empty domain value.
func (p *Processor) detail(ctx context.Context, adm adminAPI, s3 s3API, info miniogo.BucketInfo) (Bucket, error) {
	b := bucketFromInfo(info)

	var (
		usage    madmin.BucketUsageInfo
		quota    madmin.BucketQuota
		ver      miniogo.BucketVersioningConfiguration
		policy   string
		lc       *lifecycle.Configuration
		usageErr error
		quotaErr error
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var e error
		usage, e = adm.BucketUsageInfo(gctx, info.Name)
		usageErr = e
		return nil // usage failures are non-fatal — surface zero values
	})
	g.Go(func() error {
		var e error
		quota, e = adm.GetBucketQuota(gctx, info.Name)
		quotaErr = e
		return nil // a "no quota set" 404 is normal; tolerate any error here.
	})
	g.Go(func() error {
		v, e := s3.GetBucketVersioning(gctx, info.Name)
		if e != nil {
			return mapClientError(e, "failed to read bucket versioning")
		}
		ver = v
		return nil
	})
	g.Go(func() error {
		raw, e := s3.GetBucketPolicy(gctx, info.Name)
		if e != nil {
			// "no policy" returns an SDK error; treat any error as
			// "private" rather than failing the whole fetch.
			policy = ""
			return nil
		}
		policy = raw
		return nil
	})
	g.Go(func() error {
		cfg, e := s3.GetBucketLifecycle(gctx, info.Name)
		if e != nil {
			lc = nil
			return nil
		}
		lc = cfg
		return nil
	})
	if err := g.Wait(); err != nil {
		return Bucket{}, err
	}

	// Best-effort sub-fetches: a usage scan can lag behind a newly
	// created bucket, and the GetBucketQuota call returns an error when
	// no quota has been configured. Both are normal; surface them in the
	// log so an operator can investigate a bucket that perpetually shows
	// zero bytes, but never fail the parent request.
	if usageErr != nil {
		p.Logger.Warn().
			Err(usageErr).
			Str("bucket", info.Name).
			Msg("buckets: BucketUsageInfo failed; usage fields will be zero")
	}
	if quotaErr != nil {
		p.Logger.Warn().
			Err(quotaErr).
			Str("bucket", info.Name).
			Msg("buckets: GetBucketQuota failed; quota field will be empty")
	}
	b = applyUsage(b, usage)
	b.VersioningEnabled = versioningEnabled(ver)
	b.HasLifecycleRules = lc != nil && len(lc.Rules) > 0
	b.PublicAccess = publicAccessFromPolicy(policy)
	b.Quota = quotaFromMadmin(quota, b.EstimatedBytes)
	return b, nil
}

// Create makes a new bucket and applies the optional settings carried in
// opts. The bucket is created first; subsequent setting calls happen
// best-effort against the newly created bucket. On a partial failure the
// bucket is left in place — operators can re-issue the failing setting
// against the now-existing bucket via the dedicated action endpoint.
//
// actor and sourceIP are stamped onto the audit row; the resource layer is
// responsible for sourcing them from the authenticated session context.
// LifecycleTemplate (T3.21) is validated up-front: an unknown template
// short-circuits with apierror 422 unknown_lifecycle_template BEFORE
// MakeBucket runs so a typo cannot leak an orphaned bucket.
func (p *Processor) Create(ctx context.Context, name string, opts CreateOpts, actor, sourceIP string) (Bucket, error) {
	auditPayload := map[string]any{
		"name":               name,
		"versioning_enabled": opts.VersioningEnabled,
		"public_access":      string(opts.PublicAccess),
		"has_quota":          opts.Quota != nil && opts.Quota.Bytes > 0,
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionBucketCreate,
			TargetType:     "bucket",
			TargetID:       name,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateBucketName(name); err != nil {
		return Bucket{}, failAudit(apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error()))
	}
	// Up-front template validation: reject unknown wire names before
	// MakeBucket so a typo cannot leak an orphaned bucket.
	var tmpl *lifecycleTemplate
	if opts.LifecycleTemplate != "" {
		t, ok := lifecycleTemplates[opts.LifecycleTemplate]
		if !ok {
			return Bucket{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
				"unknown_lifecycle_template",
				fmt.Sprintf("lifecycle_template must be one of: expire-30d, expire-90d (got %q)", opts.LifecycleTemplate)))
		}
		tmpl = &t
	}
	adm, s3, err := p.clients(ctx)
	if err != nil {
		return Bucket{}, failAudit(err)
	}
	if err := s3.MakeBucket(ctx, name, miniogo.MakeBucketOptions{}); err != nil {
		return Bucket{}, failAudit(mapClientError(err, "failed to create bucket"))
	}
	if err := p.applyCreateOpts(ctx, adm, s3, name, opts, tmpl); err != nil {
		return Bucket{}, failAudit(err)
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionBucketCreate,
		TargetType:     "bucket",
		TargetID:       name,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return p.Get(ctx, name)
}

// applyCreateOpts applies the optional bucket settings (versioning, quota,
// public-access policy, lifecycle template) to a freshly created bucket.
// Each failure returns a typed apierror so Create routes it through failAudit.
func (p *Processor) applyCreateOpts(ctx context.Context, adm adminAPI, s3 s3API, name string, opts CreateOpts, tmpl *lifecycleTemplate) error {
	if opts.VersioningEnabled {
		if err := applyVersioning(ctx, s3, name, true); err != nil {
			return apierror.Internal(err.Error())
		}
	}
	if opts.Quota != nil && opts.Quota.Bytes > 0 {
		// FIFO requires versioning off; if the caller supplied
		// VersioningEnabled=true alongside a FIFO quota that's a
		// validation error the caller should have caught at the REST
		// layer. Defence in depth here so it's not silently accepted.
		if opts.Quota.Kind == QuotaKindFifo && opts.VersioningEnabled {
			return apierror.New(http.StatusUnprocessableEntity,
				"fifo_requires_versioning_off",
				"FIFO quota requires versioning to be disabled")
		}
		if err := applyQuota(ctx, adm, name, opts.Quota.Kind, opts.Quota.Bytes); err != nil {
			return apierror.Internal(err.Error())
		}
	}
	if opts.PublicAccess != "" && opts.PublicAccess != PublicAccessPrivate {
		policyJSON, perr := policies.BucketPolicyFor(name, string(opts.PublicAccess))
		if perr != nil {
			return apierror.New(http.StatusBadRequest, "public_access_invalid", perr.Error())
		}
		if err := applyPolicy(ctx, s3, name, policyJSON); err != nil {
			return apierror.Internal(err.Error())
		}
	}
	if tmpl != nil && p.Lifecycle != nil {
		if err := p.Lifecycle.Create(ctx, name, tmpl.days, tmpl.prefix); err != nil {
			return apierror.Internal("failed to apply lifecycle template: " + err.Error())
		}
	}
	return nil
}

// Delete removes bucket after confirming the name (operator typed the
// bucket name to authorize the delete) and that the bucket is empty.
// MinIO's RemoveBucket refuses non-empty buckets, but we re-check via
// ListObjects with MaxKeys=1 so the operator gets the typed
// "bucket_not_empty" envelope instead of the raw MinIO error.
func (p *Processor) Delete(ctx context.Context, name, confirmName, actor, sourceIP string) error {
	auditPayload := map[string]any{"name": name}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionBucketDelete,
			TargetType:     "bucket",
			TargetID:       name,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateBucketName(name); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error()))
	}
	if name != confirmName {
		return failAudit(apierror.New(http.StatusForbidden, "confirm_name_mismatch",
			"confirm_name must equal the bucket name"))
	}
	_, s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	// Re-check emptiness. We drain the channel after the first hit so
	// the underlying lister goroutine does not leak (per minio-go docs).
	objCh := s3.ListObjects(ctx, name, miniogo.ListObjectsOptions{MaxKeys: 1})
	nonEmpty := false
	// Peek at most one object to decide emptiness; drain the rest so the
	// underlying lister goroutine does not leak (per minio-go docs).
	if obj, ok := <-objCh; ok {
		if obj.Err != nil {
			go drain(objCh)
			return failAudit(mapClientError(obj.Err, "failed to check bucket emptiness"))
		}
		nonEmpty = true
		go drain(objCh)
	}
	if nonEmpty {
		return failAudit(apierror.New(http.StatusConflict, "bucket_not_empty",
			"bucket is not empty; delete or move all objects before retrying"))
	}
	if err := s3.RemoveBucket(ctx, name); err != nil {
		return failAudit(mapClientError(err, "failed to remove bucket"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionBucketDelete,
		TargetType:     "bucket",
		TargetID:       name,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// SetVersioning is the action endpoint for toggling versioning on an
// existing bucket. Suspends rather than removes — see applyVersioning.
func (p *Processor) SetVersioning(ctx context.Context, name string, enabled bool, actor, sourceIP string) error {
	action := audit.ActionBucketVersioningOff
	if enabled {
		action = audit.ActionBucketVersioningOn
	}
	auditPayload := map[string]any{"name": name}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         action,
			TargetType:     "bucket",
			TargetID:       name,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateBucketName(name); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error()))
	}
	_, s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := applyVersioning(ctx, s3, name, enabled); err != nil {
		return failAudit(apierror.Internal(err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         action,
		TargetType:     "bucket",
		TargetID:       name,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// SetPublicAccess materialises one of the three canned policies (or
// removes the policy for "private"). confirmName guards against accidental
// privilege escalation: the operator must re-type the bucket name when
// transitioning to a public mode, matching the destructive-action pattern
// used by Delete.
//
// T3.1 only implements the validation outline; the canned-policy JSON
// lives in internal/policies (T3.2). Until that package lands this method
// returns a typed 501 so the REST layer can wire the route without lying
// about the feature being live.
func (p *Processor) SetPublicAccess(ctx context.Context, name, mode, confirmName, actor, sourceIP string) error {
	auditPayload := map[string]any{"name": name, "mode": mode}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionBucketPublicUpdate,
			TargetType:     "bucket",
			TargetID:       name,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateBucketName(name); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error()))
	}
	pa := PublicAccess(mode)
	switch pa {
	case PublicAccessPrivate, PublicAccessPublicRead, PublicAccessPublicReadWrite:
	default:
		return failAudit(apierror.New(http.StatusBadRequest, "public_access_invalid",
			fmt.Sprintf("mode must be one of: private, public-read, public-read-write (got %q)", mode)))
	}
	// Per api-contracts.md §buckets/{name}/public-access: confirm_name is
	// required when transitioning into public-read-write (the write-allowing
	// mode); private and public-read transitions accept an empty/missing
	// confirm_name. Mismatch is 403 per the global error table.
	if pa == PublicAccessPublicReadWrite && name != confirmName {
		return failAudit(apierror.New(http.StatusForbidden, "confirm_name_mismatch",
			"confirm_name must equal the bucket name when granting public-read-write access"))
	}
	_, s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	policyJSON, perr := policies.BucketPolicyFor(name, string(pa))
	if perr != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "public_access_invalid", perr.Error()))
	}
	if err := applyPolicy(ctx, s3, name, policyJSON); err != nil {
		return failAudit(apierror.Internal(err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionBucketPublicUpdate,
		TargetType:     "bucket",
		TargetID:       name,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// SetQuota writes the bucket quota. Enforces the cross-domain invariant
// that a FIFO quota requires versioning to be disabled — otherwise the
// FIFO eviction loop (T3.21) would generate an unbounded chain of delete
// markers.
func (p *Processor) SetQuota(ctx context.Context, name string, kind QuotaKind, bytes int64, actor, sourceIP string) error {
	auditPayload := map[string]any{"name": name, "kind": string(kind), "bytes": bytes}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionBucketQuotaUpdate,
			TargetType:     "bucket",
			TargetID:       name,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateBucketName(name); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "bucket_invalid_name", err.Error()))
	}
	switch kind {
	case QuotaKindHard, QuotaKindFifo:
	default:
		return failAudit(apierror.New(http.StatusBadRequest, "quota_kind_invalid",
			fmt.Sprintf("kind must be 'hard' or 'fifo' (got %q)", kind)))
	}
	if bytes < 0 {
		return failAudit(apierror.New(http.StatusBadRequest, "quota_bytes_invalid",
			"bytes must be >= 0 (use 0 to clear the quota)"))
	}
	adm, s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if kind == QuotaKindFifo {
		ver, err := s3.GetBucketVersioning(ctx, name)
		if err != nil {
			return failAudit(mapClientError(err, "failed to read bucket versioning"))
		}
		if versioningEnabled(ver) {
			return failAudit(apierror.New(http.StatusUnprocessableEntity,
				"fifo_requires_versioning_off",
				"FIFO quota requires versioning to be disabled"))
		}
	}
	if err := applyQuota(ctx, adm, name, kind, bytes); err != nil {
		return failAudit(apierror.Internal(err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionBucketQuotaUpdate,
		TargetType:     "bucket",
		TargetID:       name,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// clients is a tiny indirection that wraps the ClientGetter's error in an
// apierror so callers can return it directly to the HTTP layer.
func (p *Processor) clients(ctx context.Context) (adminAPI, s3API, error) {
	if p.Clients == nil {
		return nil, nil, apierror.Internal("buckets: client getter not configured")
	}
	adm, s3, err := p.Clients(ctx)
	if err != nil {
		return nil, nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return adm, s3, nil
}

// mapClientError wraps a raw MinIO SDK error into the action-style
// apierror used by bucket endpoints. For now we forward the SDK message
// verbatim; a future task may translate well-known codes into typed
// envelopes (e.g. NoSuchBucket -> apierror.NotFound).
func mapClientError(err error, fallback string) *apierror.Error {
	if err == nil {
		return nil
	}
	var ae *apierror.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apierror.New(http.StatusBadGateway, "minio_error", fallback+": "+err.Error())
}

// drain consumes the remaining items on a ListObjects channel so the
// minio-go lister goroutine exits cleanly after an early-return.
func drain(ch <-chan miniogo.ObjectInfo) {
	for range ch {
	}
}
