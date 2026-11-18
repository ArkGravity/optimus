// Package image exposes live image listing, deletion and pruning on a stored
// Docker host. Nothing is persisted; every call opens a fresh tunnel.
package image

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerapi "github.com/docker/docker/client"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

const resource = "image"

// Only content-addressed IDs are accepted so a path segment can never name a
// different image through a mutable tag.
var imageIDPattern = regexp.MustCompile(`^(sha256:)?[a-f0-9]{12,64}$`)

type Service struct {
	connect dockerssh.Connector
	audit   *audit.Recorder
}

func NewService(connect dockerssh.Connector, rec *audit.Recorder) *Service {
	return &Service{connect: connect, audit: rec}
}

// List returns every top-level image, newest first.
func (s *Service) List(ctx context.Context, actorID, hostID uint64) (*ListResponse, error) {
	var raw []dockerimage.Summary
	_, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.image.list", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			var err error
			raw, err = cli.ImageList(ctx, dockerimage.ListOptions{})
			return err
		})
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(raw))
	for i := range raw {
		items = append(items, toSummary(&raw[i]))
	}
	slices.SortStableFunc(items, func(a, b Summary) int { return b.Created.Compare(a.Created) })
	return &ListResponse{Items: items, Total: len(items)}, nil
}

// Remove deletes one image by ID. Without force, the Engine refuses images
// referenced by containers or by several tags (409 → 44105).
func (s *Service) Remove(ctx context.Context, actorID uint64, ip, ua string, hostID uint64, imageID string, force bool) (*DeleteResponse, error) {
	if !imageIDPattern.MatchString(imageID) {
		return nil, apperr.New(apperr.CodeValidation, "docker.image.id_invalid", "image ID must be a hexadecimal image ID")
	}
	var raw []dockerimage.DeleteResponse
	h, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.image.delete", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			var err error
			raw, err = cli.ImageRemove(ctx, imageID, dockerimage.RemoveOptions{Force: force, PruneChildren: true})
			return err
		})
	if err != nil {
		return nil, err
	}
	out := &DeleteResponse{}
	out.Untagged, out.Deleted = splitDeletes(raw)
	s.writeAudit(ctx, actorID, ip, ua, "docker.image.delete", imageID, h, map[string]any{
		"force":    force,
		"untagged": len(out.Untagged),
		"deleted":  len(out.Deleted),
	})
	return out, nil
}

// Prune removes dangling images, or every unused image when all is set.
func (s *Service) Prune(ctx context.Context, actorID uint64, ip, ua string, hostID uint64, all bool) (*PruneResponse, error) {
	var report dockerimage.PruneReport
	h, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.image.prune", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			var err error
			// Matches `docker image prune [-a]`: dangling=false widens to unused.
			report, err = cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", strconv.FormatBool(!all))))
			return err
		})
	if err != nil {
		return nil, err
	}
	out := &PruneResponse{SpaceReclaimed: report.SpaceReclaimed}
	out.Untagged, out.Deleted = splitDeletes(report.ImagesDeleted)
	s.writeAudit(ctx, actorID, ip, ua, "docker.image.prune", "", h, map[string]any{
		"all":             all,
		"untagged":        len(out.Untagged),
		"deleted":         len(out.Deleted),
		"space_reclaimed": out.SpaceReclaimed,
	})
	return out, nil
}

// writeAudit records a successful mutation against the host; the image
// reference lives in the payload because target_id is limited to 64 chars.
func (s *Service) writeAudit(ctx context.Context, actorID uint64, ip, ua, action, imageID string, h *models.DockerHost, payload map[string]any) {
	if s.audit == nil || h == nil {
		return
	}
	payload["host_name"] = h.Name
	if imageID != "" {
		payload["image_id"] = imageID
	}
	var uid *uint64
	if actorID != 0 {
		uid = &actorID
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: uid, Action: action, TargetType: "docker.host", TargetID: strconv.FormatUint(h.ID, 10),
		Payload: payload, IP: ip, UserAgent: ua,
	})
}

func toSummary(m *dockerimage.Summary) Summary {
	tags := make([]string, 0, len(m.RepoTags))
	for _, t := range m.RepoTags {
		if t != "<none>:<none>" {
			tags = append(tags, t)
		}
	}
	return Summary{
		ID:       m.ID,
		ShortID:  dockerssh.ShortID(m.ID),
		RepoTags: tags,
		Dangling: len(tags) == 0,
		Size:     m.Size,
		Created:  time.Unix(m.Created, 0).UTC(),
	}
}

func splitDeletes(raw []dockerimage.DeleteResponse) (untagged, deleted []string) {
	untagged, deleted = []string{}, []string{}
	for _, r := range raw {
		if r.Untagged != "" {
			untagged = append(untagged, r.Untagged)
		}
		if r.Deleted != "" {
			deleted = append(deleted, r.Deleted)
		}
	}
	slices.Sort(untagged)
	slices.Sort(deleted)
	return untagged, deleted
}
