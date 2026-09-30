package objectstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rustfsHealthReadyShape is a verbatim capture (redacted of nothing
// sensitive — it is a public, unauthenticated endpoint) from a live
// RustFS 1.0.0 cluster's GET /health/ready during the task-005 Step 3
// live check.
const rustfsHealthReadyShape = `{
  "status": "ok",
  "service": "rustfs-endpoint",
  "timestamp": "2026-09-25T02:48:21.482153586Z",
  "version": "1.0.0",
  "ready": true,
  "details": {
    "storage": {
      "status": "connected",
      "ready": true,
      "readinessScope": "write_quorum_and_pool_metadata",
      "source": "local_runtime",
      "readQuorum": true,
      "writeQuorum": true
    },
    "iam": {"status": "connected", "ready": true},
    "lock": {"status": "connected", "ready": true},
    "poolMetadata": {"ready": true, "status": "writable"}
  },
  "degradedReasons": []
}`

const rustfsHealthReadyNotReadyShape = `{
  "status": "degraded",
  "version": "1.0.0",
  "ready": false,
  "details": {
    "storage": {"ready": false}
  }
}`

func TestDecodeHealthReady_ReadyNode(t *testing.T) {
	got, err := decodeHealthReady([]byte(rustfsHealthReadyShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("want version 1.0.0, got %q", got.Version)
	}
	if !got.Ready || !got.StorageReady {
		t.Errorf("want ready+storageReady true, got %+v", got)
	}
}

func TestDecodeHealthReady_NotReady(t *testing.T) {
	got, err := decodeHealthReady([]byte(rustfsHealthReadyNotReadyShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Ready || got.StorageReady {
		t.Errorf("want ready+storageReady false, got %+v", got)
	}
}

func TestDecodeHealthReady_BadJSON(t *testing.T) {
	if _, err := decodeHealthReady([]byte(`{`)); err == nil {
		t.Error("want error on malformed body")
	}
}

// stubHealthServer stands in for a RustFS node: it answers GET /health/ready
// with a fixed RustFS-shaped body and 404s everything else, so a test can
// assert ServerHealth hits exactly that path.
func stubHealthServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/ready" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestPool_ServerHealth_NotInitialized(t *testing.T) {
	p := NewEmpty()
	if _, err := p.ServerHealth(context.Background()); err != ErrNotInitialized {
		t.Errorf("want ErrNotInitialized, got %v", err)
	}
}

func TestPool_ServerHealth_ParsesLiveRustFSShape(t *testing.T) {
	srv := stubHealthServer(t, rustfsHealthReadyShape)
	defer srv.Close()

	p := NewEmpty()
	if err := p.Rebuild(Credentials{EndpointURL: srv.URL, AccessKey: "ak", SecretKey: "sk"}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	got, err := p.ServerHealth(context.Background())
	if err != nil {
		t.Fatalf("ServerHealth: %v", err)
	}
	if got.Version != "1.0.0" || !got.Ready || !got.StorageReady {
		t.Errorf("want version=1.0.0 ready+storageReady=true, got %+v", got)
	}
}
