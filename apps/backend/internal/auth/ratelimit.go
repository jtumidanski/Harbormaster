package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter throttles failed logins per source IP. In-memory only;
// single-replica deployment as documented in risks.md R6.
type LoginRateLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	window   time.Duration
	max      int
}

// NewLoginRateLimiter creates a limiter with the given window and max-fail count.
func NewLoginRateLimiter(window time.Duration, max int) *LoginRateLimiter {
	return &LoginRateLimiter{failures: map[string][]time.Time{}, window: window, max: max}
}

// Allow returns true iff the IP may attempt a login now. It does NOT record
// the attempt; callers MUST call RecordFailure() after a failed login.
func (l *LoginRateLimiter) Allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	pruned := l.failures[ip][:0]
	for _, t := range l.failures[ip] {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	l.failures[ip] = pruned
	return len(pruned) < l.max
}

// RecordFailure appends a failure timestamp.
func (l *LoginRateLimiter) RecordFailure(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.failures[ip], now)
}

// Reset clears all recorded failures for an IP (call on successful login).
func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}
