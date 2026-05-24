// Package dashboard owns the cross-domain aggregate that powers
// GET /api/v1/dashboard. It is the documented exception to the DDD
// boundary rule: a dashboard handler may call several processors
// directly because its only purpose is to assemble a read-only view
// (see backend-dev-guidelines/resources/architecture-overview.md
// §Cross-Domain Orchestration). There is no domain entity, no local
// persistence, and no mutation path here.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
)

// Window is the failures-widget time horizon. The wire string ("24h" /
// "7d" / "30d") is the canonical form; Window values are used internally
// and re-emitted via the FailuresWidget.Window field.
type Window string

const (
	Window24h Window = "24h"
	Window7d  Window = "7d"
	Window30d Window = "30d"
)

// defaultWindow is applied when the request omits failures_window. 7d
// matches the api-contracts.md default and gives operators enough history
// to spot the kind of slow-burn failures that motivate the widget without
// dragging in a month of noise.
const defaultWindow = Window7d

// ErrInvalidWindow is returned by Parse when the input is neither empty
// nor one of the documented choices. The HTTP layer surfaces this as the
// typed 422 `invalid_failures_window` envelope.
var ErrInvalidWindow = errors.New("invalid_failures_window")

// Parse converts a wire failures_window string into a Window value. An
// empty input defaults to defaultWindow. Any other unrecognised value
// returns ErrInvalidWindow.
func Parse(s string) (Window, error) {
	switch s {
	case "":
		return defaultWindow, nil
	case string(Window24h), string(Window7d), string(Window30d):
		return Window(s), nil
	}
	return "", ErrInvalidWindow
}

// Duration returns the time.Duration this Window covers.
func (w Window) Duration() time.Duration {
	switch w {
	case Window24h:
		return 24 * time.Hour
	case Window30d:
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// recentActivityLimit and failuresEntriesLimit are the documented caps
// from api-contracts.md §dashboard: the recent-activity feed is capped at
// 25 entries; the failures widget includes at most 10 entries alongside
// the total count.
const (
	recentActivityLimit  = 25
	failuresEntriesLimit = 10
)

// View is the full dashboard aggregate returned by Build.
// Field order and JSON tags match api-contracts.md §dashboard exactly so
// the response is contract-correct without an intermediate DTO. The
// activity / failure rows are projected into transport-shaped helper
// structs (see rest.go) because audit.Event is the domain type and
// intentionally carries no JSON tags.
type View struct {
	Server         ServerInfo     `json:"server"`
	Totals         Totals         `json:"totals"`
	Nodes          []NodeStatus   `json:"nodes"`
	Warnings       []string       `json:"warnings"`
	RecentActivity []EventSummary `json:"recent_activity"`
	RecentFailures FailuresWidget `json:"recent_failures"`
}

// ServerInfo summarises the active MinIO target. Sourced from
// madmin.ServerInfo via the PoolGetter adapter.
type ServerInfo struct {
	Version        string `json:"version"`
	DeploymentMode string `json:"deployment_mode"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}

// Totals aggregates per-bucket detail counts across the cluster. The
// counts are computed from buckets.Processor.List output so they reflect
// the same in-flight data the buckets endpoint shows.
type Totals struct {
	Buckets        int64 `json:"buckets"`
	EstimatedBytes int64 `json:"estimated_bytes"`
	Objects        int64 `json:"objects"`
}

// NodeStatus is the per-server row inside the dashboard's nodes[] block.
type NodeStatus struct {
	Endpoint string     `json:"endpoint"`
	State    string     `json:"state"`
	Drives   DriveCount `json:"drives"`
}

// DriveCount summarises the disks on a single server.
type DriveCount struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// FailuresWidget is the recent_failures block: a count + truncated entries.
// Count is the unfiltered total inside the window so the UI can render
// "N failures in 7d" alongside the most-recent failuresEntriesLimit rows.
type FailuresWidget struct {
	Window  Window           `json:"window"`
	Count   int64            `json:"count"`
	Entries []FailureSummary `json:"entries"`
}

// PoolGetter abstracts the MinIO admin pool. The HTTP layer supplies a
// thin adapter around the live pool; tests substitute a struct literal
// returning canned ServerInfo / nodes / warnings. Returning the warning
// slice up-front (rather than letting the dashboard derive it from the
// raw madmin payload) keeps the warning policy at the adapter boundary
// where the operator's notion of "concerning" lives.
type PoolGetter interface {
	ServerInfo(ctx context.Context) (ServerInfo, []NodeStatus, []string, error)
}

// BucketsLister is the narrow contract Build needs from the buckets
// domain. The live *buckets.Processor satisfies this directly; tests
// substitute a struct returning canned Bucket slices without spinning up
// a fake MinIO client.
type BucketsLister interface {
	List(ctx context.Context) ([]buckets.Bucket, error)
}

// AuditQuerier is the narrow contract Build needs from the audit
// processor. The live *audit.Processor satisfies this via the methods
// added in T5.2.
type AuditQuerier interface {
	Recent(ctx context.Context, limit int) ([]audit.Event, error)
	FailuresSince(ctx context.Context, cutoff time.Time, limit int) (int64, []audit.Event, error)
}

// Processor orchestrates the four read fan-outs that produce a View.
// Construct via NewProcessor; zero values are not supported because all
// three dependencies must be supplied.
type Processor struct {
	pool    PoolGetter
	bks     BucketsLister
	auditer AuditQuerier
	now     func() time.Time
}

// NewProcessor returns a Processor wired to the supplied dependencies.
// All three are required — Build panics with a typed error if a nil pool
// or bucket lister or audit querier slips through, which matches the
// composition-root invariant (the HTTP wire-up always supplies live
// implementations).
func NewProcessor(pool PoolGetter, bks BucketsLister, a AuditQuerier) *Processor {
	return &Processor{pool: pool, bks: bks, auditer: a, now: time.Now}
}

// withClock is exposed for tests so they can pin a deterministic "now"
// when asserting failures-window cutoff math. Not used in production.
func (p *Processor) withClock(now func() time.Time) *Processor {
	p.now = now
	return p
}

// Build runs the four read fan-outs in parallel under an errgroup and
// returns the assembled View. The first sub-call to fail cancels the
// shared context so the surviving goroutines abort their RPCs instead of
// finishing useless work. Per-goroutine writes target distinct fields, so
// no shared-mutation guard is required.
func (p *Processor) Build(ctx context.Context, w Window) (View, error) {
	if p.pool == nil || p.bks == nil || p.auditer == nil {
		return View{}, fmt.Errorf("dashboard: processor not fully configured")
	}

	var (
		server   ServerInfo
		nodes    []NodeStatus
		warnings []string
		bks      []buckets.Bucket
		recent   []audit.Event
		failures FailuresWidget
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		s, n, warn, err := p.pool.ServerInfo(gctx)
		if err != nil {
			return fmt.Errorf("dashboard server info: %w", err)
		}
		server = s
		nodes = n
		warnings = warn
		return nil
	})

	g.Go(func() error {
		out, err := p.bks.List(gctx)
		if err != nil {
			return fmt.Errorf("dashboard buckets list: %w", err)
		}
		bks = out
		return nil
	})

	g.Go(func() error {
		out, err := p.auditer.Recent(gctx, recentActivityLimit)
		if err != nil {
			return fmt.Errorf("dashboard recent activity: %w", err)
		}
		recent = out
		return nil
	})

	g.Go(func() error {
		cutoff := p.now().UTC().Add(-w.Duration())
		count, entries, err := p.auditer.FailuresSince(gctx, cutoff, failuresEntriesLimit)
		if err != nil {
			return fmt.Errorf("dashboard failures widget: %w", err)
		}
		summarised := make([]FailureSummary, len(entries))
		for i, e := range entries {
			summarised[i] = summariseFailure(e)
		}
		failures = FailuresWidget{Window: w, Count: count, Entries: summarised}
		return nil
	})

	if err := g.Wait(); err != nil {
		return View{}, err
	}

	activity := make([]EventSummary, len(recent))
	for i, e := range recent {
		activity[i] = summariseEvent(e)
	}

	view := View{
		Server:         server,
		Nodes:          ensureNodeSlice(nodes),
		Warnings:       ensureStringSlice(warnings),
		RecentActivity: ensureActivitySlice(activity),
		RecentFailures: failures,
	}
	for _, b := range bks {
		view.Totals.Buckets++
		view.Totals.EstimatedBytes += b.EstimatedBytes
		view.Totals.Objects += b.ObjectCount
	}
	// Ensure the failures entries slice marshals as [] rather than null
	// when no failures exist — the SPA expects an array shape.
	view.RecentFailures.Entries = ensureFailureSlice(view.RecentFailures.Entries)
	return view, nil
}

// ensureNodeSlice / ensureStringSlice / ensureActivitySlice /
// ensureFailureSlice guarantee the JSON-encoded form is `[]` instead of
// `null` for the documented array fields. The contract examples always
// show empty arrays, never null.
func ensureNodeSlice(s []NodeStatus) []NodeStatus {
	if s == nil {
		return []NodeStatus{}
	}
	return s
}

func ensureStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ensureActivitySlice(s []EventSummary) []EventSummary {
	if s == nil {
		return []EventSummary{}
	}
	return s
}

func ensureFailureSlice(s []FailureSummary) []FailureSummary {
	if s == nil {
		return []FailureSummary{}
	}
	return s
}
