package users

import (
	"context"
	"errors"
	"net/http"
	"sort"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// adminAPI is the subset of *madmin.AdminClient the users processor uses.
// Defining it as a local interface lets tests substitute an in-memory stub
// without standing up a fake MinIO server. The shape mirrors madmin v3's
// signatures verbatim so the live client satisfies the interface by
// structural typing.
type adminAPI interface {
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	GetUserInfo(ctx context.Context, name string) (madmin.UserInfo, error)
	AddUser(ctx context.Context, accessKey, secretKey string) error
	SetUserStatus(ctx context.Context, accessKey string, status madmin.AccountStatus) error
	RemoveUser(ctx context.Context, accessKey string) error
	AttachPolicy(ctx context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error)
	DetachPolicy(ctx context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error)
	AddCannedPolicy(ctx context.Context, name string, body []byte) error
}

// AdminClient is the public face of adminAPI. It exists so callers outside
// the package (the HTTP wiring in cmd/harbormaster) can supply a live
// admin-client adapter to NewClientGetter without leaking the unexported
// adminAPI shape into the surrounding code. The live *madmin.AdminClient
// satisfies this shape directly.
type AdminClient interface {
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	GetUserInfo(ctx context.Context, name string) (madmin.UserInfo, error)
	AddUser(ctx context.Context, accessKey, secretKey string) error
	SetUserStatus(ctx context.Context, accessKey string, status madmin.AccountStatus) error
	RemoveUser(ctx context.Context, accessKey string) error
	AttachPolicy(ctx context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error)
	DetachPolicy(ctx context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error)
	AddCannedPolicy(ctx context.Context, name string, body []byte) error
}

// ClientGetter is the concrete dependency the Processor pulls from on
// every call. The HTTP layer adapts internal/minio.Pool to this shape so
// the package never imports the live pool type; tests inject a getter
// that returns hand-rolled stubs satisfying adminAPI.
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

// Processor is the users-domain orchestrator. It depends only on the
// ClientGetter and the policy materializer — there is no GORM DB here
// because the domain has no local persistence.
//
// Logger is used to surface best-effort sub-fetch failures (per-user
// GetUserInfo) that intentionally do not fail the parent list call. The
// default value is zerolog.Nop so unit tests need not configure it; the
// HTTP wire-up calls WithLogger to inject the real logger.
//
// Audit is the (optional) audit.Processor handle used to record user
// mutations. nil disables audit emission — call WithAudit at the
// composition root to enable it.
//
// Mat is the policy materializer used by Create / UpdatePolicies to
// ensure the canonical Harbormaster policy exists on MinIO before
// attaching it to the user.
type Processor struct {
	Clients ClientGetter
	Mat     *policies.Materializer
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

// NewProcessor returns a Processor bound to clients and the policy
// materializer. The logger defaults to zerolog.Nop; use WithLogger to
// attach the real logger.
func NewProcessor(clients ClientGetter, mat *policies.Materializer) *Processor {
	return &Processor{Clients: clients, Mat: mat, Logger: zerolog.Nop()}
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

// List returns every IAM user on the configured MinIO with attached
// policies classified into managed (TemplateRef) and other slots. The
// per-user GetUserInfo call is required because ListUsers returns the
// policy attachments inline only for some MinIO versions; the explicit
// call keeps behaviour consistent across server builds.
//
// The result is sorted by AccessKey so the UI gets a stable order.
func (p *Processor) List(ctx context.Context) ([]User, error) {
	adm, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	rawUsers, err := adm.ListUsers(ctx)
	if err != nil {
		return nil, mapClientError(err, "failed to list users")
	}
	out := make([]User, 0, len(rawUsers))
	for ak, info := range rawUsers {
		// Prefer the GetUserInfo result so we get the freshest policy
		// list. If GetUserInfo errors, fall back to the ListUsers row.
		fresh, err := adm.GetUserInfo(ctx, ak)
		if err != nil {
			p.Logger.Warn().
				Err(err).
				Str("access_key", ak).
				Msg("users: GetUserInfo failed; falling back to ListUsers row")
			fresh = info
		}
		managed, other := classifyPolicies(splitPolicyList(fresh.PolicyName))
		out = append(out, User{
			AccessKey:         ak,
			Status:            string(fresh.Status),
			AttachedTemplates: managed,
			OtherPolicies:     other,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccessKey < out[j].AccessKey })
	return out, nil
}

// Get returns the single-user view.
func (p *Processor) Get(ctx context.Context, accessKey string) (User, error) {
	if err := ValidateAccessKey(accessKey); err != nil {
		return User{}, apierror.New(http.StatusBadRequest, "invalid_access_key", err.Error())
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return User{}, err
	}
	info, err := adm.GetUserInfo(ctx, accessKey)
	if err != nil {
		return User{}, mapClientError(err, "failed to get user")
	}
	managed, other := classifyPolicies(splitPolicyList(info.PolicyName))
	return User{
		AccessKey:         accessKey,
		Status:            string(info.Status),
		AttachedTemplates: managed,
		OtherPolicies:     other,
	}, nil
}

// Create generates a fresh secret, registers the user with MinIO, and
// attaches every requested template after materialising its canonical
// policy. The returned secret is shown to the operator exactly once; the
// caller's responsibility is to render it on the 201 response and never
// retain it server-side.
//
// Partial-failure semantics: if AddUser succeeds but a template
// materialisation or attachment fails, the user is left registered on
// MinIO with whatever subset of templates were successfully attached
// before the error. The operator can re-attach via the
// UpdatePolicies endpoint, which is the same idempotent path.
func (p *Processor) Create(ctx context.Context, accessKey string, templates []TemplateRef, actor, sourceIP string) (User, string, error) {
	auditPayload := map[string]any{
		"access_key": accessKey,
		"templates":  templateNames(templates),
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionUserCreate,
			TargetType:     "user",
			TargetID:       accessKey,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: auditPayload,
		})
		return err
	}
	if err := ValidateAccessKey(accessKey); err != nil {
		return User{}, "", failAudit(apierror.New(http.StatusUnprocessableEntity, "invalid_access_key", err.Error()))
	}
	// Up-front template validation: reject unknown / bad-param templates
	// before AddUser so a typo cannot leak an orphaned user.
	for _, ref := range templates {
		t, ok := policies.Find(ref.Name)
		if !ok {
			return User{}, "", failAudit(apierror.New(http.StatusUnprocessableEntity, "unknown_template",
				"unknown policy template: "+ref.Name))
		}
		if _, err := t.Render(ref.Params); err != nil {
			return User{}, "", failAudit(apierror.New(http.StatusUnprocessableEntity, "invalid_template_params", err.Error()))
		}
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return User{}, "", failAudit(err)
	}
	secret, err := GenerateSecret()
	if err != nil {
		return User{}, "", failAudit(apierror.Internal("failed to generate secret: " + err.Error()))
	}
	if err := adm.AddUser(ctx, accessKey, secret); err != nil {
		return User{}, "", failAudit(mapClientError(err, "failed to add user"))
	}
	// Materialize + attach each template. The Materializer.Admin function
	// is wired at composition time; here we just call EnsurePolicy.
	for _, ref := range templates {
		name, err := p.Mat.EnsurePolicy(ctx, ref.Name, ref.Params)
		if err != nil {
			return User{}, "", failAudit(apierror.Internal("failed to materialize policy: " + err.Error()))
		}
		if _, err := adm.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{name},
			User:     accessKey,
		}); err != nil {
			return User{}, "", failAudit(mapClientError(err, "failed to attach policy"))
		}
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionUserCreate,
		TargetType:     "user",
		TargetID:       accessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	u, err := p.Get(ctx, accessKey)
	if err != nil {
		// The user exists; surface the on-cluster facts even if the
		// post-create read failed (e.g. transient admin RPC hiccup).
		return User{AccessKey: accessKey, Status: "enabled", AttachedTemplates: templates}, secret, nil
	}
	return u, secret, nil
}

// SetStatus flips a user's account between "enabled" and "disabled". The
// audit action constants discriminate so an audit reader can grep for
// "user.enable" / "user.disable" without parsing payloads.
func (p *Processor) SetStatus(ctx context.Context, accessKey string, enabled bool, actor, sourceIP string) error {
	action := audit.ActionUserDisable
	if enabled {
		action = audit.ActionUserEnable
	}
	auditPayload := map[string]any{"access_key": accessKey, "enabled": enabled}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         action,
			TargetType:     "user",
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
	status := madmin.AccountDisabled
	if enabled {
		status = madmin.AccountEnabled
	}
	if err := adm.SetUserStatus(ctx, accessKey, status); err != nil {
		return failAudit(mapClientError(err, "failed to set user status"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         action,
		TargetType:     "user",
		TargetID:       accessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// Delete removes a user after the operator confirms by re-typing the
// access key. The confirm-key guard mirrors the bucket delete path —
// preventing fat-finger deletions is the whole point.
func (p *Processor) Delete(ctx context.Context, accessKey, confirmKey, actor, sourceIP string) error {
	auditPayload := map[string]any{"access_key": accessKey}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionUserDelete,
			TargetType:     "user",
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
	if accessKey != confirmKey {
		return failAudit(apierror.New(http.StatusForbidden, "confirm_name_mismatch",
			"confirm_access_key must equal the access key"))
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := adm.RemoveUser(ctx, accessKey); err != nil {
		return failAudit(mapClientError(err, "failed to remove user"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionUserDelete,
		TargetType:     "user",
		TargetID:       accessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// UpdatePolicies diffs the user's current Harbormaster-managed templates
// against the requested set, detaching removed and attaching added ones.
// Unchanged attachments are left alone so the call is cheap when nothing
// actually changed.
//
// OtherPolicies (operator-installed names like consoleAdmin) are left
// untouched — Harbormaster never detaches a policy it did not attach.
func (p *Processor) UpdatePolicies(ctx context.Context, accessKey string, requested []TemplateRef, actor, sourceIP string) error {
	auditPayload := map[string]any{
		"access_key": accessKey,
		"templates":  templateNames(requested),
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionUserPoliciesUpdate,
			TargetType:     "user",
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
	// Up-front validation of every requested template.
	for _, ref := range requested {
		t, ok := policies.Find(ref.Name)
		if !ok {
			return failAudit(apierror.New(http.StatusUnprocessableEntity, "unknown_template",
				"unknown policy template: "+ref.Name))
		}
		if _, err := t.Render(ref.Params); err != nil {
			return failAudit(apierror.New(http.StatusUnprocessableEntity, "invalid_template_params", err.Error()))
		}
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	info, err := adm.GetUserInfo(ctx, accessKey)
	if err != nil {
		return failAudit(mapClientError(err, "failed to read user info"))
	}
	currentManaged, _ := classifyPolicies(splitPolicyList(info.PolicyName))

	// Build canonical-name sets for the diff.
	currentSet := map[string]TemplateRef{}
	for _, ref := range currentManaged {
		currentSet[templateKey(ref)] = ref
	}
	requestedSet := map[string]TemplateRef{}
	for _, ref := range requested {
		requestedSet[templateKey(ref)] = ref
	}

	// Detach removed.
	for key := range currentSet {
		if _, keep := requestedSet[key]; keep {
			continue
		}
		if _, err := adm.DetachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{key},
			User:     accessKey,
		}); err != nil {
			return failAudit(mapClientError(err, "failed to detach policy"))
		}
	}
	// Attach added (materialize first so the canonical policy exists).
	for key, ref := range requestedSet {
		if _, have := currentSet[key]; have {
			continue
		}
		name, err := p.Mat.EnsurePolicy(ctx, ref.Name, ref.Params)
		if err != nil {
			return failAudit(apierror.Internal("failed to materialize policy: " + err.Error()))
		}
		if _, err := adm.AttachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{name},
			User:     accessKey,
		}); err != nil {
			return failAudit(mapClientError(err, "failed to attach policy"))
		}
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionUserPoliciesUpdate,
		TargetType:     "user",
		TargetID:       accessKey,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: auditPayload,
	})
	return nil
}

// clients is a tiny indirection that wraps the ClientGetter's error in an
// apierror so callers can return it directly to the HTTP layer.
func (p *Processor) clients(ctx context.Context) (adminAPI, error) {
	if p.Clients == nil {
		return nil, apierror.Internal("users: client getter not configured")
	}
	adm, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return adm, nil
}

// mapClientError wraps a raw MinIO SDK error into the action-style
// apierror used by user endpoints.
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

// templateNames extracts the canonical names from a TemplateRef slice for
// audit payloads. Names only — params (which may carry bucket identifiers
// that aren't strictly secret but are noisier than the audit log needs)
// are included separately by callers when relevant.
func templateNames(refs []TemplateRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}
