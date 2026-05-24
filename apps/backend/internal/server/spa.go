package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func spaHandler(basePath string) http.Handler {
	dist, err := fs.Sub(spaFS, "spa-dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "SPA bundle missing", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(dist))
	index, _ := fs.ReadFile(dist, "index.html")
	indexWithBase := rewriteBase(index, basePath)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean(r.URL.Path)
		if isAssetPath(clean) {
			fileServer.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		// When no SPA bundle is embedded (local dev / unit tests with only
		// .gitkeep in spa-dist), index will be empty and we return a tiny
		// placeholder so the server doesn't 200-empty.
		if len(indexWithBase) == 0 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Harbormaster</title><p>SPA bundle not embedded — build the frontend or run the container image.</p>`))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(indexWithBase)
	})
}

func isAssetPath(p string) bool {
	switch {
	case strings.HasPrefix(p, "/assets/"):
		return true
	case p == "/favicon.ico", p == "/favicon.svg":
		return true
	case p == "/manifest.webmanifest":
		return true
	}
	return false
}

func rewriteBase(html []byte, basePath string) []byte {
	if basePath == "" || basePath == "/" {
		return html
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return bytes.ReplaceAll(html, []byte(`<base href="/"`), []byte(`<base href="`+basePath+`"`))
}
