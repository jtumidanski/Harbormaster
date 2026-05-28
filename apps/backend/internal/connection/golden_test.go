package connection_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/connection"
)

func goldenKeySet(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, raw)
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestConnectionGetResponseWireContract(t *testing.T) {
	got := goldenKeySet(t, connection.GetResponse{
		EndpointURL: "https://minio.lan:9000", TLSSkipVerify: true,
		AccessKeyMasked: "AKIA***", SecretKeyPresent: true, CustomCAPEMPresent: false,
	})
	want := []string{"access_key_masked", "custom_ca_pem_present", "endpoint_url", "secret_key_present", "tls_skip_verify"}
	if len(got) != len(want) {
		t.Fatalf("wire keys: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wire keys: got %v want %v", got, want)
		}
	}
}

// TestConnectionTestResultSkippedStepsAreNull guards the /connection/test wire
// shape: each step is "ok" | {failed} | null. A step not reached on an early
// failure must serialize as JSON null (the SPA models it that way), never "".
func TestConnectionTestResultSkippedStepsAreNull(t *testing.T) {
	raw, err := json.Marshal(connection.TestResult{
		TCPConnect: map[string]string{"failed": "connection refused"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["list_buckets"]) != "null" || string(m["admin_ping"]) != "null" {
		t.Fatalf("skipped steps must serialize as null: %s", raw)
	}
}
