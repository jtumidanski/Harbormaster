package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
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

	pool := hmminio.NewEmpty()
	if err := pool.Rebuild(hmminio.Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
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

func TestDashboardPoolAdapter_ServerInfo_NotReadyWarns(t *testing.T) {
	const notReady = `{"status":"degraded","version":"1.0.0","ready":false,"details":{"storage":{"ready":false}}}`
	srv := stubRustFSServer(t, rustfsEmptyInfoBody, notReady)
	defer srv.Close()

	pool := hmminio.NewEmpty()
	if err := pool.Rebuild(hmminio.Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
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
