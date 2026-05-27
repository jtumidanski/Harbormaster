package log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewJSONWriter(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewWith("info", "json", &buf)
	require.NoError(t, err)
	logger.Info().Str("key", "value").Msg("hello")
	require.Contains(t, buf.String(), `"key":"value"`)
	require.Contains(t, buf.String(), `"message":"hello"`)
	require.Contains(t, buf.String(), `"level":"info"`)
}

func TestCtxRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := NewWith("debug", "json", &buf)
	ctx := WithLogger(context.Background(), logger)
	l := FromCtx(ctx)
	l.Info().Msg("ping")
	require.True(t, strings.Contains(buf.String(), `"message":"ping"`))
}
