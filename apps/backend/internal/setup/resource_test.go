package setup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/setup"
)

func newRouter(p *setup.Processor) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", setup.Routes(p))
	return r
}

// TestRoutes_StatusEndpoint exercises GET /setup/status before and after a
// successful Submit, asserting the documented wire shape {"initialized": bool}
// (api-contracts.md). Asserting the raw JSON key — not a Go struct field — is
// deliberate: the SPA reads `initialized`, so emitting any other key (e.g. the
// legacy `setup_completed`) leaves the frontend permanently on the setup
// wizard and 409-looping after setup completes.
func TestRoutes_StatusEndpoint(t *testing.T) {
	p, _ := newProcessor(t, "/nonexistent")
	srv := newRouter(p)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var before map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &before))
	require.Contains(t, before, "initialized", "status must use the contract key `initialized`")
	require.NotContains(t, before, "setup_completed", "status must not use the legacy key")
	require.Equal(t, false, before["initialized"])

	// Flip via the processor; the next GET should see it.
	require.NoError(t, p.Submit(context.Background(), validRequest(), "127.0.0.1"))

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var after map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &after))
	require.Equal(t, true, after["initialized"])
}

// TestRoutes_McAliasesEndpoint exercises the three branches of
// GET /setup/mc-aliases: v10 (parsed), v9 (empty + unsupported_version),
// and missing file (empty + no version).
func TestRoutes_McAliasesEndpoint(t *testing.T) {
	dir := t.TempDir()

	// Branch 1: v10 file.
	mcV10 := filepath.Join(dir, "v10.json")
	require.NoError(t, os.WriteFile(mcV10, []byte(`{
		"version": "10",
		"aliases": {"myminio": {"url": "https://minio.lan:9000", "accessKey": "AKIA", "secretKey": "S", "insecure": false}}
	}`), 0o600))
	p, _ := newProcessor(t, mcV10)
	srv := newRouter(p)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/mc-aliases", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp setup.McAliasesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Aliases, 1)
	require.Equal(t, "myminio", resp.Aliases[0].Name)
	require.Empty(t, resp.UnsupportedVersion)

	// Branch 2: v9 (unsupported).
	mcV9 := filepath.Join(dir, "v9.json")
	require.NoError(t, os.WriteFile(mcV9, []byte(`{"version":"9","aliases":{}}`), 0o600))
	p2, _ := newProcessor(t, mcV9)
	srv2 := newRouter(p2)
	w = httptest.NewRecorder()
	srv2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/mc-aliases", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Aliases)
	require.Equal(t, "9", resp.UnsupportedVersion)

	// Branch 3: missing file.
	p3, _ := newProcessor(t, filepath.Join(dir, "does-not-exist.json"))
	srv3 := newRouter(p3)
	w = httptest.NewRecorder()
	srv3.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/setup/mc-aliases", nil))
	require.Equal(t, http.StatusOK, w.Code)
	resp = setup.McAliasesResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Aliases)
	require.Empty(t, resp.UnsupportedVersion)
}

// TestRoutes_PostSetupHappyPath asserts that POST /setup returns 204 on
// the first call and 409 setup_already_completed on the second.
func TestRoutes_PostSetupHappyPath(t *testing.T) {
	p, _ := newProcessor(t, "/nonexistent")
	srv := newRouter(p)

	body, err := json.Marshal(validRequest())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// Second call: 409 setup_already_completed.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusConflict, w2.Code)
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &env))
	require.Equal(t, "setup_already_completed", env.Error.Code)
}

// TestRoutes_PostSetupBadJSON asserts that an invalid request body returns
// 400 bad_request via the action envelope.
func TestRoutes_PostSetupBadJSON(t *testing.T) {
	p, _ := newProcessor(t, "/nonexistent")
	srv := newRouter(p)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "bad_request", env.Error.Code)
}

// TestRoutes_PostSetupAliasNotFound asserts that referencing a missing
// alias returns 422 mc_alias_not_found via the action envelope.
func TestRoutes_PostSetupAliasNotFound(t *testing.T) {
	dir := t.TempDir()
	mcPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(mcPath, []byte(`{"version":"10","aliases":{}}`), 0o600))

	p, _ := newProcessor(t, mcPath)
	srv := newRouter(p)

	var req setup.Request
	req.Admin.Username = "admin"
	req.Admin.Password = "pw"
	req.MinIO.FromMcAlias = "missing"
	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httpReq)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "mc_alias_not_found", env.Error.Code)
}
