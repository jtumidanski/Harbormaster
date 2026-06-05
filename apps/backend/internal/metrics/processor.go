package metrics

import (
	"context"
	"time"
)

// Processor holds a Store and poll configuration, and serves aggregated
// metric views.
type Processor struct {
	store      *Store
	pollPeriod time.Duration
	now        func() time.Time
}

// NewProcessor returns a Processor backed by store and using time.Now for
// freshness checks.
func NewProcessor(store *Store, pollPeriod time.Duration) *Processor {
	return &Processor{store: store, pollPeriod: pollPeriod, now: time.Now}
}

// trackedNames returns the names of all tracked metrics.
func trackedNames() []string {
	names := make([]string, 0, len(trackedMetrics))
	for k := range trackedMetrics {
		names = append(names, k)
	}
	return names
}

// View queries the store, aggregates the results into a View for the given
// window, and sets Collected based on data freshness.
func (p *Processor) View(ctx context.Context, w Window) (View, error) {
	now := p.now().UTC()
	raw, err := p.store.Query(ctx, trackedNames(), now.Add(-w.Duration()))
	if err != nil {
		return View{}, err
	}
	view := Aggregate(w, raw, now)
	view.Collected = p.isFresh(raw, now)
	return view, nil
}

// isFresh returns true if any series has a data point within 2*pollPeriod
// of now, indicating the poller is actively collecting data.
func (p *Processor) isFresh(raw map[string][]Point, now time.Time) bool {
	if len(raw) == 0 {
		return false
	}
	threshold := now.Add(-2 * p.pollPeriod)
	for _, pts := range raw {
		if len(pts) == 0 {
			continue
		}
		last := pts[len(pts)-1]
		if !last.T.Before(threshold) {
			return true
		}
	}
	return false
}
