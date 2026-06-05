package metrics

import "time"

// viewResponse is the plain-JSON shape returned by GET /api/v1/metrics.
type viewResponse struct {
	Window      string               `json:"window"`
	StepSeconds int                  `json:"step_seconds"`
	Collected   bool                 `json:"collected"`
	Series      map[string][]pointWire `json:"series"`
}

// pointWire is a single downsampled point in the REST response.
type pointWire struct {
	T string  `json:"t"`
	V float64 `json:"v"`
}

// toResponse converts an aggregated View into the wire representation.
// Point timestamps are formatted as RFC 3339.
func toResponse(v View) viewResponse {
	series := make(map[string][]pointWire, len(v.Series))
	for metric, pts := range v.Series {
		wire := make([]pointWire, len(pts))
		for i, p := range pts {
			wire[i] = pointWire{T: p.T.UTC().Format(time.RFC3339), V: p.V}
		}
		series[metric] = wire
	}
	return viewResponse{
		Window:      string(v.Window),
		StepSeconds: v.StepSeconds,
		Collected:   v.Collected,
		Series:      series,
	}
}
