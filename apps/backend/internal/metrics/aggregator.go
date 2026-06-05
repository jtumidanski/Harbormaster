package metrics

import "time"

// Aggregate downsamples raw samples into <=~300 points per series for the
// window. Counters become per-second rates (negative deltas from resets
// clamped to 0); gauges pass through as the last value in each bucket. now
// is the upper bound of the window (injected for deterministic tests).
func Aggregate(w Window, raw map[string][]Point, now time.Time) View {
	step := w.Step()
	start := now.Add(-w.Duration())
	view := View{
		Window:      w,
		StepSeconds: int(step / time.Second),
		Collected:   len(raw) > 0,
		Series:      map[string][]Point{},
	}
	for metric, pts := range raw {
		if _, isCounter := counterMetrics[metric]; isCounter {
			view.Series[metric] = downsampleRate(pts, start, now, step)
		} else {
			view.Series[metric] = downsampleGauge(pts, start, now, step)
		}
	}
	return view
}

// bucketIndex returns the step bucket a timestamp falls into.
func bucketIndex(t, start time.Time, step time.Duration) int {
	return int(t.Sub(start) / step)
}

// downsampleGauge takes the last value seen in each step bucket.
func downsampleGauge(pts []Point, start, now time.Time, step time.Duration) []Point {
	buckets := map[int]Point{}
	for _, p := range pts {
		if p.T.Before(start) || p.T.After(now) {
			continue
		}
		idx := bucketIndex(p.T, start, step)
		buckets[idx] = Point{T: start.Add(time.Duration(idx) * step), V: p.V}
	}
	return orderedBuckets(buckets)
}

// downsampleRate computes a per-second counter rate per step bucket.
// For each bucket it uses the last sample in that bucket; the rate is derived
// from the delta between successive bucket values divided by the step duration.
// Counter resets (negative delta) clamp to 0.
func downsampleRate(pts []Point, start, now time.Time, step time.Duration) []Point {
	// Collect the last sample per bucket.
	last := map[int]Point{}
	for _, p := range pts {
		if p.T.Before(start) || p.T.After(now) {
			continue
		}
		idx := bucketIndex(p.T, start, step)
		last[idx] = Point{T: start.Add(time.Duration(idx) * step), V: p.V}
	}
	if len(last) == 0 {
		return nil
	}
	// Find min/max bucket indices present.
	minIdx, maxIdx := int(^uint(0)>>1), -int(^uint(0)>>1)-1
	for idx := range last {
		if idx < minIdx {
			minIdx = idx
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	// Compute per-second rate between consecutive occupied buckets.
	out := map[int]Point{}
	stepSec := step.Seconds()
	var prevVal *float64
	for i := minIdx; i <= maxIdx; i++ {
		p, ok := last[i]
		if !ok {
			prevVal = nil // gap resets continuity
			continue
		}
		if prevVal != nil {
			delta := p.V - *prevVal
			if delta < 0 {
				delta = 0 // counter reset
			}
			rate := delta / stepSec
			out[i] = Point{T: p.T, V: rate}
		}
		v := p.V
		prevVal = &v
	}
	// If there was only one bucket (no prev), emit zero rate for it.
	if len(out) == 0 && len(last) == 1 {
		for idx, p := range last {
			out[idx] = Point{T: p.T, V: 0}
		}
	}
	return orderedBuckets(out)
}

// orderedBuckets returns bucket points sorted by time.
func orderedBuckets(buckets map[int]Point) []Point {
	if len(buckets) == 0 {
		return nil
	}
	maxIdx := -1
	for idx := range buckets {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	out := make([]Point, 0, len(buckets))
	for i := 0; i <= maxIdx; i++ {
		if p, ok := buckets[i]; ok {
			out = append(out, p)
		}
	}
	return out
}
