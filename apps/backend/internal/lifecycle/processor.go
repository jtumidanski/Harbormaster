package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"strings"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// s3API is the subset of *miniogo.Client the lifecycle processor uses.
// Defining it as a local interface lets tests substitute an in-memory
// stub without standing up a fake MinIO server. The wire-up site
// adapts the live miniogo.Client to this shape via the S3Client
// indirection below.
type s3API interface {
	GetBucketLifecycle(ctx context.Context, bucket string) (*mlifecycle.Configuration, error)
	SetBucketLifecycle(ctx context.Context, bucket string, config *mlifecycle.Configuration) error
}

// ClientGetter is the concrete dependency the Processor pulls from on
// every call. The HTTP layer adapts internal/minio.Pool to this shape
// so the package never imports the live pool type; tests inject a
// getter that returns a hand-rolled stub satisfying s3API.
type ClientGetter func(ctx context.Context) (s3API, error)

// S3Client is the public face of s3API. It exists so callers outside
// the package (the HTTP wiring in cmd/harbormaster) can supply a live
// minio-go adapter to NewClientGetter without leaking the unexported
// s3API shape into the surrounding code. The live *miniogo.Client
// already satisfies this directly.
type S3Client interface {
	GetBucketLifecycle(ctx context.Context, bucket string) (*mlifecycle.Configuration, error)
	SetBucketLifecycle(ctx context.Context, bucket string, config *mlifecycle.Configuration) error
}

// NewClientGetter adapts a resolver that yields the public S3Client
// into a ClientGetter compatible with the unexported s3API interface
// used inside the package. This is the supported integration point for
// the HTTP layer; callers should not fabricate a ClientGetter literal
// because the underlying interface type is intentionally unexported.
func NewClientGetter(resolve func(ctx context.Context) (S3Client, error)) ClientGetter {
	return func(ctx context.Context) (s3API, error) {
		s3, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return s3, nil
	}
}

// Processor is the lifecycle-domain orchestrator. It depends only on
// the ClientGetter — there is no GORM DB here because the domain has
// no local persistence.
//
// Logger surfaces best-effort operational warnings. The default is
// zerolog.Nop so unit tests need not configure it; the HTTP wire-up
// calls WithLogger to inject the real logger. Audit is the (optional)
// audit.Processor handle used by the REST layer to record actions;
// T3.12-3.13 leave audit wiring stubbed and T3.23 wires the actual
// Record calls.
type Processor struct {
	Clients ClientGetter
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

// NewProcessor returns a Processor bound to the supplied ClientGetter.
// The logger defaults to zerolog.Nop; use WithLogger to attach the
// real logger. The audit handle defaults to nil; use WithAudit at the
// wire-up site once T3.23 lands.
func NewProcessor(c ClientGetter) *Processor {
	return &Processor{Clients: c, Logger: zerolog.Nop()}
}

// WithLogger returns p with the supplied logger attached.
func (p *Processor) WithLogger(l zerolog.Logger) *Processor {
	p.Logger = l
	return p
}

// WithAudit returns p with the supplied audit processor attached.
// Wired at the HTTP construction site; T3.12-3.13 do not yet invoke
// Record but keeping the slot wired now means T3.23 can light it up
// without changing the Processor surface.
func (p *Processor) WithAudit(a *audit.Processor) *Processor {
	p.Audit = a
	return p
}

// recordAudit is a nil-safe helper. Audit writes are best-effort and
// must never surface to the operator's foreground operation.
func (p *Processor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

// List returns the classified rule set currently attached to bucket.
// A bucket with no lifecycle config returns an empty slice and a nil
// error — MinIO reports the absence via the typed
// "NoSuchLifecycleConfiguration" error code, which we silently treat
// as "no rules" so the SPA renders an empty table rather than an error
// banner.
func (p *Processor) List(ctx context.Context, bucket string) ([]Rule, error) {
	s3, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := s3.GetBucketLifecycle(ctx, bucket)
	if err != nil {
		if isNoSuchLifecycleConfiguration(err) {
			return []Rule{}, nil
		}
		return nil, mapClientError(err, "failed to load lifecycle configuration")
	}
	if cfg == nil || len(cfg.Rules) == 0 {
		return []Rule{}, nil
	}
	out := make([]Rule, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		out = append(out, classify(r))
	}
	return out, nil
}

// Create generates a deterministic-ID expiration rule from days/prefix
// and MERGES it into the bucket's existing lifecycle configuration via
// SetBucketLifecycle (MinIO's PUT endpoint replaces the whole config,
// so we read-modify-write). If a rule with the same generated ID
// already exists it is replaced in place; this keeps the operation
// idempotent for the "click create twice" UI case.
//
// The processor enforces the operator-facing constraint (days > 0) up
// front; the wire layer enforces kind=="expiration" because it's the
// only managed kind v1 accepts.
func (p *Processor) Create(ctx context.Context, bucket string, days int, prefix, actor, sourceIP string) (Rule, error) {
	id := generateRuleID(days, prefix)
	payload := map[string]any{
		"bucket":  bucket,
		"rule_id": id,
		"days":    days,
		"prefix":  prefix,
	}
	failAudit := p.lifecycleFailAudit(ctx, actor, sourceIP, bucket, payload)
	if err := validateDays(days); err != nil {
		return Rule{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule", err.Error()).WithPointer("/data/attributes/days"))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return Rule{}, failAudit(err)
	}
	newRule := mlifecycle.Rule{
		ID:     id,
		Status: "Enabled",
		Expiration: mlifecycle.Expiration{
			Days: mlifecycle.ExpirationDays(days),
		},
		RuleFilter: mlifecycle.Filter{
			Prefix: prefix,
		},
	}
	return p.upsertManaged(ctx, s3, bucket, newRule, actor, sourceIP, payload, failAudit)
}

// lifecycleFailAudit returns a closure that records a FAILURE audit row
// and passes the error through. Callers assign the result to a local
// failAudit variable and call it on every error path so the audit trail
// is complete without repeating the boilerplate in each method.
func (p *Processor) lifecycleFailAudit(ctx context.Context, actor, sourceIP, bucket string, payload map[string]any) func(error) error {
	return func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionLifecycleRuleCreate,
			TargetType:     "bucket",
			TargetID:       bucket,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
}

// upsertManaged performs the read-modify-write round-trip shared by all
// Create* methods: it fetches the current lifecycle config (treating
// NoSuchLifecycleConfiguration as "empty"), merges newRule via upsertRule,
// writes the result back, and records a SUCCESS audit row. On any error it
// calls failAudit and returns. The returned Rule is classify(newRule).
func (p *Processor) upsertManaged(
	ctx context.Context,
	s3 s3API,
	bucket string,
	newRule mlifecycle.Rule,
	actor, sourceIP string,
	payload map[string]any,
	failAudit func(error) error,
) (Rule, error) {
	cfg, err := s3.GetBucketLifecycle(ctx, bucket)
	if err != nil && !isNoSuchLifecycleConfiguration(err) {
		return Rule{}, failAudit(mapClientError(err, "failed to load lifecycle configuration"))
	}
	if cfg == nil {
		cfg = mlifecycle.NewConfiguration()
	}
	cfg.Rules = upsertRule(cfg.Rules, newRule)
	if err := s3.SetBucketLifecycle(ctx, bucket, cfg); err != nil {
		return Rule{}, failAudit(mapClientError(err, "failed to save lifecycle configuration"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionLifecycleRuleCreate,
		TargetType:     "bucket",
		TargetID:       bucket,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return classify(newRule), nil
}

// CreateNoncurrent generates a deterministic-ID noncurrent-version-expiration
// rule from noncurrentDays/newerNoncurrent/prefix and merges it into the
// bucket's existing lifecycle configuration via SetBucketLifecycle. If a
// rule with the same generated ID already exists it is replaced in place
// (idempotent for the "click create twice" UI case).
func (p *Processor) CreateNoncurrent(ctx context.Context, bucket string, noncurrentDays, newerNoncurrent int, prefix, actor, sourceIP string) (Rule, error) {
	id := generateNoncurrentRuleID(noncurrentDays, prefix)
	payload := map[string]any{
		"bucket":                    bucket,
		"rule_id":                   id,
		"kind":                      KindNoncurrentExpiration,
		"noncurrent_days":           noncurrentDays,
		"newer_noncurrent_versions": newerNoncurrent,
		"prefix":                    prefix,
	}
	failAudit := p.lifecycleFailAudit(ctx, actor, sourceIP, bucket, payload)
	if err := validateNoncurrent(noncurrentDays, newerNoncurrent); err != nil {
		return Rule{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule", err.Error()).WithPointer("/data/attributes/noncurrent_days"))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return Rule{}, failAudit(err)
	}
	newRule := mlifecycle.Rule{
		ID:     id,
		Status: "Enabled",
		NoncurrentVersionExpiration: mlifecycle.NoncurrentVersionExpiration{
			NoncurrentDays:          mlifecycle.ExpirationDays(noncurrentDays),
			NewerNoncurrentVersions: newerNoncurrent,
		},
		RuleFilter: mlifecycle.Filter{
			Prefix: prefix,
		},
	}
	return p.upsertManaged(ctx, s3, bucket, newRule, actor, sourceIP, payload, failAudit)
}

// CreateAbortMPU generates a deterministic-ID abort-incomplete-multipart-upload
// rule from days/prefix and merges it into the bucket's existing lifecycle
// configuration via SetBucketLifecycle. If a rule with the same generated ID
// already exists it is replaced in place (idempotent for the "click create
// twice" UI case).
func (p *Processor) CreateAbortMPU(ctx context.Context, bucket string, days int, prefix, actor, sourceIP string) (Rule, error) {
	id := generateAbortMPURuleID(days, prefix)
	payload := map[string]any{
		"bucket":                bucket,
		"rule_id":               id,
		"kind":                  KindAbortIncompleteMPU,
		"days_after_initiation": days,
		"prefix":                prefix,
	}
	failAudit := p.lifecycleFailAudit(ctx, actor, sourceIP, bucket, payload)
	if err := validateDaysAfterInitiation(days); err != nil {
		return Rule{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule", err.Error()).WithPointer("/data/attributes/days_after_initiation"))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return Rule{}, failAudit(err)
	}
	newRule := mlifecycle.Rule{
		ID:     id,
		Status: "Enabled",
		AbortIncompleteMultipartUpload: mlifecycle.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: mlifecycle.ExpirationDays(days),
		},
		RuleFilter: mlifecycle.Filter{
			Prefix: prefix,
		},
	}
	return p.upsertManaged(ctx, s3, bucket, newRule, actor, sourceIP, payload, failAudit)
}

// Delete removes the rule with id ruleID from bucket's lifecycle
// configuration. A missing rule is a no-op (returns nil) so a
// double-DELETE from the UI does not raise a spurious 404; this
// matches the typical REST-on-S3 ergonomic where "the desired state is
// already true" is success. Removing the last rule clears the config
// (SetBucketLifecycle calls removeBucketLifecycle when the config is
// empty), so the bucket returns to the "no lifecycle" state cleanly.
func (p *Processor) Delete(ctx context.Context, bucket, ruleID, actor, sourceIP string) error {
	payload := map[string]any{"bucket": bucket, "rule_id": ruleID}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionLifecycleRuleDelete,
			TargetType:     "bucket",
			TargetID:       bucket,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	cfg, err := s3.GetBucketLifecycle(ctx, bucket)
	if err != nil {
		if isNoSuchLifecycleConfiguration(err) {
			// No-op for idempotent double-DELETE; do not emit an audit
			// row because no state changed.
			return nil
		}
		return failAudit(mapClientError(err, "failed to load lifecycle configuration"))
	}
	if cfg == nil {
		return nil
	}
	filtered, removed := removeRule(cfg.Rules, ruleID)
	if !removed {
		// Idempotent no-op (rule already absent); do not emit audit.
		return nil
	}
	cfg.Rules = filtered
	if err := s3.SetBucketLifecycle(ctx, bucket, cfg); err != nil {
		return failAudit(mapClientError(err, "failed to save lifecycle configuration"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionLifecycleRuleDelete,
		TargetType:     "bucket",
		TargetID:       bucket,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return nil
}

// clients is a tiny indirection that wraps the ClientGetter's error in
// a typed 502 envelope; the rest of the processor calls this so the
// every-method "fetch client" preamble is uniform.
func (p *Processor) clients(ctx context.Context) (s3API, error) {
	s3, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusBadGateway,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return s3, nil
}

// upsertRule returns rules with a single Rule equal to next: any
// existing rule sharing next.ID is replaced in place (preserving
// ordering) and the result is appended if no match was found. The
// in-place replacement keeps the UI's "I clicked Create again with
// the same params" idempotent without surfacing a "duplicate rule"
// error.
func upsertRule(rules []mlifecycle.Rule, next mlifecycle.Rule) []mlifecycle.Rule {
	for i, r := range rules {
		if r.ID == next.ID {
			rules[i] = next
			return rules
		}
	}
	return append(rules, next)
}

// removeRule returns the rule slice with any entry matching id elided
// and a boolean indicating whether a removal actually happened. The
// boolean lets Delete short-circuit the SetBucketLifecycle round-trip
// when the requested rule was already absent.
func removeRule(rules []mlifecycle.Rule, id string) ([]mlifecycle.Rule, bool) {
	out := rules[:0:0]
	removed := false
	for _, r := range rules {
		if r.ID == id {
			removed = true
			continue
		}
		out = append(out, r)
	}
	return out, removed
}

// isNoSuchLifecycleConfiguration recognises MinIO's "no lifecycle set"
// error so the read path can return an empty slice instead of bubbling
// the error. The code string is the AWS-spec value MinIO emits in the
// XML ErrorResponse body; minio-go surfaces it as the typed
// ErrorResponse.Code field but we match on the wrapped error string to
// stay independent of the SDK's exact error-shape choice (and to keep
// the tests free of import gymnastics).
func isNoSuchLifecycleConfiguration(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchLifecycleConfiguration") ||
		strings.Contains(msg, "The lifecycle configuration does not exist")
}

// mapClientError wraps an upstream MinIO transport/admin error in the
// typed 502 envelope the REST layer renders. The "lifecycle: " prefix
// keeps the log message scannable when correlating with audit rows.
func mapClientError(err error, hint string) error {
	if err == nil {
		return nil
	}
	// Already a typed apierror? Pass it through so callers don't double-wrap.
	var ae *apierror.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apierror.New(http.StatusBadGateway, "minio_unavailable",
		hint+": "+err.Error())
}
