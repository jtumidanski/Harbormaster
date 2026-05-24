package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunPrintsHello(t *testing.T) {
	var out bytes.Buffer
	err := run(&out, []string{"harbormaster"})
	require.NoError(t, err)
	require.Contains(t, out.String(), "harbormaster placeholder")
}
