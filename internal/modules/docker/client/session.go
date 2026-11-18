package client

import (
	"context"
	"errors"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	dockerclient "github.com/docker/docker/client"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
)

// OpTimeout bounds one live operation, including the SSH connect.
const OpTimeout = 30 * time.Second

// Connector opens a tunnelled client for one stored host. Errors must already
// be client-safe; cleanup is non-nil only on success. host.Service.Connect
// satisfies this signature.
type Connector func(ctx context.Context, actorID, hostID uint64, purpose string) (*models.DockerHost, *dockerclient.Client, func(), error)

// Session runs fn against a fresh tunnelled client and always tears the
// tunnel down. Engine failures are mapped through ResourceError. The host is
// returned whenever it was loaded so callers can attribute audit events.
func Session(ctx context.Context, connect Connector, actorID, hostID uint64, purpose, resource string,
	fn func(context.Context, *dockerclient.Client) error,
) (*models.DockerHost, error) {
	ctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	h, cli, cleanup, err := connect(ctx, actorID, hostID, purpose)
	if err != nil {
		return h, err
	}
	defer cleanup()
	if err := fn(ctx, cli); err != nil {
		return h, ResourceError(err, resource)
	}
	return h, nil
}

// ShortID returns the 12-character display form of an Engine object ID.
func ShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ResourceError maps an Engine failure on one resource kind ("image",
// "container") to a safe envelope. Transport failures keep their dial kind.
func ResourceError(err error, resource string) error {
	if err == nil {
		return nil
	}
	var de *Error
	if !errors.As(err, &de) {
		switch {
		case cerrdefs.IsNotFound(err):
			return apperr.Wrap(err, apperr.CodeNotFound, "docker."+resource+".not_found",
				"Docker "+resource+" not found")
		case cerrdefs.IsConflict(err):
			return apperr.Wrap(err, apperr.CodeDockerResourceConflict, "docker."+resource+".conflict",
				"Docker "+resource+" is in use or in a conflicting state")
		}
	}
	return AppError(err)
}
