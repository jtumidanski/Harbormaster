package connection

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v4"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// TestProbe_RejectsMalformedEndpointURL verifies that a missing or
// scheme-less URL fails fast on the URL parse step with the documented
// "object_store_unreachable" code. No network I/O is attempted.
func TestProbe_RejectsMalformedEndpointURL(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
	}{
		{name: "missing scheme", endpoint: "minio.lan:9000"},
		{name: "unsupported scheme", endpoint: "ftp://minio.lan:9000"},
		{name: "empty host", endpoint: "https://"},
		{name: "garbage", endpoint: "::not-a-url::"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, ae := Probe(ctx, SubmitInput{
				EndpointURL: tc.endpoint,
				AccessKey:   "ak",
				SecretKey:   "sk",
			})
			require.NotNil(t, ae, "expected an apierror for %q", tc.endpoint)
			require.Equal(t, http.StatusUnprocessableEntity, ae.HTTPStatus)
			require.Equal(t, "object_store_unreachable", ae.Code)
		})
	}
}

// TestProbe_TCPConnectFailure verifies that a closed-port endpoint
// surfaces a TCP-step failure with the "object_store_unreachable" code. The
// listener is bound to 127.0.0.1:0 and then closed so the OS frees the
// port before Probe attempts to dial it, ensuring a deterministic ECONNREFUSED.
func TestProbe_TCPConnectFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, ae := Probe(ctx, SubmitInput{
		EndpointURL: "http://" + addr,
		AccessKey:   "ak",
		SecretKey:   "sk",
	})
	require.NotNil(t, ae)
	require.Equal(t, http.StatusUnprocessableEntity, ae.HTTPStatus)
	require.Equal(t, "object_store_unreachable", ae.Code)
	require.NotNil(t, ae.Details, "expected underlying detail on dial failure")

	// Sanity-check: the typed error survives errors.As round-trips.
	var unwrapped *apierror.Error
	require.True(t, errors.As(error(ae), &unwrapped))
	require.Equal(t, "object_store_unreachable", unwrapped.Code)
}

func TestServerVersion_BareSemver(t *testing.T) {
	info := madmin.InfoMessage{Servers: []madmin.ServerProperties{{Version: "1.0.0"}}}
	if got := serverVersion(info); got != "1.0.0" {
		t.Errorf("want 1.0.0, got %q", got)
	}
}

func TestServerVersion_NoServersFallsBackToMode(t *testing.T) {
	info := madmin.InfoMessage{Mode: "online"}
	if got := serverVersion(info); got != "online" {
		t.Errorf("want mode fallback, got %q", got)
	}
}

func TestServerVersion_Empty(t *testing.T) {
	if got := serverVersion(madmin.InfoMessage{}); got != "unknown" {
		t.Errorf("want \"unknown\" for an empty banner, got %q", got)
	}
}

// TestProbeResult_EmitsServerVersionAndLegacyKey verifies that the success
// path's version banner is serialised under both server_version and the
// legacy minio_version key.
func TestProbeResult_EmitsServerVersionAndLegacyKey(t *testing.T) {
	out := withServerVersion(TestResult{TCPConnect: "ok", ListBuckets: "ok", AdminPing: "ok"}, "1.0.0")
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"server_version":"1.0.0"`)
	require.Contains(t, string(raw), `"minio_version":"1.0.0"`)
}
