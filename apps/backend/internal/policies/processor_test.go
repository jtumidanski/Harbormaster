package policies

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// ---------------------------------------------------------------------------
// stubAdmin — in-memory adminAPI implementation
// ---------------------------------------------------------------------------

type stubAdmin struct {
	mu sync.Mutex

	// policies maps name → raw document (like the MinIO server stores them).
	policies map[string]json.RawMessage

	// users maps access_key → UserInfo (PolicyName is comma-separated).
	users map[string]madmin.UserInfo

	// groups maps group name → GroupDesc.
	groups map[string]madmin.GroupDesc

	// call capture
	addCannedPolicyCalls    []policyCannedCall
	removeCannedPolicyCalls []string
	addCannedErr            error
	removeCannedErr         error
	listGroupsErr           error
}

// policyCannedCall records a single AddCannedPolicy invocation.
type policyCannedCall struct {
	Name string
	Body []byte
}

func newStubAdmin() *stubAdmin {
	return &stubAdmin{
		policies: map[string]json.RawMessage{},
		users:    map[string]madmin.UserInfo{},
		groups:   map[string]madmin.GroupDesc{},
	}
}

func (s *stubAdmin) ListCannedPolicies(_ context.Context) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]json.RawMessage, len(s.policies))
	for k, v := range s.policies {
		out[k] = v
	}
	return out, nil
}

func (s *stubAdmin) InfoCannedPolicy(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.policies[name]
	if !ok {
		return nil, errors.New("policy not found: " + name)
	}
	return []byte(doc), nil
}

func (s *stubAdmin) AddCannedPolicy(_ context.Context, name string, body []byte) error {
	if s.addCannedErr != nil {
		return s.addCannedErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	s.addCannedPolicyCalls = append(s.addCannedPolicyCalls, policyCannedCall{Name: name, Body: cp})
	s.policies[name] = cp
	return nil
}

func (s *stubAdmin) RemoveCannedPolicy(_ context.Context, name string) error {
	if s.removeCannedErr != nil {
		return s.removeCannedErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeCannedPolicyCalls = append(s.removeCannedPolicyCalls, name)
	delete(s.policies, name)
	return nil
}

func (s *stubAdmin) ListUsers(_ context.Context) (map[string]madmin.UserInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]madmin.UserInfo, len(s.users))
	for k, v := range s.users {
		out[k] = v
	}
	return out, nil
}

func (s *stubAdmin) ListGroups(_ context.Context) ([]string, error) {
	if s.listGroupsErr != nil {
		return nil, s.listGroupsErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.groups))
	for k := range s.groups {
		out = append(out, k)
	}
	return out, nil
}

func (s *stubAdmin) GetGroupDescription(_ context.Context, group string) (*madmin.GroupDesc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	desc, ok := s.groups[group]
	if !ok {
		return nil, errors.New("group not found: " + group)
	}
	return &desc, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertAPIError unwraps err to *apierror.Error and checks its HTTPStatus and Code.
func assertAPIError(t *testing.T, err error, wantStatus int, wantCode string) {
	t.Helper()
	require.Error(t, err)
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae), "expected *apierror.Error, got %T: %v", err, err)
	require.Equal(t, wantStatus, ae.HTTPStatus, "HTTPStatus mismatch for code %q", wantCode)
	require.Equal(t, wantCode, ae.Code, "code mismatch")
}

func validDoc() []byte {
	return []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
}

func newTestProcessor(t *testing.T) (*Processor, *stubAdmin) {
	t.Helper()
	adm := newStubAdmin()
	getter := func(_ context.Context) (adminAPI, error) { return adm, nil }
	return NewProcessor(getter), adm
}

func newAuditedProcessor(t *testing.T) (*Processor, *stubAdmin, *audit.Processor) {
	t.Helper()
	adm := newStubAdmin()
	getter := func(_ context.Context) (adminAPI, error) { return adm, nil }
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "policies_audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p := NewProcessor(getter).WithAudit(a)
	return p, adm, a
}

// ---------------------------------------------------------------------------
// Create guards
// ---------------------------------------------------------------------------

func TestCreateRejectsInvalidJSON(t *testing.T) {
	p, _ := newTestProcessor(t)
	_, err := p.Create(context.Background(), "my-policy", []byte(`not json`), "", "")
	assertAPIError(t, err, 422, "invalid_policy_json")
}

func TestCreateRejectsBadStructure(t *testing.T) {
	p, _ := newTestProcessor(t)
	// Valid JSON but missing Statement.
	_, err := p.Create(context.Background(), "my-policy", []byte(`{"Version":"2012-10-17"}`), "", "")
	assertAPIError(t, err, 422, "invalid_policy_structure")
}

func TestCreateRejectsBadName(t *testing.T) {
	p, _ := newTestProcessor(t)
	_, err := p.Create(context.Background(), "bad name!", validDoc(), "", "")
	assertAPIError(t, err, 422, "invalid_policy_name")
}

func TestCreateRejectsReservedName_Builtin(t *testing.T) {
	p, _ := newTestProcessor(t)
	_, err := p.Create(context.Background(), "readonly", validDoc(), "", "")
	assertAPIError(t, err, 409, "policy_name_reserved")
}

func TestCreateRejectsReservedName_Template(t *testing.T) {
	p, _ := newTestProcessor(t)
	_, err := p.Create(context.Background(), "harbormaster-custom", validDoc(), "", "")
	assertAPIError(t, err, 409, "policy_name_reserved")
}

func TestCreateAddsCannedPolicy(t *testing.T) {
	p, adm := newTestProcessor(t)
	policy, err := p.Create(context.Background(), "my-policy", validDoc(), "operator", "10.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "my-policy", policy.Name)
	require.Equal(t, OriginCustom, policy.Origin)
	require.True(t, policy.Editable)
	require.Len(t, adm.addCannedPolicyCalls, 1)
	require.Equal(t, "my-policy", adm.addCannedPolicyCalls[0].Name)
}

func TestCreateAudit(t *testing.T) {
	p, _, a := newAuditedProcessor(t)
	_, err := p.Create(context.Background(), "my-policy", validDoc(), "operator", "10.0.0.1")
	require.NoError(t, err)
	events, err := audit.List(a.DB(), audit.Filter{Action: audit.ActionPolicyCreate, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	require.Equal(t, "my-policy", events[0].TargetID)
}

// ---------------------------------------------------------------------------
// Update guards
// ---------------------------------------------------------------------------

func TestUpdateRejectsNonCustom_Builtin(t *testing.T) {
	p, _ := newTestProcessor(t)
	err := p.Update(context.Background(), "readonly", validDoc(), "", "")
	assertAPIError(t, err, 403, "policy_read_only")
}

func TestUpdateRejectsNonCustom_Template(t *testing.T) {
	p, _ := newTestProcessor(t)
	err := p.Update(context.Background(), "harbormaster-read-only", validDoc(), "", "")
	assertAPIError(t, err, 403, "policy_read_only")
}

func TestUpdateHappyPath(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()
	err := p.Update(context.Background(), "my-policy", validDoc(), "operator", "10.0.0.1")
	require.NoError(t, err)
	require.Len(t, adm.addCannedPolicyCalls, 1)
}

// ---------------------------------------------------------------------------
// Delete guards
// ---------------------------------------------------------------------------

func TestDeleteRejectsNonCustom(t *testing.T) {
	p, _ := newTestProcessor(t)
	err := p.Delete(context.Background(), "readonly", "", "")
	assertAPIError(t, err, 403, "policy_read_only")
}

func TestDeleteRejectsInUse(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()
	adm.users["alice"] = madmin.UserInfo{PolicyName: "my-policy"}

	err := p.Delete(context.Background(), "my-policy", "", "")
	assertAPIError(t, err, 409, "policy_in_use")

	// Details must carry attached_to with the user.
	var ae *apierror.Error
	require.True(t, errors.As(err, &ae))
	require.NotNil(t, ae.Details)
	attachedTo, ok := ae.Details["attached_to"].(map[string]any)
	require.True(t, ok, "expected attached_to to be map[string]any")
	usersSlice, ok := attachedTo["users"].([]string)
	require.True(t, ok, "expected users key to be []string")
	require.Contains(t, usersSlice, "alice")
}

func TestDeleteRemovesUnusedCustom(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()

	err := p.Delete(context.Background(), "my-policy", "", "")
	require.NoError(t, err)
	require.Equal(t, []string{"my-policy"}, adm.removeCannedPolicyCalls)
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGetReturnsDocument(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = json.RawMessage(validDoc())

	detail, err := p.Get(context.Background(), "my-policy")
	require.NoError(t, err)
	require.Equal(t, "my-policy", detail.Name)
	require.JSONEq(t, string(validDoc()), string(detail.Document))
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestListReturnsAllPolicies(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["readonly"] = json.RawMessage(validDoc())
	adm.policies["my-policy"] = json.RawMessage(validDoc())

	ps, err := p.List(context.Background())
	require.NoError(t, err)
	require.Len(t, ps, 2)
	// Results are sorted by name.
	require.Equal(t, "my-policy", ps[0].Name)
	require.Equal(t, "readonly", ps[1].Name)
}
