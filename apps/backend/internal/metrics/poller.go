package metrics

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// collectorIface is the minimal interface the poller needs from a Collector.
// Using an interface allows tests to inject a fakeCollector without MinIO deps.
type collectorIface interface {
	Collect(ctx context.Context) (map[string]float64, error)
}

// pollOnce scrapes the collector and writes results into the store at time at.
// On collector error nothing is written and the error is returned.
func pollOnce(ctx context.Context, c collectorIface, store *Store, at time.Time) error {
	values, err := c.Collect(ctx)
	if err != nil {
		return err
	}
	return store.Insert(ctx, at, values)
}

// StartPoller launches a background goroutine that calls pollOnce on every
// tick. Collect errors are logged and skipped; the goroutine exits when ctx is
// cancelled.
func StartPoller(ctx context.Context, c collectorIface, store *Store, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				if err := pollOnce(ctx, c, store, t.UTC()); err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("metrics poll failed")
				}
			}
		}
	}()
}

// StartRetentionSweeper launches a background goroutine that deletes metric
// samples older than retention on every tick. Sweep errors are logged; the
// goroutine exits when ctx is cancelled.
func StartRetentionSweeper(ctx context.Context, store *Store, retention, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-retention)
				n, err := store.RetentionSweep(cutoff)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("metrics retention sweep failed")
					continue
				}
				if n > 0 {
					log.Ctx(ctx).Info().Int64("deleted", n).Msg("metrics retention sweep complete")
				}
			}
		}
	}()
}
