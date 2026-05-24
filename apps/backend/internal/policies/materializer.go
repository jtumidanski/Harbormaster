package policies

import (
	"context"
	"errors"
)

// ErrUnknownTemplate is returned by Materializer.EnsurePolicy when the
// caller names a template that is not one of the bundled set.
var ErrUnknownTemplate = errors.New("policies: unknown template")

// PolicyAdmin is the narrow contract the materializer requires of MinIO's
// admin client. Defining it as a local interface lets the users package
// reuse the same shape for its IAM mocks without re-importing madmin.
type PolicyAdmin interface {
	AddCannedPolicy(ctx context.Context, name string, body []byte) error
}

// Materializer ensures a Harbormaster-managed policy exists on MinIO with
// the expected canonical content, creating or overwriting as needed.
//
// Admin is invoked for every EnsurePolicy call so callers can supply a
// per-request admin client (e.g. one drawn from the live MinIO pool). The
// indirection also keeps the package independent of any concrete madmin
// client type — tests pass a stub implementing PolicyAdmin.
type Materializer struct {
	Admin func(ctx context.Context) (PolicyAdmin, error)
}

// EnsurePolicy creates or overwrites the canned policy keyed by
// (template, params) and returns its canonical Harbormaster name. The
// operation is idempotent: MinIO's AddCannedPolicy upserts, so two calls
// with the same (template, params) leave exactly one policy named via
// MaterializedName on the cluster.
//
// Errors:
//   - ErrUnknownTemplate when template does not match a bundled name.
//   - Whatever the template Render emits (e.g. missing required params).
//   - Whatever the admin client returns from AddCannedPolicy.
func (m *Materializer) EnsurePolicy(ctx context.Context, template string, params map[string]string) (string, error) {
	t, ok := Find(template)
	if !ok {
		return "", ErrUnknownTemplate
	}
	body, err := t.Render(params)
	if err != nil {
		return "", err
	}
	name := MaterializedName(template, params)
	admin, err := m.Admin(ctx)
	if err != nil {
		return "", err
	}
	if err := admin.AddCannedPolicy(ctx, name, []byte(body)); err != nil {
		return "", err
	}
	return name, nil
}
