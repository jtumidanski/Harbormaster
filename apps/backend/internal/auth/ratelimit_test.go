package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

func TestRateLimiterAllowsBudgetAndBlocksSixth(t *testing.T) {
	lim := auth.NewLoginRateLimiter(5*time.Minute, 5)
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		require.True(t, lim.Allow("1.2.3.4", now), "attempt %d should be allowed", i+1)
		lim.RecordFailure("1.2.3.4", now)
	}
	require.False(t, lim.Allow("1.2.3.4", now), "sixth attempt should be denied")
}

func TestRateLimiterAllowsAfterWindowAdvances(t *testing.T) {
	lim := auth.NewLoginRateLimiter(5*time.Minute, 5)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		require.True(t, lim.Allow("1.2.3.4", now))
		lim.RecordFailure("1.2.3.4", now)
	}
	require.False(t, lim.Allow("1.2.3.4", now))

	later := now.Add(6 * time.Minute)
	require.True(t, lim.Allow("1.2.3.4", later),
		"after window expires the IP should regain its budget")
}

func TestRateLimiterResetClearsBudget(t *testing.T) {
	lim := auth.NewLoginRateLimiter(5*time.Minute, 5)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		require.True(t, lim.Allow("1.2.3.4", now))
		lim.RecordFailure("1.2.3.4", now)
	}
	require.False(t, lim.Allow("1.2.3.4", now))

	lim.Reset("1.2.3.4")
	require.True(t, lim.Allow("1.2.3.4", now), "Reset should restore the full budget")
}

func TestRateLimiterIsolatesByIP(t *testing.T) {
	lim := auth.NewLoginRateLimiter(5*time.Minute, 2)
	now := time.Now().UTC()
	lim.RecordFailure("1.1.1.1", now)
	lim.RecordFailure("1.1.1.1", now)
	require.False(t, lim.Allow("1.1.1.1", now))
	require.True(t, lim.Allow("2.2.2.2", now), "another IP must have its own budget")
}
