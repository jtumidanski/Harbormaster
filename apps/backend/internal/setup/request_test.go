package setup

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestAcceptsObjectStoreAndLegacyMinioField(t *testing.T) {
	var r Request
	require.NoError(t, json.Unmarshal([]byte(`{"admin":{"username":"a","password":"p"},"object_store":{"endpoint_url":"http://x:9000","access_key":"k","secret_key":"s"}}`), &r))
	r.normalize()
	require.Equal(t, "http://x:9000", r.ObjectStore.EndpointURL)

	var legacy Request
	require.NoError(t, json.Unmarshal([]byte(`{"admin":{"username":"a","password":"p"},"minio":{"endpoint_url":"http://y:9000","access_key":"k","secret_key":"s"}}`), &legacy))
	legacy.normalize()
	require.Equal(t, "http://y:9000", legacy.ObjectStore.EndpointURL)
}
