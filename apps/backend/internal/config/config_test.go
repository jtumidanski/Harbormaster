package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.ListenAddr)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "json", cfg.LogFormat)
	require.Equal(t, 8*time.Hour, cfg.SessionTimeout)
	require.Equal(t, "harbormaster_session", cfg.SessionCookieName)
	require.Equal(t, "/", cfg.BasePath)
	require.Equal(t, int64(104857600), cfg.UploadMaxBytes)
	require.Equal(t, 168*time.Hour, cfg.ShareLinkMaxTTL)
	require.Equal(t, "proxy", cfg.DownloadProxyMode)
	require.False(t, cfg.MetricsEnabled)
	require.True(t, cfg.SessionCookieSecure, "session cookie Secure attribute defaults to true")
	require.Positive(t, cfg.MetricsPollInterval, "MetricsPollInterval default must be positive")
	require.Positive(t, cfg.MetricsRetention, "MetricsRetention default must be positive")
}

func TestLoadRejectsZeroMetricsPollInterval(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_METRICS_POLL_INTERVAL", "0")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_METRICS_POLL_INTERVAL must be positive")
}

func TestLoadRejectsZeroMetricsRetention(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_METRICS_RETENTION", "0")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_METRICS_RETENTION must be positive")
}

func TestLoadSessionCookieSecureCanBeDisabled(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_SESSION_COOKIE_SECURE", "false")
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.SessionCookieSecure,
		"operators serving over plain HTTP must be able to disable the Secure attribute")
}

func TestLoadOverridesFromEnv(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_LISTEN_ADDR", ":9090")
	t.Setenv("HARBORMASTER_BASE_PATH", "/harbormaster/")
	t.Setenv("HARBORMASTER_DOWNLOAD_PROXY_MODE", "direct")
	t.Setenv("HARBORMASTER_UPLOAD_MAX_BYTES", "52428800")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.ListenAddr)
	require.Equal(t, "/harbormaster", cfg.BasePath, "trailing slash should normalize off")
	require.Equal(t, "direct", cfg.DownloadProxyMode)
	require.Equal(t, int64(52428800), cfg.UploadMaxBytes)
}

func TestLoadRejectsInvalidBasePath(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_BASE_PATH", "harbormaster")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_BASE_PATH must begin with /")
}

func TestLoadRejectsInvalidDownloadMode(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_DOWNLOAD_PROXY_MODE", "stream")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_DOWNLOAD_PROXY_MODE")
}

func TestLoadRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_LOG_FORMAT", "logfmt")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_LOG_FORMAT")
}

func TestLoadRejectsTLSPartial(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_TLS_CERT_FILE", "/tmp/cert.pem")
	_, err := Load()
	require.ErrorContains(t, err, "TLS_CERT_FILE and HARBORMASTER_TLS_KEY_FILE must both be set or both be empty")
}
