// Package httpx holds small HTTP helpers shared across the domain packages.
package httpx

import (
	"net"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// ClientIP returns the source IP to record for a request.
//
// The value is set by whichever chi ClientIPFrom* middleware server.Run
// installed: ClientIPFromXFF when HARBORMASTER_TRUSTED_PROXIES lists the
// reverse proxies in front of us, ClientIPFromRemoteAddr otherwise. Both
// fail closed — they leave no IP in the context rather than trusting a
// header we cannot vouch for — so callers get "" for a spoofed or
// unparseable chain.
//
// The r.RemoteAddr fallback covers handlers exercised without the root
// middleware stack (unit tests, direct httptest wiring). It never sees a
// client-controlled header, so it cannot reintroduce the spoofing that
// chi's deprecated RealIP allowed.
func ClientIP(r *http.Request) string {
	if ip := chimw.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
