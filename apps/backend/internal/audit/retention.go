package audit

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// StartRetentionSweeper launches a background goroutine that calls
// p.RetentionSweep on the given interval.  The cutoff is calculated as
// time.Now() minus p.Retention() so that events older than the retention
// window are deleted.  The goroutine exits when ctx is cancelled.
func StartRetentionSweeper(ctx context.Context, p *Processor, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-p.Retention())
				n, err := p.RetentionSweep(cutoff)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("audit retention sweep failed")
					continue
				}
				if n > 0 {
					log.Ctx(ctx).Info().Int64("deleted", n).Msg("audit retention sweep complete")
				}
			}
		}
	}()
}
