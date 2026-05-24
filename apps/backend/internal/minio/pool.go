// Package minio holds the cached madmin + minio-go client pair built from the
// current decrypted connection settings. Pool.Rebuild is called by the
// connection processor after a successful update.
package minio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Credentials carries plaintext keys for client construction. Never persisted.
type Credentials struct {
	EndpointURL     string
	AccessKey       string
	SecretKey       string
	TLSSkipVerify   bool
	CustomCAPEMText string
}

// Pool holds the active client pair behind an RWMutex.
type Pool struct {
	mu   sync.RWMutex
	mc   *miniogo.Client
	madm *madmin.AdminClient
	cred Credentials
}

// NewEmpty returns an unbound Pool (useful before setup is complete).
func NewEmpty() *Pool { return &Pool{} }

// Rebuild swaps in new clients built from creds. Old credentials are zeroed.
func (p *Pool) Rebuild(creds Credentials) error {
	mc, madm, err := build(creds)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Note: strings are immutable in Go, so we cannot zero SecretKey bytes.
	// The old credential struct is overwritten and becomes eligible for GC.
	p.cred = creds
	p.mc = mc
	p.madm = madm
	return nil
}

// Get returns the current pair, or ErrNotInitialized if Rebuild has not yet
// succeeded.
func (p *Pool) Get(ctx context.Context) (*madmin.AdminClient, *miniogo.Client, error) {
	_ = ctx
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.mc == nil || p.madm == nil {
		return nil, nil, ErrNotInitialized
	}
	return p.madm, p.mc, nil
}

// ErrNotInitialized is returned by Get when the pool has no active connection.
var ErrNotInitialized = errors.New("minio pool: connection not yet configured")

func build(c Credentials) (*miniogo.Client, *madmin.AdminClient, error) {
	parsed, useTLS, host, err := parseEndpoint(c.EndpointURL)
	if err != nil {
		return nil, nil, err
	}
	tr, err := transport(c, useTLS)
	if err != nil {
		return nil, nil, err
	}
	mc, err := miniogo.New(host, &miniogo.Options{
		Creds:     credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure:    useTLS,
		Transport: tr,
	})
	if err != nil {
		return nil, nil, err
	}
	madm, err := madmin.NewWithOptions(host, &madmin.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: useTLS,
	})
	if err != nil {
		return nil, nil, err
	}
	madm.SetCustomTransport(tr)
	_ = parsed
	return mc, madm, nil
}

// parseEndpoint parses an HTTP(S) URL and returns the parsed URL,
// a boolean indicating if TLS is used, and the host:port string.
func parseEndpoint(raw string) (*url.URL, bool, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false, "", fmt.Errorf("invalid endpoint URL %q: %w", raw, err)
	}
	useTLS := u.Scheme == "https"
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false, "", fmt.Errorf("endpoint URL scheme must be http or https (got %q)", u.Scheme)
	}
	return u, useTLS, u.Host, nil
}

// transport builds an http.Transport with optional TLS configuration,
// including custom CA support via CustomCAPEMText.
func transport(c Credentials, useTLS bool) (*http.Transport, error) {
	t := &http.Transport{}
	if !useTLS {
		return t, nil
	}
	tlsConfig := &tls.Config{InsecureSkipVerify: c.TLSSkipVerify}
	if c.CustomCAPEMText != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if ok := pool.AppendCertsFromPEM([]byte(c.CustomCAPEMText)); !ok {
			return nil, errors.New("failed to parse custom CA PEM")
		}
		tlsConfig.RootCAs = pool
	}
	t.TLSClientConfig = tlsConfig
	return t, nil
}
