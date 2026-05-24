package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAssetPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"assets root file", "/assets/foo", true},
		{"assets nested file", "/assets/x/y.css", true},
		{"favicon ico", "/favicon.ico", true},
		{"favicon svg", "/favicon.svg", true},
		{"webmanifest", "/manifest.webmanifest", true},
		{"root", "/", false},
		{"index.html", "/index.html", false},
		{"arbitrary spa path", "/foo", false},
		{"assets without trailing slash", "/assets", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isAssetPath(tc.path))
		})
	}
}

func TestRewriteBase(t *testing.T) {
	html := []byte(`<!doctype html><html><head><base href="/" /><title>Harbormaster</title></head></html>`)

	t.Run("empty basePath preserves input", func(t *testing.T) {
		got := rewriteBase(html, "")
		require.True(t, bytes.Equal(html, got), "expected unchanged html")
	})

	t.Run("root basePath preserves input", func(t *testing.T) {
		got := rewriteBase(html, "/")
		require.True(t, bytes.Equal(html, got), "expected unchanged html")
	})

	t.Run("subpath rewrites base href", func(t *testing.T) {
		got := rewriteBase(html, "/admin")
		require.Contains(t, string(got), `<base href="/admin/"`)
		require.NotContains(t, string(got), `<base href="/"`)
	})

	t.Run("subpath with trailing slash not double-added", func(t *testing.T) {
		got := rewriteBase(html, "/admin/")
		require.Contains(t, string(got), `<base href="/admin/"`)
		require.NotContains(t, string(got), `<base href="/admin//"`)
	})

	t.Run("missing base tag returns input unchanged", func(t *testing.T) {
		noBase := []byte(`<!doctype html><html><head><title>Harbormaster</title></head></html>`)
		got := rewriteBase(noBase, "/admin")
		require.True(t, bytes.Equal(noBase, got), "expected unchanged html when no base tag present")
	})
}

func TestSPAHandler_HTTP(t *testing.T) {
	h := spaHandler("/")

	t.Run("GET with text/html accept returns 200 placeholder", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		require.NotEmpty(t, rec.Body.Bytes())
	})

	t.Run("POST returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/some/path", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("GET without text/html accept returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}
