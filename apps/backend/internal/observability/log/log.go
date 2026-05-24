// Package log wraps zerolog with a context-bound logger usable across the codebase.
// All emit sites should go through this package (or via FromCtx) so log format
// and level remain consistent and so secret-scrubbing lint can statically locate
// every log call.
package log

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog"
)

type ctxKey struct{}

// NewWith builds a logger from level ("debug" | "info" | "warn" | "error") and
// format ("json" | "console") writing to w. Caller owns w's lifecycle.
func NewWith(level, format string, w io.Writer) (zerolog.Logger, error) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Nop(), fmt.Errorf("invalid log level %q: %w", level, err)
	}
	var out io.Writer = w
	if format == "console" {
		out = zerolog.ConsoleWriter{Out: w}
	}
	return zerolog.New(out).Level(lvl).With().Timestamp().Caller().Logger(), nil
}

// New is a convenience that writes to stderr.
func New(level, format string) (zerolog.Logger, error) {
	return NewWith(level, format, os.Stderr)
}

// WithLogger attaches the logger to the context.
func WithLogger(ctx context.Context, l zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromCtx returns the logger attached to ctx, or a Nop logger if none.
func FromCtx(ctx context.Context) zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}
