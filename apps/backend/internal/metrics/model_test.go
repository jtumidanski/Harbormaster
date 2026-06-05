package metrics

import "testing"

func TestParseWindow(t *testing.T) {
	cases := map[string]bool{"1h": true, "6h": true, "24h": true, "7d": true, "": false, "30d": false, "bogus": false}
	for in, ok := range cases {
		w, err := ParseWindow(in)
		if ok && err != nil {
			t.Errorf("ParseWindow(%q) unexpected error: %v", in, err)
		}
		if !ok && err == nil {
			t.Errorf("ParseWindow(%q) expected error", in)
		}
		if ok && w.Duration() <= 0 {
			t.Errorf("ParseWindow(%q) duration must be positive", in)
		}
	}
}

func TestWindowStep(t *testing.T) {
	// step must keep each series at <= ~300 points.
	for _, in := range []string{"1h", "6h", "24h", "7d"} {
		w, _ := ParseWindow(in)
		points := int(w.Duration() / w.Step())
		if points > 300 {
			t.Errorf("window %q yields %d points (>300)", in, points)
		}
	}
}
