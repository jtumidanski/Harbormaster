package jsonapi_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

type bucket struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (b bucket) ResourceType() string { return "buckets" }
func (b bucket) ResourceID() string   { return b.Name }

func TestEncodeSingle(t *testing.T) {
	var buf bytes.Buffer
	enc := jsonapi.NewEncoder()
	err := enc.Single(&buf, bucket{Name: "photos", CreatedAt: "2026-05-23T14:00:00Z"}, nil)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	data := got["data"].(map[string]any)
	require.Equal(t, "buckets", data["type"])
	require.Equal(t, "photos", data["id"])
	attrs := data["attributes"].(map[string]any)
	require.Equal(t, "photos", attrs["name"])
}

func TestEncodeCollection(t *testing.T) {
	var buf bytes.Buffer
	enc := jsonapi.NewEncoder()
	items := []jsonapi.Resource{
		bucket{Name: "a", CreatedAt: "t"},
		bucket{Name: "b", CreatedAt: "t"},
	}
	err := enc.Collection(&buf, items, &jsonapi.Meta{Page: &jsonapi.Page{Number: 1, Size: 50, TotalRecords: 2, TotalPages: 1}}, nil)
	require.NoError(t, err)
	require.True(t, strings.Contains(buf.String(), `"total_records":2`))
}

func TestEncodeError(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, jsonapi.WriteError(&buf, jsonapi.Error{
		Status: 422, Code: "invalid_bucket_name", Title: "Invalid bucket name",
		Detail: "must be lowercase", Pointer: "/data/attributes/name",
	}))
	require.Contains(t, buf.String(), `"status":"422"`)
	require.Contains(t, buf.String(), `"pointer":"/data/attributes/name"`)
}

func TestDecodeSingle(t *testing.T) {
	body := `{"data":{"type":"buckets","attributes":{"name":"photos","created_at":"2026-05-23T14:00:00Z"}}}`
	var out bucket
	err := jsonapi.NewDecoder().Single(strings.NewReader(body), &out)
	require.NoError(t, err)
	require.Equal(t, "photos", out.Name)
}
