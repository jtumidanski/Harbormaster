// Package config loads Harbormaster configuration from env vars, an optional
// config file, and defaults. The returned Config value is immutable.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the resolved Harbormaster configuration. Pass by value.
type Config struct {
	ListenAddr                string
	DataDir                   string
	DatabasePath              string
	LogLevel                  string
	LogFormat                 string
	SessionTimeout            time.Duration
	SessionCookieName         string
	SessionCookieSecure       bool
	BasePath                  string
	TrustedProxies            []string
	UploadMaxBytes            int64
	ShareLinkMaxTTL           time.Duration
	DownloadProxyMode         string
	McConfigPath              string
	TLSCertFile               string
	TLSKeyFile                string
	EncryptionKeyFile         string
	MetricsEnabled            bool
	MetricsListenAddr         string
	OTELExporterOTLPEndpoint  string
	AuditRetention            time.Duration
	MetricsPollInterval       time.Duration
	MetricsRetention          time.Duration
}

// Load reads configuration in priority order: env (HARBORMASTER_*) > file > defaults.
func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("HARBORMASTER")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	defaults(v)

	if p := v.GetString("CONFIG"); p != "" {
		v.SetConfigFile(p)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("reading config file %s: %w", p, err)
		}
	}

	cfg := Config{
		ListenAddr:               v.GetString("LISTEN_ADDR"),
		DataDir:                  v.GetString("DATA_DIR"),
		DatabasePath:             v.GetString("DATABASE_PATH"),
		LogLevel:                 v.GetString("LOG_LEVEL"),
		LogFormat:                v.GetString("LOG_FORMAT"),
		SessionTimeout:           v.GetDuration("SESSION_TIMEOUT"),
		SessionCookieName:        v.GetString("SESSION_COOKIE_NAME"),
		SessionCookieSecure:      v.GetBool("SESSION_COOKIE_SECURE"),
		BasePath:                 normalizeBasePath(v.GetString("BASE_PATH")),
		TrustedProxies:           splitCSV(v.GetString("TRUSTED_PROXIES")),
		UploadMaxBytes:           v.GetInt64("UPLOAD_MAX_BYTES"),
		ShareLinkMaxTTL:          v.GetDuration("SHARE_LINK_MAX_TTL"),
		DownloadProxyMode:        v.GetString("DOWNLOAD_PROXY_MODE"),
		McConfigPath:             v.GetString("MC_CONFIG_PATH"),
		TLSCertFile:              v.GetString("TLS_CERT_FILE"),
		TLSKeyFile:               v.GetString("TLS_KEY_FILE"),
		EncryptionKeyFile:        v.GetString("ENCRYPTION_KEY_FILE"),
		MetricsEnabled:           v.GetBool("METRICS_ENABLED"),
		MetricsListenAddr:        v.GetString("METRICS_LISTEN_ADDR"),
		OTELExporterOTLPEndpoint: v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		AuditRetention:           v.GetDuration("AUDIT_RETENTION"),
		MetricsPollInterval:      v.GetDuration("METRICS_POLL_INTERVAL"),
		MetricsRetention:         v.GetDuration("METRICS_RETENTION"),
	}

	if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "harbormaster.db")
	}
	if cfg.EncryptionKeyFile == "" {
		cfg.EncryptionKeyFile = filepath.Join(cfg.DataDir, "encryption.key")
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults(v *viper.Viper) {
	v.SetDefault("LISTEN_ADDR", ":8080")
	v.SetDefault("DATA_DIR", "/var/lib/harbormaster")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")
	v.SetDefault("SESSION_TIMEOUT", 8*time.Hour)
	v.SetDefault("SESSION_COOKIE_NAME", "harbormaster_session")
	// Secure by default. Operators serving over plain HTTP (no TLS-terminating
	// proxy) must set HARBORMASTER_SESSION_COOKIE_SECURE=false, or browsers
	// will silently drop the session cookie and login will not persist.
	v.SetDefault("SESSION_COOKIE_SECURE", true)
	v.SetDefault("BASE_PATH", "/")
	v.SetDefault("UPLOAD_MAX_BYTES", int64(104857600))
	v.SetDefault("SHARE_LINK_MAX_TTL", 168*time.Hour)
	v.SetDefault("DOWNLOAD_PROXY_MODE", "proxy")
	v.SetDefault("MC_CONFIG_PATH", "/root/.mc/config.json")
	v.SetDefault("METRICS_ENABLED", false)
	v.SetDefault("METRICS_LISTEN_ADDR", ":9090")
	v.SetDefault("AUDIT_RETENTION", 90*24*time.Hour)
	v.SetDefault("METRICS_POLL_INTERVAL", 30*time.Second)
	v.SetDefault("METRICS_RETENTION", 8*24*time.Hour)
}

func validate(c Config) error {
	if !strings.HasPrefix(c.BasePath, "/") {
		return errors.New("HARBORMASTER_BASE_PATH must begin with /")
	}
	if c.LogFormat != "json" && c.LogFormat != "console" {
		return fmt.Errorf("HARBORMASTER_LOG_FORMAT must be json or console (got %q)", c.LogFormat)
	}
	if c.DownloadProxyMode != "proxy" && c.DownloadProxyMode != "direct" {
		return fmt.Errorf("HARBORMASTER_DOWNLOAD_PROXY_MODE must be proxy or direct (got %q)", c.DownloadProxyMode)
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return errors.New("HARBORMASTER_TLS_CERT_FILE and HARBORMASTER_TLS_KEY_FILE must both be set or both be empty")
	}
	if c.UploadMaxBytes <= 0 {
		return errors.New("HARBORMASTER_UPLOAD_MAX_BYTES must be positive")
	}
	if c.MetricsPollInterval <= 0 {
		return errors.New("HARBORMASTER_METRICS_POLL_INTERVAL must be positive")
	}
	if c.MetricsRetention <= 0 {
		return errors.New("HARBORMASTER_METRICS_RETENTION must be positive")
	}
	return nil
}

func normalizeBasePath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return strings.TrimRight(p, "/")
	}
	return p
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
