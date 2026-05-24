package minio_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

func TestPoolGetBeforeRebuild(t *testing.T) {
	p := hmminio.NewEmpty()
	_, _, err := p.Get(context.Background())
	require.ErrorIs(t, err, hmminio.ErrNotInitialized)
}

func TestPoolRebuildSwapsClients(t *testing.T) {
	p := hmminio.NewEmpty()
	require.NoError(t, p.Rebuild(hmminio.Credentials{
		EndpointURL: "https://minio.example.test:9000",
		AccessKey:   "AKIA",
		SecretKey:   "SECRET",
	}))
	madm, mc, err := p.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, madm)
	require.NotNil(t, mc)
}
