// Package container exposes live container listing, inspection and lifecycle
// actions on a stored Docker host. Exec, logs and create are out of scope.
package container

import (
	"context"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerapi "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

const resource = "container"

// Container IDs or names; Engine names match [a-zA-Z0-9][a-zA-Z0-9_.-]+.
var containerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`)

// Action is a lifecycle verb accepted by Do.
type Action string

const (
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"
)

type Service struct {
	connect dockerssh.Connector
	audit   *audit.Recorder
}

func NewService(connect dockerssh.Connector, rec *audit.Recorder) *Service {
	return &Service{connect: connect, audit: rec}
}

// List returns every container, including stopped ones, newest first.
func (s *Service) List(ctx context.Context, actorID, hostID uint64) (*ListResponse, error) {
	var raw []dockercontainer.Summary
	_, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.container.list", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			var err error
			raw, err = cli.ContainerList(ctx, dockercontainer.ListOptions{All: true})
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

// Get returns the inspect summary of one container.
func (s *Service) Get(ctx context.Context, actorID, hostID uint64, containerID string) (*Detail, error) {
	if err := validateID(containerID); err != nil {
		return nil, err
	}
	var raw dockercontainer.InspectResponse
	_, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.container.get", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			var err error
			raw, err = cli.ContainerInspect(ctx, containerID)
			return err
		})
	if err != nil {
		return nil, err
	}
	return toDetail(&raw), nil
}

// Do runs one lifecycle action. Starting a running or stopping a stopped
// container is a no-op success (Engine 304).
func (s *Service) Do(ctx context.Context, actorID uint64, ip, ua string, hostID uint64, containerID string, action Action) error {
	if err := validateID(containerID); err != nil {
		return err
	}
	var run func(context.Context, *dockerapi.Client) error
	switch action {
	case ActionStart:
		run = func(ctx context.Context, cli *dockerapi.Client) error {
			return cli.ContainerStart(ctx, containerID, dockercontainer.StartOptions{})
		}
	case ActionStop:
		run = func(ctx context.Context, cli *dockerapi.Client) error {
			return cli.ContainerStop(ctx, containerID, dockercontainer.StopOptions{})
		}
	case ActionRestart:
		run = func(ctx context.Context, cli *dockerapi.Client) error {
			return cli.ContainerRestart(ctx, containerID, dockercontainer.StopOptions{})
		}
	default:
		return apperr.New(apperr.CodeValidation, "docker.container.action_invalid", "unsupported container action")
	}
	h, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.container."+string(action), resource, run)
	if err != nil {
		return err
	}
	s.writeAudit(ctx, actorID, ip, ua, "docker.container."+string(action), containerID, h, map[string]any{})
	return nil
}

// Remove deletes a container. Without force the Engine refuses running
// containers (409 → 44105). Named and anonymous volumes are kept.
func (s *Service) Remove(ctx context.Context, actorID uint64, ip, ua string, hostID uint64, containerID string, force bool) error {
	if err := validateID(containerID); err != nil {
		return err
	}
	h, err := dockerssh.Session(ctx, s.connect, actorID, hostID, "docker.container.delete", resource,
		func(ctx context.Context, cli *dockerapi.Client) error {
			return cli.ContainerRemove(ctx, containerID, dockercontainer.RemoveOptions{Force: force})
		})
	if err != nil {
		return err
	}
	s.writeAudit(ctx, actorID, ip, ua, "docker.container.delete", containerID, h, map[string]any{"force": force})
	return nil
}

// writeAudit records a successful mutation against the host; the container
// reference lives in the payload because target_id is limited to 64 chars.
func (s *Service) writeAudit(ctx context.Context, actorID uint64, ip, ua, action, containerID string, h *models.DockerHost, payload map[string]any) {
	if s.audit == nil || h == nil {
		return
	}
	payload["host_name"] = h.Name
	payload["container"] = containerID
	var uid *uint64
	if actorID != 0 {
		uid = &actorID
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: uid, Action: action, TargetType: "docker.host", TargetID: strconv.FormatUint(h.ID, 10),
		Payload: payload, IP: ip, UserAgent: ua,
	})
}

func validateID(id string) error {
	if !containerIDPattern.MatchString(id) {
		return apperr.New(apperr.CodeValidation, "docker.container.id_invalid", "container ID or name is invalid")
	}
	return nil
}

func toSummary(m *dockercontainer.Summary) Summary {
	name := ""
	if len(m.Names) > 0 {
		name = strings.TrimPrefix(m.Names[0], "/")
	}
	ports := make([]string, 0, len(m.Ports))
	for _, p := range m.Ports {
		ports = append(ports, formatPort(p.IP, p.PublicPort, p.PrivatePort, p.Type))
	}
	return Summary{
		ID:      m.ID,
		ShortID: dockerssh.ShortID(m.ID),
		Name:    name,
		Image:   m.Image,
		ImageID: dockerssh.ShortID(m.ImageID),
		Command: m.Command,
		State:   m.State,
		Status:  m.Status,
		Ports:   sortedUnique(ports),
		Created: time.Unix(m.Created, 0).UTC(),
	}
}

func toDetail(m *dockercontainer.InspectResponse) *Detail {
	out := &Detail{
		Command:  []string{},
		Ports:    []string{},
		Networks: []Network{},
		Mounts:   []Mount{},
		Env:      []string{},
	}
	if b := m.ContainerJSONBase; b != nil {
		out.ID = b.ID
		out.ShortID = dockerssh.ShortID(b.ID)
		out.Name = strings.TrimPrefix(b.Name, "/")
		out.ImageID = dockerssh.ShortID(b.Image)
		out.Created = parseTime(b.Created)
		out.RestartCount = b.RestartCount
		if b.Path != "" {
			out.Command = append([]string{b.Path}, b.Args...)
		}
		if st := b.State; st != nil {
			out.State = State{
				Status: st.Status, Running: st.Running, Paused: st.Paused, Restarting: st.Restarting,
				OOMKilled: st.OOMKilled, Dead: st.Dead, ExitCode: st.ExitCode, Error: st.Error,
				StartedAt: parseTime(st.StartedAt), FinishedAt: parseTime(st.FinishedAt),
			}
			if st.Health != nil {
				out.State.Health = st.Health.Status
			}
		}
		if hc := b.HostConfig; hc != nil {
			out.RestartPolicy = RestartPolicy{Name: string(hc.RestartPolicy.Name), MaximumRetryCount: hc.RestartPolicy.MaximumRetryCount}
		}
	}
	if c := m.Config; c != nil {
		out.Image = c.Image
		if c.Env != nil {
			out.Env = c.Env
		}
	}
	if ns := m.NetworkSettings; ns != nil {
		out.Ports = formatPortMap(ns.Ports)
		names := make([]string, 0, len(ns.Networks))
		for n := range ns.Networks {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			ep := ns.Networks[n]
			if ep == nil {
				continue
			}
			out.Networks = append(out.Networks, Network{
				Name: n, IPAddress: ep.IPAddress, IPv6Address: ep.GlobalIPv6Address,
				Gateway: ep.Gateway, MacAddress: ep.MacAddress,
			})
		}
	}
	for _, mp := range m.Mounts {
		out.Mounts = append(out.Mounts, Mount{
			Type: string(mp.Type), Name: mp.Name, Source: mp.Source,
			Destination: mp.Destination, Mode: mp.Mode, RW: mp.RW,
		})
	}
	return out
}

func formatPortMap(pm nat.PortMap) []string {
	out := make([]string, 0, len(pm))
	for port, bindings := range pm {
		private, _ := strconv.ParseUint(port.Port(), 10, 16)
		if len(bindings) == 0 {
			out = append(out, formatPort("", 0, uint16(private), port.Proto()))
			continue
		}
		for _, b := range bindings {
			public, _ := strconv.ParseUint(b.HostPort, 10, 16)
			out = append(out, formatPort(b.HostIP, uint16(public), uint16(private), port.Proto()))
		}
	}
	return sortedUnique(out)
}

// formatPort renders one mapping the way `docker ps` does.
func formatPort(ip string, public, private uint16, proto string) string {
	tail := strconv.Itoa(int(private)) + "/" + proto
	if public == 0 {
		return tail
	}
	if ip == "" {
		return strconv.Itoa(int(public)) + "->" + tail
	}
	return net.JoinHostPort(ip, strconv.Itoa(int(public))) + "->" + tail
}

func sortedUnique(s []string) []string {
	slices.Sort(s)
	return slices.Compact(s)
}

// parseTime converts Engine RFC 3339 timestamps; the zero time Docker uses
// for "never" becomes nil.
func parseTime(v string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil || t.IsZero() || t.Year() <= 1 {
		return nil
	}
	t = t.UTC()
	return &t
}
