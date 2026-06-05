package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/require"
)

// newRouter wires the package's Routes onto a fresh chi.Mux. Used by the
// resource tests to exercise the full handler chain through ServeHTTP.
func newRouter(p *Processor, sa *ServiceAccountProcessor) *chi.Mux {
	r := chi.NewRouter()
	Routes(p, sa)(r)
	return r
}

// TestListUsersEnvelope — GET /users returns a JSON:API collection
// document and never leaks secret_key on the wire.
func TestListUsersEnvelope(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("harbormaster-read-only,consoleAdmin")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/vnd.api+json", rec.Header().Get("Content-Type"))

	var doc struct {
		Data []struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Len(t, doc.Data, 1)
	require.Equal(t, "users", doc.Data[0].Type)
	require.Equal(t, "alice", doc.Data[0].ID)
	require.NotContains(t, strings.ToLower(string(doc.Data[0].Attributes)), "secret")
}

// TestCreateUserRevealsSecretOnce — POST /users 201 carries the
// secret_key on the response payload. List output for the same user must
// not include it.
func TestCreateUserRevealsSecretOnce(t *testing.T) {
	p, _ := newTestProcessor(t)

	body := bytes.NewBufferString(`{
		"data":{"type":"users","attributes":{"access_key":"alice","templates":[{"name":"read-only"}]}}
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var doc struct {
		Data struct {
			Attributes struct {
				SecretKey string `json:"secret_key"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Len(t, doc.Data.Attributes.SecretKey, 40)
}

// TestDeleteUserConfirmMismatch — wrong confirm_access_key returns the
// action-envelope 403 with code confirm_name_mismatch.
func TestDeleteUserConfirmMismatch(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = madmin.UserInfo{Status: madmin.AccountEnabled}

	body := bytes.NewBufferString(`{"confirm_access_key":"wrong"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/users/alice", body)
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "confirm_name_mismatch", doc.Error.Code)
}

// TestPolicyTemplatesEndpoint — the three bundled templates are
// returned in a JSON:API collection with the params schema attached for
// backup-target.
func TestPolicyTemplatesEndpoint(t *testing.T) {
	p, _ := newTestProcessor(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/policy-templates", nil)
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var doc struct {
		Data []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Name         string          `json:"name"`
				Description  string          `json:"description"`
				ParamsSchema json.RawMessage `json:"params_schema,omitempty"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Len(t, doc.Data, 3)
	names := map[string]json.RawMessage{}
	for _, d := range doc.Data {
		require.Equal(t, "policy_templates", d.Type)
		names[d.Attributes.Name] = d.Attributes.ParamsSchema
	}
	require.Contains(t, names, "read-only")
	require.Contains(t, names, "read-write")
	require.Contains(t, names, "backup-target")
	require.NotEmpty(t, names["backup-target"])
}

// TestServiceAccountCreateRevealsSecretOnce — the nested
// POST /users/{ak}/service-accounts response includes secret_key.
func TestServiceAccountCreateRevealsSecretOnce(t *testing.T) {
	p, _ := newTestProcessor(t)
	sa, adm := newTestSAProcessor(t)
	adm.addServiceCreds = madmin.Credentials{AccessKey: "svc1", SecretKey: "secret-xyz-123"}

	body := bytes.NewBufferString(`{"data":{"type":"service_accounts","attributes":{"name":"ci","description":"d"}}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/alice/service-accounts", body)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	newRouter(p, sa).ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var doc struct {
		Data struct {
			Attributes struct {
				SecretKey string `json:"secret_key"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "secret-xyz-123", doc.Data.Attributes.SecretKey)
}

// TestServiceAccountRevokeRoute — DELETE /service-accounts/{ak} returns
// 204 on success.
func TestServiceAccountRevokeRoute(t *testing.T) {
	p, _ := newTestProcessor(t)
	sa, _ := newTestSAProcessor(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/service-accounts/svc1", nil)
	newRouter(p, sa).ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

// TestUpdatePoliciesAcceptsCustomPolicy — PUT /users/{ak}/policies with
// policies:["proj-a"] where "proj-a" is a known custom deployment policy
// returns 204 and the stub records an AttachPolicy call for "proj-a".
func TestUpdatePoliciesAcceptsCustomPolicy(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")
	adm.canned["proj-a"] = json.RawMessage(`{}`) // custom origin (no "harbormaster-" prefix)

	body := bytes.NewBufferString(`{"policies":["proj-a"]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/users/alice/policies", body)
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	found := false
	for _, c := range adm.attachCalls {
		for _, pol := range c.Policies {
			if pol == "proj-a" {
				found = true
			}
		}
	}
	require.True(t, found, "expected AttachPolicy call for proj-a, got: %v", adm.attachCalls)
}

// TestUpdatePoliciesRejectsUnknownPolicy — PUT /users/{ak}/policies with an
// unknown custom policy returns 422 with code "unknown_policy".
func TestUpdatePoliciesRejectsUnknownPolicy(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.users["alice"] = makeUserInfo("")
	// "nope" is NOT in adm.canned — the deployment doesn't know it.

	body := bytes.NewBufferString(`{"policies":["nope"]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/users/alice/policies", body)
	newRouter(p, nil).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "unknown_policy", doc.Error.Code)
}
