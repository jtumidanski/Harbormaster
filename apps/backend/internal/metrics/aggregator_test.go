package metrics

import (
	"testing"
	"time"
)

func TestAggregateCounterRate(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// counter rising 0 → 60 over 60s ⇒ rate ~1/s
	raw := map[string][]Point{
		"minio_s3_requests_total": {
			{T: base, V: 0},
			{T: base.Add(60 * time.Second), V: 60},
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	pts := view.Series["minio_s3_requests_total"]
	if len(pts) == 0 {
		t.Fatal("expected rate points")
	}
	// last non-zero rate bucket should be ~1.0/s
	var maxRate float64
	for _, p := range pts {
		if p.V > maxRate {
			maxRate = p.V
		}
	}
	if maxRate < 0.8 || maxRate > 1.2 {
		t.Errorf("expected ~1/s rate, got %v", maxRate)
	}
}

func TestAggregateCounterResetClampedToZero(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := map[string][]Point{
		"minio_s3_requests_total": {
			{T: base, V: 100},
			{T: base.Add(60 * time.Second), V: 5}, // counter reset
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	for _, p := range view.Series["minio_s3_requests_total"] {
		if p.V < 0 {
			t.Errorf("reset must clamp to >= 0, got %v", p.V)
		}
	}
}

func TestAggregateGaugePassthrough(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := map[string][]Point{
		"minio_cluster_drive_online_total": {
			{T: base, V: 4},
			{T: base.Add(60 * time.Second), V: 4},
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	pts := view.Series["minio_cluster_drive_online_total"]
	if len(pts) == 0 || pts[len(pts)-1].V != 4 {
		t.Errorf("gauge should pass through as 4, got %+v", pts)
	}
}
