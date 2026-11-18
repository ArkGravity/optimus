package client_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	dockerapi "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

func TestSession_BoundsContextAndAlwaysCleansUp(t *testing.T) {
	cleanups := 0
	connect := func(ctx context.Context, actorID, hostID uint64, purpose string) (*models.DockerHost, *dockerapi.Client, func(), error) {
		dl, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(dl), dockerssh.OpTimeout)
		require.Equal(t, uint64(3), actorID)
		require.Equal(t, uint64(9), hostID)
		require.Equal(t, "docker.image.list", purpose)
		return &models.DockerHost{ID: hostID}, &dockerapi.Client{}, func() { cleanups++ }, nil
	}

	h, err := dockerssh.Session(context.Background(), connect, 3, 9, "docker.image.list", "image",
		func(context.Context, *dockerapi.Client) error { return nil })
	require.NoError(t, err)
	require.Equal(t, uint64(9), h.ID)

	h, err = dockerssh.Session(context.Background(), connect, 3, 9, "docker.image.list", "image",
		func(context.Context, *dockerapi.Client) error { return fmt.Errorf("wrapped: %w", cerrdefs.ErrNotFound) })
	require.NotNil(t, h, "host is returned for audit attribution")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
	require.Equal(t, 2, cleanups)
}

func TestSession_ConnectErrorPassesThroughWithoutCleanup(t *testing.T) {
	safe := apperr.New(apperr.CodeNotFound, "docker.host.not_found", "Docker host not found")
	connect := func(context.Context, uint64, uint64, string) (*models.DockerHost, *dockerapi.Client, func(), error) {
		return nil, nil, nil, safe
	}
	called := false
	_, err := dockerssh.Session(context.Background(), connect, 1, 1, "p", "image",
		func(context.Context, *dockerapi.Client) error { called = true; return nil })
	require.Same(t, safe, err)
	require.False(t, called)
}

func TestResourceError_MapsEngineConditions(t *testing.T) {
	require.NoError(t, dockerssh.ResourceError(nil, "image"))

	cases := []struct {
		err  error
		code apperr.Code
		key  string
	}{
		{fmt.Errorf("No such container: x: %w", cerrdefs.ErrNotFound), apperr.CodeNotFound, "docker.container.not_found"},
		{fmt.Errorf("is running: %w", cerrdefs.ErrConflict), apperr.CodeDockerResourceConflict, "docker.container.conflict"},
		{context.DeadlineExceeded, apperr.CodeDockerHostUnreachable, "docker.host.unreachable"},
		{errors.New("raw engine text"), apperr.CodeDockerEngineError, "docker.engine.error"},
		{&dockerssh.Error{Kind: dockerssh.KindSocket}, apperr.CodeDockerHostUnreachable, "docker.host.unreachable"},
	}
	for _, tc := range cases {
		be, ok := apperr.AsBiz(dockerssh.ResourceError(tc.err, "container"))
		require.True(t, ok, tc.key)
		require.Equal(t, tc.code, be.Code, tc.key)
		require.Equal(t, tc.key, be.MessageKey)
		require.NotContains(t, be.Message, "raw engine text")
	}
}

func TestClassify_EngineConnectionFailureIsSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	cli, err := dockerapi.NewClientWithOpts(dockerapi.WithHost("tcp://"+addr), dockerapi.WithVersion("1.47"))
	require.NoError(t, err)
	defer cli.Close()
	_, err = cli.ServerVersion(context.Background())
	require.Error(t, err)
	require.Equal(t, dockerssh.KindSocket, dockerssh.Classify(err))
}

func TestShortID(t *testing.T) {
	require.Equal(t, "4f67c83422ec", dockerssh.ShortID("sha256:4f67c83422ec747235357c04556616234e66fc3fa39cb4f40b2d4441ddd8f100"))
	require.Equal(t, "9f86d081884c", dockerssh.ShortID("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"))
	require.Equal(t, "abc", dockerssh.ShortID("abc"))
}
