package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/objectstore"
)

// rustfsEmptyInfoBody mirrors the live RustFS 1.0.0 GET
// /minio/admin/v3/info response captured during the task-005 Step 3 live
// check: 200 OK with bucket/object/usage counts only — no "servers" array,
// no "mode". Decoded into madmin.InfoMessage this yields Mode="" and
// Servers=nil, which is what dashboardPoolAdapter.ServerInfo must tolerate.
const rustfsEmptyInfoBody = `{"buckets":{"count":6},"objects":{"count":337126},"versions":{"count":0},"deletemarkers":{"count":0},"usage":{"size":35980333736},"services":{"kms":{},"ldap":{}},"backend":{"backendType":"","onlineDisks":0,"offlineDisks":0,"standardSCParity":0,"rrSCParity":0}}`

const rustfsHealthReadyBody = `{"status":"ok","service":"rustfs-endpoint","version":"1.0.0","ready":true,"details":{"storage":{"status":"connected","ready":true}}}`

// stubRustFSServer answers the two endpoints dashboardPoolAdapter.ServerInfo
// touches when its primary source (madmin.ServerInfo) comes back empty: the
// admin "info" RPC (always, real code path) and the /health/ready fallback.
func stubRustFSServer(t *testing.T, infoBody, healthBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health/ready":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(healthBody))
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(infoBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestDashboardPoolAdapter_ServerInfo_FallsBackToHealthReadyWhenInfoEmpty(t *testing.T) {
	srv := stubRustFSServer(t, rustfsEmptyInfoBody, rustfsHealthReadyBody)
	defer srv.Close()

	pool := objectstore.NewEmpty()
	if err := pool.Rebuild(objectstore.Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	adapter := newDashboardPoolGetter(pool)
	info, nodes, warnings, err := adapter.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.Version != "1.0.0" {
		t.Errorf("want version 1.0.0 from /health/ready fallback, got %q", info.Version)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 synthesised node, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].State != "online" {
		t.Errorf("want node state online, got %q", nodes[0].State)
	}
	if nodes[0].Drives.Total != 1 || nodes[0].Drives.Healthy != 1 || nodes[0].Drives.Unhealthy != 0 {
		t.Errorf("want 1 healthy drive, got %+v", nodes[0].Drives)
	}
	if len(warnings) != 0 {
		t.Errorf("want no warnings for a ready node, got %v", warnings)
	}
}

// minioInfoBody is a MinIO-shaped GET /minio/admin/v3/info response
// (madmin.InfoMessage): "mode" populated and a non-empty "servers" array,
// unlike the RustFS-empty fixture above. Used to prove the /health/ready
// fallback never engages when the primary admin info call already carries
// version/node/drive data — MinIO's normal case.
const minioInfoBody = `{"mode":"server","servers":[{"state":"online","endpoint":"127.0.0.1:9000","uptime":123456,"version":"RELEASE.2025-09-07T16-13-09Z","drives":[{"endpoint":"/data1","state":"ok"},{"endpoint":"/data2","state":"faulty"}]}]}`

// stubMinIOServer answers only the admin "info" path with a MinIO-shaped
// InfoMessage. Any other admin path — in particular /health/ready, the
// fallback endpoint — fails the test immediately: the primary ServerInfo
// source is already populated, so dashboardPoolAdapter.ServerInfo must
// never consult the fallback for a MinIO target.
func stubMinIOServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(minioInfoBody))
			return
		}
		t.Errorf("unexpected request to %s; the /health/ready fallback must stay dormant when the primary admin info call already has version/servers data", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestDashboardPoolAdapter_ServerInfo_MinIOPrimaryPathNeverFallsBack(t *testing.T) {
	srv := stubMinIOServer(t)
	defer srv.Close()

	pool := objectstore.NewEmpty()
	if err := pool.Rebuild(objectstore.Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	adapter := newDashboardPoolGetter(pool)
	info, nodes, warnings, err := adapter.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.Version != "RELEASE.2025-09-07T16-13-09Z" {
		t.Errorf("want version from the primary admin info response, got %q", info.Version)
	}
	if info.DeploymentMode != "server" {
		t.Errorf("want deployment mode %q from the primary admin info response, got %q", "server", info.DeploymentMode)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node from the primary admin info response, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].State != "online" {
		t.Errorf("want node state online, got %q", nodes[0].State)
	}
	if nodes[0].Drives.Total != 2 || nodes[0].Drives.Healthy != 1 || nodes[0].Drives.Unhealthy != 1 {
		t.Errorf("want 2 drives (1 healthy, 1 unhealthy) from the primary admin info response, got %+v", nodes[0].Drives)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unhealthy drives") {
		t.Errorf("want one unhealthy-drives warning, got %v", warnings)
	}
}

func TestDashboardPoolAdapter_ServerInfo_NotReadyWarns(t *testing.T) {
	const notReady = `{"status":"degraded","version":"1.0.0","ready":false,"details":{"storage":{"ready":false}}}`
	srv := stubRustFSServer(t, rustfsEmptyInfoBody, notReady)
	defer srv.Close()

	pool := objectstore.NewEmpty()
	if err := pool.Rebuild(objectstore.Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	adapter := newDashboardPoolGetter(pool)
	info, nodes, warnings, err := adapter.ServerInfo(context.Background())
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}
	if info.Version != "1.0.0" {
		t.Errorf("want version 1.0.0, got %q", info.Version)
	}
	if len(nodes) != 1 || nodes[0].State != "offline" {
		t.Fatalf("want 1 offline node, got %+v", nodes)
	}
	if len(warnings) == 0 {
		t.Errorf("want a not-ready warning")
	}
}
