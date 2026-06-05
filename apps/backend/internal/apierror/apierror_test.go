package apierror_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

func TestWriteActionEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleAction, apierror.New(409, "bucket_not_empty", "Bucket contains objects").WithDetails(map[string]any{"object_count": 142}))
	require.Equal(t, 409, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	body := got["error"].(map[string]any)
	require.Equal(t, "bucket_not_empty", body["code"])
	require.Equal(t, float64(142), body["details"].(map[string]any)["object_count"])
}

func TestWriteJSONAPIEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleJSONAPI, apierror.New(422, "invalid_bucket_name", "Bucket name invalid").WithPointer("/data/attributes/name"))
	require.Equal(t, 422, w.Code)
	require.Equal(t, "application/vnd.api+json", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), `"pointer":"/data/attributes/name"`)
}

func TestWriteJSONAPIEnvelopeWithDetails(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleJSONAPI, apierror.New(409, "policy_in_use", "Policy is in use").WithDetails(map[string]any{
		"attached_to": map[string]any{
			"users":  []string{"alice"},
			"groups": []string{},
		},
	}))
	require.Equal(t, 409, w.Code)
	require.Equal(t, "application/vnd.api+json", w.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	errs := got["errors"].([]any)
	require.Len(t, errs, 1)
	e := errs[0].(map[string]any)
	require.Equal(t, "policy_in_use", e["code"])

	meta := e["meta"].(map[string]any)
	attachedTo := meta["attached_to"].(map[string]any)
	users := attachedTo["users"].([]any)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0])
}

func TestWriteJSONAPIEnvelopeNoDetailsOmitsMeta(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleJSONAPI, apierror.New(404, "not_found", "Resource not found"))
	require.Equal(t, 404, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	errs := got["errors"].([]any)
	require.Len(t, errs, 1)
	e := errs[0].(map[string]any)
	require.Equal(t, "not_found", e["code"])
	_, hasMeta := e["meta"]
	require.False(t, hasMeta, "meta key must be absent when there are no details")
}
