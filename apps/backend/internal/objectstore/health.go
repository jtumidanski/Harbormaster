package objectstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// healthReadyBody is RustFS's /health/ready shape. Verified live against
// RustFS 1.0.0 (task-005 Step 3 live check): its admin "info" endpoint
// (madmin.ServerInfo) responds 200 with neither a version nor a servers[]
// entry — only bucket/object/usage counts — so the dashboard falls back to
// this public, unauthenticated endpoint for version and single-node
// readiness. MinIO has no equivalent endpoint; callers only use this
// fallback when ServerInfo comes back empty.
type healthReadyBody struct {
	Version string `json:"version"`
	Ready   bool   `json:"ready"`
	Details struct {
		Storage struct {
			Ready bool `json:"ready"`
		} `json:"storage"`
	} `json:"details"`
}

// ServerHealthInfo is the subset of /health/ready the dashboard adapter
// consumes: the reported version string and whether the storage layer
// considers itself ready (mapped to "one drive online").
type ServerHealthInfo struct {
	Version      string
	Ready        bool
	StorageReady bool
}

// decodeHealthReady parses a /health/ready response body.
func decodeHealthReady(body []byte) (ServerHealthInfo, error) {
	var b healthReadyBody
	if err := json.Unmarshal(body, &b); err != nil {
		return ServerHealthInfo{}, fmt.Errorf("health/ready: decode: %w", err)
	}
	return ServerHealthInfo{Version: b.Version, Ready: b.Ready, StorageReady: b.Details.Storage.Ready}, nil
}

// ServerHealth fetches the target's /health/ready endpoint and returns its
// version + readiness. It is a fallback for targets (RustFS) whose admin
// "info" endpoint does not report a version or per-server state. Returns
// ErrNotInitialized when no connection is configured, matching Get.
func (p *Pool) ServerHealth(ctx context.Context) (ServerHealthInfo, error) {
	p.mu.RLock()
	cred := p.cred
	ready := p.mc != nil && p.madm != nil
	p.mu.RUnlock()
	if !ready {
		return ServerHealthInfo{}, ErrNotInitialized
	}

	parsed, useTLS, _, err := parseEndpoint(cred.EndpointURL)
	if err != nil {
		return ServerHealthInfo{}, err
	}
	tr, err := transport(cred, useTLS)
	if err != nil {
		return ServerHealthInfo{}, err
	}
	client := &http.Client{Transport: tr}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u := *parsed
	u.Path = "/health/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ServerHealthInfo{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ServerHealthInfo{}, fmt.Errorf("health/ready: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ServerHealthInfo{}, fmt.Errorf("health/ready: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ServerHealthInfo{}, fmt.Errorf("health/ready: read: %w", err)
	}
	return decodeHealthReady(body)
}
