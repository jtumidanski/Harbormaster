package users

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/policies"
)

// White-box test package: the IAM-domain interfaces (adminAPI,
// ClientGetter) are intentionally unexported so callers outside the
// package have to go through the HTTP layer. Living in package users
// gives us direct access without leaking test-only constructors.

// stubAdmin is a hand-rolled adminAPI fake that also satisfies
// policies.PolicyAdmin (so it can be the Materializer's backing client)
// and saAdminAPI (so it backs the ServiceAccountProcessor too).
type stubAdmin struct {
	mu sync.Mutex

	users  map[string]madmin.UserInfo // access_key -> info (PolicyName is comma-separated)
	canned map[string]json.RawMessage // name -> raw policy doc (for ListCannedPolicies)

	addUserCalls       []addUserCall
	addUserErr         error
	setStatusCalls     []setStatusCall
	setStatusErr       error
	removeCalls        []string
	removeErr          error
	attachCalls        []madmin.PolicyAssociationReq
	attachErr          error
	detachCalls        []madmin.PolicyAssociationReq
	detachErr          error
	addCannedCalls     []addCannedCall
	addCannedErr       error
	listCannedErr      error
	addServiceCalls    []madmin.AddServiceAccountReq
	addServiceCreds    madmin.Credentials
	addServiceErr      error
	deleteServiceCalls []string
	deleteServiceErr   error
	listServiceResp    madmin.ListServiceAccountsResp
	listServiceErr     error
}

type addUserCall struct {
	AccessKey string
	SecretKey string
}

type setStatusCall struct {
	AccessKey string
	Status    madmin.AccountStatus
}

type addCannedCall struct {
	Name string
	Body []byte
}

func newStubAdmin() *stubAdmin {
	return &stubAdmin{
		users:  map[string]madmin.UserInfo{},
		canned: map[string]json.RawMessage{},
	}
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

func (s *stubAdmin) GetUserInfo(_ context.Context, name string) (madmin.UserInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.users[name]
	if !ok {
		return madmin.UserInfo{}, nil
	}
	return info, nil
}

func (s *stubAdmin) AddUser(_ context.Context, ak, sk string) error {
	if s.addUserErr != nil {
		return s.addUserErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addUserCalls = append(s.addUserCalls, addUserCall{AccessKey: ak, SecretKey: sk})
	s.users[ak] = madmin.UserInfo{Status: madmin.AccountEnabled}
	return nil
}

func (s *stubAdmin) SetUserStatus(_ context.Context, ak string, st madmin.AccountStatus) error {
	if s.setStatusErr != nil {
		return s.setStatusErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStatusCalls = append(s.setStatusCalls, setStatusCall{AccessKey: ak, Status: st})
	u := s.users[ak]
	u.Status = st
	s.users[ak] = u
	return nil
}

func (s *stubAdmin) RemoveUser(_ context.Context, ak string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeCalls = append(s.removeCalls, ak)
	delete(s.users, ak)
	return nil
}

func (s *stubAdmin) AttachPolicy(_ context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error) {
	if s.attachErr != nil {
		return madmin.PolicyAssociationResp{}, s.attachErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attachCalls = append(s.attachCalls, req)
	// Update the synthetic PolicyName so subsequent GetUserInfo reflects
	// the attachment. The real MinIO server does this on its own.
	if req.User != "" {
		u := s.users[req.User]
		existing := splitPolicyList(u.PolicyName)
		seen := map[string]struct{}{}
		for _, p := range existing {
			seen[p] = struct{}{}
		}
		for _, p := range req.Policies {
			if _, ok := seen[p]; ok {
				continue
			}
			existing = append(existing, p)
			seen[p] = struct{}{}
		}
		u.PolicyName = strings.Join(existing, ",")
		s.users[req.User] = u
	}
	return madmin.PolicyAssociationResp{PoliciesAttached: req.Policies}, nil
}

func (s *stubAdmin) DetachPolicy(_ context.Context, req madmin.PolicyAssociationReq) (madmin.PolicyAssociationResp, error) {
	if s.detachErr != nil {
		return madmin.PolicyAssociationResp{}, s.detachErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detachCalls = append(s.detachCalls, req)
	if req.User != "" {
		u := s.users[req.User]
		existing := splitPolicyList(u.PolicyName)
		remove := map[string]struct{}{}
		for _, p := range req.Policies {
			remove[p] = struct{}{}
		}
		kept := existing[:0]
		for _, p := range existing {
			if _, drop := remove[p]; drop {
				continue
			}
			kept = append(kept, p)
		}
		u.PolicyName = strings.Join(kept, ",")
		s.users[req.User] = u
	}
	return madmin.PolicyAssociationResp{PoliciesDetached: req.Policies}, nil
}

func (s *stubAdmin) AddCannedPolicy(_ context.Context, name string, body []byte) error {
	if s.addCannedErr != nil {
		return s.addCannedErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	s.addCannedCalls = append(s.addCannedCalls, addCannedCall{Name: name, Body: cp})
	return nil
}

func (s *stubAdmin) ListCannedPolicies(_ context.Context) (map[string]json.RawMessage, error) {
	if s.listCannedErr != nil {
		return nil, s.listCannedErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]json.RawMessage, len(s.canned))
	for k, v := range s.canned {
		out[k] = v
	}
	return out, nil
}

func (s *stubAdmin) InfoCannedPolicy(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.canned[name]
	if !ok {
		return nil, nil
	}
	return []byte(doc), nil
}

func (s *stubAdmin) ListServiceAccounts(_ context.Context, _ string) (madmin.ListServiceAccountsResp, error) {
	if s.listServiceErr != nil {
		return madmin.ListServiceAccountsResp{}, s.listServiceErr
	}
	return s.listServiceResp, nil
}

func (s *stubAdmin) InfoServiceAccount(_ context.Context, _ string) (madmin.InfoServiceAccountResp, error) {
	return madmin.InfoServiceAccountResp{}, nil
}

func (s *stubAdmin) AddServiceAccount(_ context.Context, opts madmin.AddServiceAccountReq) (madmin.Credentials, error) {
	if s.addServiceErr != nil {
		return madmin.Credentials{}, s.addServiceErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addServiceCalls = append(s.addServiceCalls, opts)
	if s.addServiceCreds.AccessKey == "" {
		return madmin.Credentials{AccessKey: "svc-" + opts.TargetUser, SecretKey: "stubbed-secret"}, nil
	}
	return s.addServiceCreds, nil
}

func (s *stubAdmin) DeleteServiceAccount(_ context.Context, ak string) error {
	if s.deleteServiceErr != nil {
		return s.deleteServiceErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteServiceCalls = append(s.deleteServiceCalls, ak)
	return nil
}

// newTestProcessor wires a Processor against an in-memory stub admin and
// a materializer pointing at the same stub (so policy upserts and
// user-level attaches all land on the same fake client).
func newTestProcessor(t *testing.T) (*Processor, *stubAdmin) {
	t.Helper()
	adm := newStubAdmin()
	mat := &policies.Materializer{
		Admin: func(_ context.Context) (policies.PolicyAdmin, error) { return adm, nil },
	}
	getter := func(_ context.Context) (adminAPI, error) { return adm, nil }
	return NewProcessor(getter, mat), adm
}

// newTestSAProcessor wires a ServiceAccountProcessor against the stub
// admin and a shared materializer.
func newTestSAProcessor(t *testing.T) (*ServiceAccountProcessor, *stubAdmin) {
	t.Helper()
	adm := newStubAdmin()
	mat := &policies.Materializer{
		Admin: func(_ context.Context) (policies.PolicyAdmin, error) { return adm, nil },
	}
	getter := func(_ context.Context) (saAdminAPI, error) { return adm, nil }
	return NewServiceAccountProcessor(getter, mat), adm
}

// newAuditDB opens an in-process SQLite database in a temp dir and runs
// all migrations.
func newAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "users_audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newAuditedProcessor returns a Processor + ServiceAccountProcessor pair
// backed by the same stub admin and a real audit.Processor sharing a
// fresh test DB.
func newAuditedProcessor(t *testing.T) (*Processor, *ServiceAccountProcessor, *audit.Processor, *stubAdmin) {
	t.Helper()
	adm := newStubAdmin()
	mat := &policies.Materializer{
		Admin: func(_ context.Context) (policies.PolicyAdmin, error) { return adm, nil },
	}
	getter := func(_ context.Context) (adminAPI, error) { return adm, nil }
	saGetter := func(_ context.Context) (saAdminAPI, error) { return adm, nil }
	gdb := newAuditDB(t)
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p := NewProcessor(getter, mat).WithAudit(a)
	sa := NewServiceAccountProcessor(saGetter, mat).WithAudit(a)
	return p, sa, a, adm
}

// loadLatestPayload returns the most-recently inserted audit Event for
// action together with the raw payload_summary_json column value.
func loadLatestPayload(t *testing.T, a *audit.Processor, action string) (audit.Event, string) {
	t.Helper()
	events, err := audit.List(a.DB(), audit.Filter{Action: action, PageSize: 1})
	require.NoError(t, err)
	if len(events) == 0 {
		return audit.Event{}, ""
	}
	type row struct {
		PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
	}
	var r row
	require.NoError(t,
		a.DB().
			Table("audit_events").
			Select("payload_summary_json").
			Where("id = ?", events[0].ID).
			Scan(&r).Error,
	)
	return events[0], r.PayloadSummaryJSON
}

// makeUserInfo builds a madmin.UserInfo with the given (comma-separated)
// policy list. Status defaults to AccountEnabled.
func makeUserInfo(policies string) madmin.UserInfo {
	return madmin.UserInfo{
		PolicyName: policies,
		Status:     madmin.AccountEnabled,
	}
}

// requireNoSecrets asserts payload never carries any sensitive token
// substring. Mirrors the bucket helper.
func requireNoSecrets(t *testing.T, payload string) {
	t.Helper()
	lower := strings.ToLower(payload)
	for _, banned := range []string{"password", "secret", "token", "signature", "presigned"} {
		require.NotContainsf(t, lower, banned,
			"payload leaked banned substring %q: %s", banned, payload)
	}
}
