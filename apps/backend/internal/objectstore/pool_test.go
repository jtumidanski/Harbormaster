package objectstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/objectstore"
)

func TestPoolGetBeforeRebuild(t *testing.T) {
	p := objectstore.NewEmpty()
	_, _, err := p.Get(context.Background())
	require.ErrorIs(t, err, objectstore.ErrNotInitialized)
}

func TestPoolRebuildSwapsClients(t *testing.T) {
	p := objectstore.NewEmpty()
	require.NoError(t, p.Rebuild(objectstore.Credentials{
		EndpointURL: "https://minio.example.test:9000",
		AccessKey:   "AKIA",
		SecretKey:   "SECRET",
	}))
	madm, mc, err := p.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, madm)
	require.NotNil(t, mc)
}
