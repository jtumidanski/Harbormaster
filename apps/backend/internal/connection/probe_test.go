package connection_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/connection"
)

// TestProbe_RejectsMalformedEndpointURL verifies that a missing or
// scheme-less URL fails fast on the URL parse step with the documented
// "minio_unreachable" code. No network I/O is attempted.
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

			_, ae := connection.Probe(ctx, connection.SubmitInput{
				EndpointURL: tc.endpoint,
				AccessKey:   "ak",
				SecretKey:   "sk",
			})
			require.NotNil(t, ae, "expected an apierror for %q", tc.endpoint)
			require.Equal(t, http.StatusUnprocessableEntity, ae.HTTPStatus)
			require.Equal(t, "minio_unreachable", ae.Code)
		})
	}
}

// TestProbe_TCPConnectFailure verifies that a closed-port endpoint
// surfaces a TCP-step failure with the "minio_unreachable" code. The
// listener is bound to 127.0.0.1:0 and then closed so the OS frees the
// port before Probe attempts to dial it, ensuring a deterministic ECONNREFUSED.
func TestProbe_TCPConnectFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, ae := connection.Probe(ctx, connection.SubmitInput{
		EndpointURL: "http://" + addr,
		AccessKey:   "ak",
		SecretKey:   "sk",
	})
	require.NotNil(t, ae)
	require.Equal(t, http.StatusUnprocessableEntity, ae.HTTPStatus)
	require.Equal(t, "minio_unreachable", ae.Code)
	require.NotNil(t, ae.Details, "expected underlying detail on dial failure")

	// Sanity-check: the typed error survives errors.As round-trips.
	var unwrapped *apierror.Error
	require.True(t, errors.As(error(ae), &unwrapped))
	require.Equal(t, "minio_unreachable", unwrapped.Code)
}
