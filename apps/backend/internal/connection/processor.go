package connection

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	minioPool "github.com/jtumidanski/Harbormaster/internal/minio"
)

// sanitizeEndpointForAudit returns a credential-free rendering of raw that
// only preserves scheme + host (with port). User-info, path, query, and
// fragment are dropped because operators sometimes paste pre-signed URLs or
// `https://user:pass@host` shorthand into the wizard, and the audit log is
// not the place to retain those. On parse failure we return the literal
// "invalid_url" rather than echo the raw input.
func sanitizeEndpointForAudit(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Host == "" {
		return "invalid_url"
	}
	scrubbed := &url.URL{Scheme: u.Scheme, Host: u.Host}
	return scrubbed.String()
}

// Prober is the contract used by Processor to validate a candidate
// SubmitInput. Production wires it to the package-level Probe; tests can
// inject a stub to avoid hitting the network.
type Prober func(ctx context.Context, in SubmitInput) (TestResult, *apierror.Error)

// Processor coordinates reads and writes for the minio_connections
// singleton, the validation probe, and the live MinIO client pool.
//
// Wiring (set in cmd/harbormaster): DB is the migrated *gorm.DB; Cipher is
// the AES-256-GCM cipher constructed from the master key; Pool is the
// shared *minio.Pool that downstream domains read from. The Probe field
// defaults to the package-level Probe and may be overridden for tests.
type Processor struct {
	DB     *gorm.DB
	Cipher *crypto.Cipher
	Pool   *minioPool.Pool
	Probe  Prober
	Audit  *audit.Processor // optional; nil disables audit emission
}

// NewProcessor returns a Processor with the default network probe wired up.
// Override .Probe after construction for tests.
func NewProcessor(db *gorm.DB, c *crypto.Cipher, p *minioPool.Pool) *Processor {
	return &Processor{
		DB:     db,
		Cipher: c,
		Pool:   p,
		Probe:  Probe,
	}
}

// recordAudit is a nil-safe helper. Audit writes are best-effort and must
// never surface to the operator's foreground operation.
func (p *Processor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

// Validate runs the probe and returns the typed apierror as a plain error.
// Returning *apierror.Error directly through an `error` interface keeps the
// envelope discoverable via errors.As at the HTTP layer.
func (p *Processor) Validate(ctx context.Context, in SubmitInput) error {
	if err := validateSubmit(in); err != nil {
		return err
	}
	if _, ae := p.Probe(ctx, in); ae != nil {
		return ae
	}
	return nil
}

// PersistInTx encrypts in's credentials and upserts the singleton row using
// the supplied transaction. The caller owns the txn lifecycle so this can
// be composed with sibling domain writes (e.g. internal/setup's bootstrap).
func (p *Processor) PersistInTx(ctx context.Context, tx *gorm.DB, in SubmitInput) error {
	if err := validateSubmit(in); err != nil {
		return err
	}
	now := time.Now().UTC()
	e, err := ToEntity(in, p.Cipher, now)
	if err != nil {
		return apierror.Internal("failed to encrypt connection credentials")
	}
	if err := upsertSingleton(tx.WithContext(ctx), e); err != nil {
		return apierror.Internal("failed to persist connection: " + err.Error())
	}
	return nil
}

// Update is the public mutation used by PUT /api/v1/connection. It runs
// Validate (probe), then writes in a single transaction, then rebuilds the
// live MinIO client pool. Pool.Rebuild happens *after* commit so a probe
// success followed by a write failure does not leave the live pool pointing
// at credentials that were never persisted.
//
// actor and sourceIP are stamped onto the audit row; the resource layer is
// responsible for sourcing them from the authenticated session context.
func (p *Processor) Update(ctx context.Context, in SubmitInput, actor, sourceIP string) error {
	skipVerify := false
	if in.TLSSkipVerify != nil {
		skipVerify = *in.TLSSkipVerify
	}
	// The `url`-substring filter in audit.Sanitize would drop a key like
	// "endpoint_url", so the payload uses bare "endpoint" instead. The
	// value is scrubbed through sanitizeEndpointForAudit so any user-info
	// (e.g. https://user:pass@host) or query-string token an operator
	// pasted into the wizard never reaches the audit log.
	auditPayload := map[string]any{
		"endpoint":          sanitizeEndpointForAudit(in.EndpointURL),
		"tls_skip_verify":   skipVerify,
		"custom_ca_pem_set": in.CustomCAPEM != "",
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionConnectionUpdate,
			TargetType:     "connection",
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := p.Validate(ctx, in); err != nil {
		return failAudit(err)
	}
	if err := p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return p.PersistInTx(ctx, tx, in)
	}); err != nil {
		return failAudit(err)
	}
	if err := p.Pool.Rebuild(minioPool.Credentials{
		EndpointURL:     in.EndpointURL,
		AccessKey:       in.AccessKey,
		SecretKey:       in.SecretKey,
		TLSSkipVerify:   skipVerify,
		CustomCAPEMText: in.CustomCAPEM,
	}); err != nil {
		// The row is already written. A Rebuild failure leaves the
		// on-disk record valid but the in-process pool stale; report as
		// an internal error so the operator retries. The next process
		// boot will rebuild from the persisted row.
		return failAudit(apierror.Internal("failed to rebuild minio client pool: " + err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionConnectionUpdate,
		TargetType:     "connection",
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// Get reads the singleton row and returns the masked-view Connection.
// Returns apierror.NotFound when nothing has been persisted yet.
func (p *Processor) Get(ctx context.Context) (Connection, error) {
	e, err := getSingleton()(p.DB.WithContext(ctx))
	if err != nil {
		if errors.Is(err, ErrNoConnection) {
			return Connection{}, apierror.NotFound("connection")
		}
		return Connection{}, apierror.Internal("failed to read connection: " + err.Error())
	}
	view, _, err := Make(e, p.Cipher)
	if err != nil {
		return Connection{}, apierror.Internal("failed to decrypt connection: " + err.Error())
	}
	return view, nil
}

// Test runs Probe without persisting. The HTTP handler always returns 200
// with the TestResult body; the *apierror.Error (when non-nil) is informational
// — its Code drives the per-step "failed" string and is included in the
// response when the wizard wants to surface the structured failure reason.
//
// We intentionally return both so the handler can pick: api-contracts.md
// shows a successful 200 carrying mixed-step results.
//
// Audit policy: success is silent (operators run the probe repeatedly during
// setup; a row per click would drown out the signal). Only failures are
// recorded, with the per-step outcomes summarised in the payload.
func (p *Processor) Test(ctx context.Context, in SubmitInput, actor, sourceIP string) (TestResult, *apierror.Error) {
	result, ae := p.runTest(ctx, in)
	if ae != nil {
		p.recordAudit(ctx, audit.Event{
			Actor:        actor,
			SourceIP:     sourceIP,
			Action:       audit.ActionConnectionTest,
			TargetType:   "connection",
			Outcome:      audit.OutcomeFailure,
			ErrorMessage: ae.Message,
			PayloadSummary: map[string]any{
				"endpoint":     sanitizeEndpointForAudit(in.EndpointURL),
				"tcp_connect":  stepStatus(result.TCPConnect),
				"list_buckets": stepStatus(result.ListBuckets),
				"admin_ping":   stepStatus(result.AdminPing),
				"reason":       ae.Code,
			},
		})
	}
	return result, ae
}

// runTest is the non-audited core of Test. Extracted so the audit hook in
// Test can wrap a single return path.
func (p *Processor) runTest(ctx context.Context, in SubmitInput) (TestResult, *apierror.Error) {
	if err := validateSubmit(in); err != nil {
		// Surface body validation as a probe failure on the TCP step.
		var ae *apierror.Error
		if errors.As(err, &ae) {
			return TestResult{TCPConnect: map[string]string{"failed": ae.Message}}, ae
		}
		return TestResult{TCPConnect: map[string]string{"failed": err.Error()}},
			apierror.New(http.StatusUnprocessableEntity, "minio_unreachable", err.Error())
	}
	return p.Probe(ctx, in)
}

// stepStatus reduces a TestResult step (either "ok", "", or
// map[string]string{"failed": "..."}) to a short, audit-safe label. The
// underlying error text is intentionally NOT included in the payload — that
// belongs in the audit Event.ErrorMessage column, not the JSON summary.
func stepStatus(v any) string {
	switch s := v.(type) {
	case string:
		if s == "" {
			return "skipped"
		}
		return s
	case map[string]string:
		if _, failed := s["failed"]; failed {
			return "failed"
		}
		return "ok"
	default:
		return "skipped"
	}
}

// validateSubmit enforces the minimum field set required by the probe.
// FromMcAlias is intentionally not checked here — that's setup's concern.
func validateSubmit(in SubmitInput) error {
	if strings.TrimSpace(in.EndpointURL) == "" {
		return apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"endpoint_url is required")
	}
	if in.AccessKey == "" {
		return apierror.New(http.StatusUnprocessableEntity, "minio_invalid_credentials",
			"access_key is required")
	}
	if in.SecretKey == "" {
		return apierror.New(http.StatusUnprocessableEntity, "minio_invalid_credentials",
			"secret_key is required")
	}
	return nil
}
