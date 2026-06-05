package policies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// AdminClient is the public face of adminAPI. It exists so callers outside
// the package (the HTTP wiring in cmd/harbormaster) can supply a live
// admin-client adapter to NewClientGetter without leaking the unexported
// adminAPI shape into the surrounding code. The live *madmin.AdminClient
// satisfies this shape directly.
type AdminClient interface {
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	InfoCannedPolicy(ctx context.Context, policyName string) ([]byte, error)
	AddCannedPolicy(ctx context.Context, policyName string, policy []byte) error
	RemoveCannedPolicy(ctx context.Context, policyName string) error
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	ListGroups(ctx context.Context) ([]string, error)
	GetGroupDescription(ctx context.Context, group string) (*madmin.GroupDesc, error)
}

// ClientGetter is the concrete dependency the Processor pulls from on every
// call. The HTTP layer adapts internal/minio.Pool to this shape so the
// package never imports the live pool type; tests inject a getter that
// returns hand-rolled stubs satisfying adminAPI.
type ClientGetter func(ctx context.Context) (adminAPI, error)

// NewClientGetter adapts a resolver that yields the public AdminClient
// into a ClientGetter compatible with the unexported adminAPI interface
// used inside the package.
func NewClientGetter(resolve func(ctx context.Context) (AdminClient, error)) ClientGetter {
	return func(ctx context.Context) (adminAPI, error) {
		c, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// Processor is the policies-domain orchestrator. It depends only on the
// ClientGetter — there is no GORM DB here because the domain has no local
// persistence.
//
// Logger defaults to zerolog.Nop so unit tests need not configure it; the
// HTTP wire-up calls WithLogger to inject the real logger.
//
// Audit is the (optional) audit.Processor handle used to record mutations.
// nil disables audit emission — call WithAudit at the composition root to
// enable it.
type Processor struct {
	Clients ClientGetter
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

// NewProcessor returns a Processor bound to clients. The logger defaults to
// zerolog.Nop; use WithLogger to attach the real logger.
func NewProcessor(clients ClientGetter) *Processor {
	return &Processor{Clients: clients, Logger: zerolog.Nop()}
}

// WithLogger returns p with the supplied logger attached.
func (p *Processor) WithLogger(l zerolog.Logger) *Processor {
	p.Logger = l
	return p
}

// WithAudit returns p with the supplied audit processor attached.
func (p *Processor) WithAudit(a *audit.Processor) *Processor {
	p.Audit = a
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

// clients is a tiny indirection that wraps the ClientGetter's error in an
// apierror so callers can return it directly to the HTTP layer.
func (p *Processor) clients(ctx context.Context) (adminAPI, error) {
	if p.Clients == nil {
		return nil, apierror.Internal("policies: client getter not configured")
	}
	adm, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return adm, nil
}

// mapClientError wraps a raw MinIO SDK error into the apierror used by
// policy endpoints.
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

// policyFailAudit is a helper for CRUD failure paths. It records a failure
// audit event for the given action and returns err unchanged.
func (p *Processor) policyFailAudit(ctx context.Context, action, name, actor, sourceIP string, err error) error {
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     action,
		TargetType: "policy",
		TargetID:   name,
		Outcome:    audit.OutcomeFailure,
		ErrorMessage: err.Error(),
		PayloadSummary: map[string]any{
			"policy": name,
		},
	})
	return err
}

// validateForWrite validates name (when checkName=true) and doc for a
// Create or Update operation. It returns a typed apierror on validation
// failures.
func validateForWrite(name string, doc []byte, checkName bool) error {
	if checkName {
		if err := ValidatePolicyName(name); err != nil {
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_name",
				err.Error()).WithPointer("/data/attributes/name")
		}
		if IsBuiltin(name) || isTemplateName(name) {
			return apierror.New(http.StatusConflict, "policy_name_reserved",
				"policy name is reserved and cannot be used for custom policies")
		}
	}
	if err := ValidatePolicyDocument(doc); err != nil {
		switch {
		case IsInvalidJSON(err):
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_json",
				"policy document is not valid JSON").WithPointer("/data/attributes/document")
		case IsInvalidStructure(err):
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_structure",
				"policy document has invalid structure").WithPointer("/data/attributes/document")
		}
		return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_document", err.Error())
	}
	return nil
}

// List returns every canned policy on the configured MinIO, sorted by name.
func (p *Processor) List(ctx context.Context) ([]Policy, error) {
	adm, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	return listCanned(ctx, adm)
}

// Get returns the full policy detail (including document) for a single policy.
func (p *Processor) Get(ctx context.Context, name string) (PolicyDetail, error) {
	adm, err := p.clients(ctx)
	if err != nil {
		return PolicyDetail{}, err
	}
	raw, err := adm.InfoCannedPolicy(ctx, name)
	if err != nil {
		return PolicyDetail{}, mapClientError(err, "failed to get policy")
	}
	pol := policyFromEntry(name, raw)
	return PolicyDetail{Policy: pol, Document: raw}, nil
}

// Create validates name+doc, calls AddCannedPolicy, and emits a success
// audit event. Returns the new Policy on success.
//
// Guards (applied in order):
//  1. Name charset/length — 422 invalid_policy_name.
//  2. Reserved name (builtin or template-prefixed) — 409 policy_name_reserved.
//  3. Document JSON validity — 422 invalid_policy_json.
//  4. Document structure — 422 invalid_policy_structure.
//  5. MinIO rejection — 422 minio_rejected_policy.
func (p *Processor) Create(ctx context.Context, name string, doc []byte, actor, sourceIP string) (Policy, error) {
	failAudit := func(err error) error {
		return p.policyFailAudit(ctx, audit.ActionPolicyCreate, name, actor, sourceIP, err)
	}
	if err := validateForWrite(name, doc, true); err != nil {
		return Policy{}, failAudit(err)
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return Policy{}, failAudit(err)
	}
	if err := adm.AddCannedPolicy(ctx, name, doc); err != nil {
		return Policy{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"minio_rejected_policy", "MinIO rejected the policy: "+err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionPolicyCreate,
		TargetType: "policy",
		TargetID:   name,
		Outcome:    audit.OutcomeSuccess,
		PayloadSummary: map[string]any{
			"policy": name,
		},
	})
	return policyFromEntry(name, doc), nil
}

// Update upserts the canned policy document for an existing custom policy.
// Non-custom policies (builtins, templates) are rejected with 403
// policy_read_only BEFORE document validation.
//
// Guards:
//  1. !EditableFor(name) — 403 policy_read_only.
//  2. Document validation (JSON + structure).
//  3. AddCannedPolicy (upsert semantics).
func (p *Processor) Update(ctx context.Context, name string, doc []byte, actor, sourceIP string) error {
	failAudit := func(err error) error {
		return p.policyFailAudit(ctx, audit.ActionPolicyUpdate, name, actor, sourceIP, err)
	}
	if !EditableFor(name) {
		return failAudit(apierror.New(http.StatusForbidden, "policy_read_only",
			"policy is read-only and cannot be modified"))
	}
	if err := validateForWrite(name, doc, false); err != nil {
		return failAudit(err)
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := adm.AddCannedPolicy(ctx, name, doc); err != nil {
		return failAudit(apierror.New(http.StatusUnprocessableEntity,
			"minio_rejected_policy", "MinIO rejected the policy: "+err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionPolicyUpdate,
		TargetType: "policy",
		TargetID:   name,
		Outcome:    audit.OutcomeSuccess,
		PayloadSummary: map[string]any{
			"policy": name,
		},
	})
	return nil
}

// Delete removes a custom canned policy after verifying it is not attached
// to any user or group.
//
// Guards:
//  1. !EditableFor(name) — 403 policy_read_only.
//  2. attachmentScan — 409 policy_in_use (with Details.attached_to).
//  3. RemoveCannedPolicy.
func (p *Processor) Delete(ctx context.Context, name, actor, sourceIP string) error {
	failAudit := func(err error) error {
		return p.policyFailAudit(ctx, audit.ActionPolicyDelete, name, actor, sourceIP, err)
	}
	if !EditableFor(name) {
		return failAudit(apierror.New(http.StatusForbidden, "policy_read_only",
			"policy is read-only and cannot be deleted"))
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	users, groups, err := attachmentScan(ctx, adm, name)
	if err != nil {
		return failAudit(mapClientError(err, "failed to scan policy attachments"))
	}

	// Sort for deterministic output.
	sort.Strings(users)
	sort.Strings(groups)

	if len(users) > 0 || len(groups) > 0 {
		inUseErr := apierror.New(http.StatusConflict, "policy_in_use",
			"policy is still attached to one or more users or groups").
			WithDetails(map[string]any{
				"attached_to": map[string]any{
					"users":  users,
					"groups": groups,
				},
			})
		return failAudit(inUseErr)
	}

	if err := adm.RemoveCannedPolicy(ctx, name); err != nil {
		return failAudit(mapClientError(err, "failed to remove policy"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionPolicyDelete,
		TargetType: "policy",
		TargetID:   name,
		Outcome:    audit.OutcomeSuccess,
		PayloadSummary: map[string]any{
			"policy": name,
		},
	})
	return nil
}
