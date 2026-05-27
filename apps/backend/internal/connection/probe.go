package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// SubmitInput is the body shape accepted by /api/v1/setup and
// /api/v1/connection (PUT, test). FromMcAlias is only honoured by the
// setup wizard; the connection-domain handlers ignore it.
type SubmitInput struct {
	EndpointURL   string `json:"endpoint_url"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	TLSSkipVerify *bool  `json:"tls_skip_verify,omitempty"`
	CustomCAPEM   string `json:"custom_ca_pem,omitempty"`
	FromMcAlias   string `json:"from_mc_alias,omitempty"`
}

// TestResult mirrors the response shape documented in api-contracts.md for
// POST /api/v1/connection/test. Each step is either the string "ok" or a
// {"failed": "<reason>"} object; ServerInfo also returns the version banner.
type TestResult struct {
	TCPConnect   any    `json:"tcp_connect"`
	ListBuckets  any    `json:"list_buckets"`
	AdminPing    any    `json:"admin_ping"`
	MinIOVersion string `json:"minio_version,omitempty"`
}

// probeDialTimeout bounds the initial TCP connect. Kept short so a bad
// endpoint URL surfaces quickly in the setup wizard.
const probeDialTimeout = 3 * time.Second

// Probe performs the three checks documented in the API contract:
//
//  1. TCP connect to host:port (within probeDialTimeout)
//  2. ListBuckets via minio-go (auth + basic data-plane access)
//  3. ServerInfo via madmin-go (admin capability + version banner)
//
// On the happy path it returns (TestResult{all "ok", MinIOVersion: …}, nil).
// On any failure it returns the partially-filled TestResult and the typed
// apierror describing the first failed step; callers map that directly to
// a 422 response with one of the documented codes:
//
//   - minio_unreachable        — TCP / TLS / non-auth network errors
//   - minio_invalid_credentials — InvalidAccessKeyId or SignatureDoesNotMatch
//   - minio_not_admin           — AccessDenied on madmin.ServerInfo
func Probe(ctx context.Context, in SubmitInput) (TestResult, *apierror.Error) {
	out := TestResult{}

	u, err := url.Parse(in.EndpointURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		reason := "endpoint_url must be an absolute http or https URL"
		if err != nil {
			reason = err.Error()
		}
		out.TCPConnect = map[string]string{"failed": reason}
		return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"Endpoint URL is malformed").
			WithDetails(map[string]any{"underlying": reason})
	}
	useTLS := u.Scheme == "https"
	host := u.Host

	dialer := &net.Dialer{Timeout: probeDialTimeout}
	conn, derr := dialer.DialContext(ctx, "tcp", host)
	if derr != nil {
		out.TCPConnect = map[string]string{"failed": derr.Error()}
		return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"TCP connect failed").
			WithDetails(map[string]any{"underlying": derr.Error()})
	}
	_ = conn.Close()
	out.TCPConnect = "ok"

	skipVerify := false
	if in.TLSSkipVerify != nil {
		skipVerify = *in.TLSSkipVerify
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: skipVerify} //nolint:gosec // operator-configurable
	if in.CustomCAPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(in.CustomCAPEM)) {
			return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
				"Custom CA PEM is not valid")
		}
		tlsCfg.RootCAs = pool
	}
	tr := &http.Transport{TLSClientConfig: tlsCfg}

	mc, err := miniogo.New(host, &miniogo.Options{
		Creds:     credentials.NewStaticV4(in.AccessKey, in.SecretKey, ""),
		Secure:    useTLS,
		Transport: tr,
	})
	if err != nil {
		return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"client init failed").
			WithDetails(map[string]any{"underlying": err.Error()})
	}
	if _, err := mc.ListBuckets(ctx); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "InvalidAccessKeyId"),
			strings.Contains(msg, "SignatureDoesNotMatch"):
			out.ListBuckets = map[string]string{"failed": msg}
			return out, apierror.New(http.StatusUnprocessableEntity,
				"minio_invalid_credentials",
				"MinIO rejected the provided keys")
		default:
			out.ListBuckets = map[string]string{"failed": msg}
			return out, apierror.New(http.StatusUnprocessableEntity,
				"minio_unreachable", "list buckets failed").
				WithDetails(map[string]any{"underlying": msg})
		}
	}
	out.ListBuckets = "ok"

	madm, err := madmin.NewWithOptions(host, &madmin.Options{
		Creds:  credentials.NewStaticV4(in.AccessKey, in.SecretKey, ""),
		Secure: useTLS,
	})
	if err != nil {
		return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"admin client init failed").
			WithDetails(map[string]any{"underlying": err.Error()})
	}
	madm.SetCustomTransport(tr)
	info, err := madm.ServerInfo(ctx)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "AccessDenied") {
			out.AdminPing = map[string]string{"failed": msg}
			return out, apierror.New(http.StatusUnprocessableEntity, "minio_not_admin",
				"Provided MinIO keys lack admin capability")
		}
		out.AdminPing = map[string]string{"failed": msg}
		return out, apierror.New(http.StatusUnprocessableEntity, "minio_unreachable",
			"admin ping failed").
			WithDetails(map[string]any{"underlying": msg})
	}
	out.AdminPing = "ok"
	out.MinIOVersion = serverVersion(info)
	return out, nil
}

// serverVersion picks the most useful version banner available in the
// madmin v3 InfoMessage. Prefer the per-server RELEASE.YYYY-… string;
// fall back to Mode if no servers are reported (older deployments).
func serverVersion(info madmin.InfoMessage) string {
	if len(info.Servers) > 0 && info.Servers[0].Version != "" {
		return info.Servers[0].Version
	}
	return info.Mode
}
