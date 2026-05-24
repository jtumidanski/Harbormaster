package server

import (
	"net/http"
	"strings"
)

func spaHandler(basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		_ = basePath
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Harbormaster</title><p>SPA placeholder — M2 will wire embedded assets.</p>`))
	})
}
