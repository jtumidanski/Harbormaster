package users

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// ServiceAccount describes a child credential belonging to a parent IAM
// user. A service account inherits its parent's authority unless the
// optional AttachedTemplate narrows it (the override is materialised on
// MinIO via the same policy templates the parent user uses).
//
// The secret key is intentionally absent — it is shown to the operator
// exactly once at creation time and never retrievable thereafter.
type ServiceAccount struct {
	AccessKey        string
	ParentUser       string
	Name             string
	Description      string
	Status           string
	AttachedTemplate *TemplateRef // optional policy override
}

// saAdminAPI is the subset of *madmin.AdminClient the service-account
// processor uses. The shape mirrors madmin v3 verbatim.
type saAdminAPI interface {
	ListServiceAccounts(ctx context.Context, user string) (madmin.ListServiceAccountsResp, error)
	InfoServiceAccount(ctx context.Context, accessKey string) (madmin.InfoServiceAccountResp, error)
	AddServiceAccount(ctx context.Context, opts madmin.AddServiceAccountReq) (madmin.Credentials, error)
	DeleteServiceAccount(ctx context.Context, accessKey string) error
}

// SAAdminClient is the public face of saAdminAPI; the live
// *madmin.AdminClient satisfies it directly.
type SAAdminClient interface {
	ListServiceAccounts(ctx context.Context, user string) (madmin.ListServiceAccountsResp, error)
	InfoServiceAccount(ctx context.Context, accessKey string) (madmin.InfoServiceAccountResp, error)
	AddServiceAccount(ctx context.Context, opts madmin.AddServiceAccountReq) (madmin.Credentials, error)
	DeleteServiceAccount(ctx context.Context, accessKey string) error
}

// SAClientGetter resolves the admin client used by the service-account
// processor on every call.
type SAClientGetter func(ctx context.Context) (saAdminAPI, error)

// NewSAClientGetter adapts a resolver that yields the public
// SAAdminClient into a SAClientGetter compatible with the unexported
// saAdminAPI interface used inside the package.
func NewSAClientGetter(resolve func(ctx context.Context) (SAAdminClient, error)) SAClientGetter {
	return func(ctx context.Context) (saAdminAPI, error) {
		c, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// ServiceAccountProcessor is the orchestrator for child-credential
// operations on a parent user.
//
// Mat is the policy materializer used by Create when an override template
// is supplied. The materialized policy is passed as the inline policy
// body on AddServiceAccount — MinIO interprets it as the service
// account's effective policy (intersected with the parent user's).
type ServiceAccountProcessor struct {
	Clients SAClientGetter
	Mat     *policies.Materializer
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

// NewServiceAccountProcessor returns a processor bound to clients and the
// policy materializer.
func NewServiceAccountProcessor(clients SAClientGetter, mat *policies.Materializer) *ServiceAccountProcessor {
	return &ServiceAccountProcessor{Clients: clients, Mat: mat, Logger: zerolog.Nop()}
}

// WithLogger returns p with the supplied logger attached.
func (p *ServiceAccountProcessor) WithLogger(l zerolog.Logger) *ServiceAccountProcessor {
	p.Logger = l
	return p
}

// WithAudit returns p with the supplied audit processor attached.
func (p *ServiceAccountProcessor) WithAudit(a *audit.Processor) *ServiceAccountProcessor {
	p.Audit = a
	return p
}

func (p *ServiceAccountProcessor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

func (p *ServiceAccountProcessor) clients(ctx context.Context) (saAdminAPI, error) {
	if p.Clients == nil {
		return nil, apierror.Internal("users: SA client getter not configured")
	}
	adm, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return adm, nil
}

// List returns every service account belonging to parent. The result is
// sorted by AccessKey for stable UI ordering.
func (p *ServiceAccountProcessor) List(ctx context.Context, parent string) ([]ServiceAccount, error) {
	if err := ValidateAccessKey(parent); err != nil {
		return nil, apierror.New(http.StatusBadRequest, "invalid_access_key", err.Error())
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := adm.ListServiceAccounts(ctx, parent)
	if err != nil {
		return nil, mapClientError(err, "failed to list service accounts")
	}
	out := make([]ServiceAccount, 0, len(resp.Accounts))
	for _, info := range resp.Accounts {
		out = append(out, ServiceAccount{
			AccessKey:   info.AccessKey,
			ParentUser:  info.ParentUser,
			Name:        info.Name,
			Description: info.Description,
			Status:      info.AccountStatus,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccessKey < out[j].AccessKey })
	return out, nil
}

// Create mints a new service account under parent. When override is
// non-nil the bundled template is materialised via the policy
// materializer and the rendered JSON is supplied as AddServiceAccount's
// inline policy. The returned secret is shown to the operator exactly
// once.
func (p *ServiceAccountProcessor) Create(ctx context.Context, parent, name, description string, override *TemplateRef, actor, sourceIP string) (ServiceAccount, string, error) {
	auditPayload := map[string]any{
		"parent_user": parent,
		"name":        name,
	}
	if override != nil {
		auditPayload["template"] = override.Name
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionServiceAccountCreate,
			TargetType:     "service_account",
			TargetID:       parent,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateAccessKey(parent); err != nil {
		return ServiceAccount{}, "", failAudit(apierror.New(http.StatusBadRequest, "invalid_access_key", err.Error()))
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return ServiceAccount{}, "", failAudit(err)
	}
	req := madmin.AddServiceAccountReq{
		TargetUser:  parent,
		Name:        name,
		Description: description,
	}
	if override != nil {
		// Materialise the canonical policy on MinIO (so the upserted
		// named policy exists for any later attach/list call), then
		// also send the rendered body inline so MinIO scopes this
		// service account to it immediately.
		if _, err := p.Mat.EnsurePolicy(ctx, override.Name, override.Params); err != nil {
			return ServiceAccount{}, "", failAudit(apierror.Internal("failed to materialize override policy: " + err.Error()))
		}
		t, ok := policies.Find(override.Name)
		if !ok {
			return ServiceAccount{}, "", failAudit(apierror.New(http.StatusUnprocessableEntity, "unknown_template",
				"unknown policy template: "+override.Name))
		}
		body, err := t.Render(override.Params)
		if err != nil {
			return ServiceAccount{}, "", failAudit(apierror.New(http.StatusUnprocessableEntity, "invalid_template_params", err.Error()))
		}
		req.Policy = json.RawMessage(body)
	}
	creds, err := adm.AddServiceAccount(ctx, req)
	if err != nil {
		return ServiceAccount{}, "", failAudit(mapClientError(err, "failed to add service account"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionServiceAccountCreate,
		TargetType:     "service_account",
		TargetID:       creds.AccessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	sa := ServiceAccount{
		AccessKey:        creds.AccessKey,
		ParentUser:       parent,
		Name:             name,
		Description:      description,
		Status:           "enabled",
		AttachedTemplate: override,
	}
	return sa, creds.SecretKey, nil
}

// Revoke deletes a service account.
func (p *ServiceAccountProcessor) Revoke(ctx context.Context, accessKey, actor, sourceIP string) error {
	auditPayload := map[string]any{"access_key": accessKey}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionServiceAccountRevoke,
			TargetType:     "service_account",
			TargetID:       accessKey,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateAccessKey(accessKey); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "invalid_access_key", err.Error()))
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := adm.DeleteServiceAccount(ctx, accessKey); err != nil {
		return failAudit(mapClientError(err, "failed to revoke service account"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionServiceAccountRevoke,
		TargetType:     "service_account",
		TargetID:       accessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// ServiceAccountResource is the JSON:API resource wrapper for a
// ServiceAccount. The Resource type is "service-accounts".
type ServiceAccountResource struct {
	ServiceAccount
}

// ResourceType returns the canonical JSON:API type string.
func (r ServiceAccountResource) ResourceType() string { return "service_accounts" }

// ResourceID returns the access key.
func (r ServiceAccountResource) ResourceID() string { return r.AccessKey }

// MarshalJSON shapes the on-the-wire payload. The secret_key is omitted
// (not present in the read view) — only the CreatedServiceAccountResource
// includes it on the 201 response.
func (r ServiceAccountResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		AccessKey        string        `json:"access_key"`
		ParentUser       string        `json:"parent_user"`
		Name             string        `json:"name,omitempty"`
		Description      string        `json:"description,omitempty"`
		Status           string        `json:"status,omitempty"`
		AttachedTemplate *TemplateWire `json:"attached_template,omitempty"`
	}
	out := alias{
		AccessKey:   r.AccessKey,
		ParentUser:  r.ParentUser,
		Name:        r.Name,
		Description: r.Description,
		Status:      r.Status,
	}
	if r.AttachedTemplate != nil {
		out.AttachedTemplate = &TemplateWire{
			Name:   r.AttachedTemplate.Name,
			Params: r.AttachedTemplate.Params,
		}
	}
	return json.Marshal(out)
}

// CreatedServiceAccountResource is the one-time-secret-reveal wrapper for
// the POST /users/{access_key}/service-accounts response.
type CreatedServiceAccountResource struct {
	ServiceAccount ServiceAccount
	SecretKey      string
}

// ResourceType returns the canonical JSON:API type string.
func (r CreatedServiceAccountResource) ResourceType() string { return "service_accounts" }

// ResourceID returns the access key.
func (r CreatedServiceAccountResource) ResourceID() string { return r.ServiceAccount.AccessKey }

// MarshalJSON shapes the on-the-wire payload for a freshly created
// service account, including the one-time secret_key.
func (r CreatedServiceAccountResource) MarshalJSON() ([]byte, error) {
	type alias struct {
		AccessKey        string        `json:"access_key"`
		ParentUser       string        `json:"parent_user"`
		Name             string        `json:"name,omitempty"`
		Description      string        `json:"description,omitempty"`
		Status           string        `json:"status,omitempty"`
		AttachedTemplate *TemplateWire `json:"attached_template,omitempty"`
		SecretKey        string        `json:"secret_key"`
	}
	out := alias{
		AccessKey:   r.ServiceAccount.AccessKey,
		ParentUser:  r.ServiceAccount.ParentUser,
		Name:        r.ServiceAccount.Name,
		Description: r.ServiceAccount.Description,
		Status:      r.ServiceAccount.Status,
		SecretKey:   r.SecretKey,
	}
	if r.ServiceAccount.AttachedTemplate != nil {
		out.AttachedTemplate = &TemplateWire{
			Name:   r.ServiceAccount.AttachedTemplate.Name,
			Params: r.ServiceAccount.AttachedTemplate.Params,
		}
	}
	return json.Marshal(out)
}

// CreateServiceAccountRequest is the attributes payload for
// POST /users/{access_key}/service-accounts.
type CreateServiceAccountRequest struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	// TemplateOverride scopes the child credential to a policy template.
	// The wire key is `template_override` per api-contracts.md and the SPA;
	// it was previously decoded as `template`, so the operator's selection
	// was silently dropped and every service account inherited the parent.
	TemplateOverride *TemplateWire `json:"template_override,omitempty"`
}

// Override converts the wire DTO into the domain pointer, returning nil
// when no template override was supplied.
func (r CreateServiceAccountRequest) Override() *TemplateRef {
	if r.TemplateOverride == nil {
		return nil
	}
	return &TemplateRef{Name: r.TemplateOverride.Name, Params: r.TemplateOverride.Params}
}
