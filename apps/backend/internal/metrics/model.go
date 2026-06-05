package metrics

import (
	"errors"
	"time"
)

// Window is a validated dashboard time window.
type Window string

const (
	Window1h  Window = "1h"
	Window6h  Window = "6h"
	Window24h Window = "24h"
	Window7d  Window = "7d"
)

// ErrInvalidWindow is returned by ParseWindow for an unrecognized value.
var ErrInvalidWindow = errors.New("invalid metrics window")

// ParseWindow validates a raw window string.
func ParseWindow(s string) (Window, error) {
	switch Window(s) {
	case Window1h, Window6h, Window24h, Window7d:
		return Window(s), nil
	default:
		return "", ErrInvalidWindow
	}
}

// Duration is the wall-clock span of the window.
func (w Window) Duration() time.Duration {
	switch w {
	case Window1h:
		return time.Hour
	case Window6h:
		return 6 * time.Hour
	case Window24h:
		return 24 * time.Hour
	case Window7d:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// Step is the downsample bucket width, chosen so each series stays at
// <= ~300 points (design §5.2).
// 1h→60s (60 pts), 6h→300s (72 pts), 24h→300s (288 pts), 7d→2400s (252 pts).
func (w Window) Step() time.Duration {
	switch w {
	case Window1h:
		return 60 * time.Second // 60 points
	case Window6h:
		return 300 * time.Second // 72 points
	case Window24h:
		return 300 * time.Second // 288 points
	case Window7d:
		return 2400 * time.Second // 252 points (1800s would yield 336 > 300)
	default:
		return time.Minute
	}
}

// Point is one downsampled value at time T.
type Point struct {
	T time.Time
	V float64
}

// MetricsView is the aggregated dashboard payload.
type MetricsView struct {
	Window      Window
	StepSeconds int
	Collected   bool
	Series      map[string][]Point
}
