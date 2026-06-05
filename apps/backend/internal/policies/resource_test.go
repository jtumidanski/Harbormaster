package policies

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// newPolicyRouter wires the package's Routes onto a fresh chi.Mux. Used by
// the resource tests to exercise the full handler chain through ServeHTTP.
func newPolicyRouter(p *Processor) *chi.Mux {
	r := chi.NewRouter()
	Routes(p)(r)
	return r
}

// TestListPoliciesEnvelope — GET /policies returns a JSON:API collection
// document with the correct type for each entry.
func TestListPoliciesEnvelope(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()
	adm.policies["readonly"] = validDoc()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/policies", nil)
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/vnd.api+json", rec.Header().Get("Content-Type"))

	var doc struct {
		Data []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"data"`
		Meta struct {
			Page struct {
				TotalRecords int `json:"total_records"`
			} `json:"page"`
		} `json:"meta"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Len(t, doc.Data, 2)
	for _, d := range doc.Data {
		require.Equal(t, "policies", d.Type)
	}
	require.Equal(t, 2, doc.Meta.Page.TotalRecords)
}

// TestGetPolicyIncludesDocument — GET /policies/{name} returns a single
// JSON:API document that includes the full IAM document in attributes.
func TestGetPolicyIncludesDocument(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/policies/my-policy", nil)
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/vnd.api+json", rec.Header().Get("Content-Type"))

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Name     string          `json:"name"`
				Document json.RawMessage `json:"document"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "policies", doc.Data.Type)
	require.Equal(t, "my-policy", doc.Data.ID)
	require.Equal(t, "my-policy", doc.Data.Attributes.Name)
	require.NotEmpty(t, doc.Data.Attributes.Document)
}

// TestCreatePolicyInvalidDocument — POST /policies with an invalid JSON
// document in the attributes returns a 422 with code invalid_policy_json
// rendered as JSON:API (errors[0].code).
func TestCreatePolicyInvalidDocument(t *testing.T) {
	p, _ := newTestProcessor(t)

	// The document field is not valid JSON — processor will return invalid_policy_json.
	body := bytes.NewBufferString(`{
		"data":{"type":"policies","attributes":{"name":"my-policy","document":"not-json"}}
	}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/policies", body)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "application/vnd.api+json", rec.Header().Get("Content-Type"))

	var doc struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.NotEmpty(t, doc.Errors)
	require.Equal(t, "invalid_policy_json", doc.Errors[0].Code)
}

// TestCreatePolicySuccess — POST /policies with a valid name and document
// returns 201 with the new policy resource.
func TestCreatePolicySuccess(t *testing.T) {
	p, _ := newTestProcessor(t)

	docJSON := string(validDoc())
	bodyJSON := `{"data":{"type":"policies","attributes":{"name":"my-policy","document":` + docJSON + `}}}`
	body := bytes.NewBufferString(bodyJSON)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/policies", body)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "application/vnd.api+json", rec.Header().Get("Content-Type"))

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "policies", doc.Data.Type)
	require.Equal(t, "my-policy", doc.Data.ID)
	require.Equal(t, "my-policy", doc.Data.Attributes.Name)
}

// TestDeletePolicyReturns204 — DELETE /policies/{name} for a custom policy
// with no attachments returns 204 with no body.
func TestDeletePolicyReturns204(t *testing.T) {
	p, adm := newTestProcessor(t)
	adm.policies["my-policy"] = validDoc()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/policies/my-policy", nil)
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.Bytes())
}

// TestDeleteBuiltinPolicyReturns403 — DELETE /policies/{name} for a builtin
// returns 403 with code policy_read_only.
func TestDeleteBuiltinPolicyReturns403(t *testing.T) {
	p, _ := newTestProcessor(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/policies/readonly", nil)
	newPolicyRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	var doc struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.NotEmpty(t, doc.Errors)
	require.Equal(t, "policy_read_only", doc.Errors[0].Code)
}
