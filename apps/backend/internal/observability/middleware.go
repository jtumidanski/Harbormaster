package observability

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	hmlog "github.com/jtumidanski/Harbormaster/internal/observability/log"
)

var nowFn = time.Now

// Logger emits one structured log line per HTTP request.
func Logger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			rid := chimw.GetReqID(r.Context())
			l := base.With().Str("request_id", rid).Logger()
			ctx := hmlog.WithLogger(r.Context(), l)
			start := nowFn()
			next.ServeHTTP(ww, r.WithContext(ctx))
			l.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("latency", nowFn().Sub(start)).
				Int("bytes", ww.BytesWritten()).
				Msg("http_request")
		})
	}
}
