// Package client builds per-request Docker Engine clients that reach the
// remote host's Docker Unix socket through an SSH connection. Nothing is
// cached: every call authenticates, tunnels, and is torn down by cleanup.
package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	dockerclient "github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
)

// Timeout bounds the TCP connect plus SSH handshake. Callers should also
// bound each Engine API call with their own context deadline.
const Timeout = 20 * time.Second

// DefaultSocketPath is used when a host row has no explicit socket path.
const DefaultSocketPath = "/var/run/docker.sock"

// Auth types accepted on docker_hosts.auth_type.
const (
	AuthSSHKey   = "ssh_key"
	AuthPassword = "password"
)

// Material carries decrypted secret material for exactly one Dial. Callers
// must call Wipe once Dial returns.
type Material struct {
	PrivateKey []byte
	Passphrase []byte
	Password   []byte
}

// Wipe zeroes the secret buffers. Best-effort: parsed signers and Go strings
// derived during the handshake may persist until garbage collection.
func (m *Material) Wipe() {
	for _, b := range [][]byte{m.PrivateKey, m.Passphrase, m.Password} {
		for i := range b {
			b[i] = 0
		}
	}
	m.PrivateKey, m.Passphrase, m.Password = nil, nil, nil
}

// Kind classifies a dial or Engine failure into a client-safe category.
type Kind int

const (
	KindConfig Kind = iota + 1
	KindUnreachable
	KindAuth
	KindHandshake
	KindSocket
	KindTimeout
	KindEngine
)

// Error wraps a raw transport error with its Kind. Error() never includes the
// raw cause so it is safe to persist; use errors.Unwrap for server logs.
type Error struct {
	Kind Kind
	err  error
}

func (e *Error) Error() string { return SafeMessage(e.Kind) }
func (e *Error) Unwrap() error { return e.err }

// SafeMessage is the fixed, credential-free text for each failure kind.
func SafeMessage(k Kind) string {
	switch k {
	case KindConfig:
		return "invalid SSH connection configuration"
	case KindUnreachable:
		return "SSH host unreachable"
	case KindAuth:
		return "SSH authentication failed"
	case KindHandshake:
		return "SSH handshake failed"
	case KindSocket:
		return "Docker socket unavailable on host"
	case KindTimeout:
		return "connection to Docker host timed out"
	case KindEngine:
		return "Docker Engine request failed"
	default:
		return "Docker host connection failed"
	}
}

// Classify maps any error from Dial or a Docker client call to a Kind.
func Classify(err error) Kind {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	return KindEngine
}

// AppError converts a dial/Engine failure into the fixed response envelope
// without exposing raw SSH or Docker payloads.
func AppError(err error) error {
	if err == nil {
		return nil
	}
	k := Classify(err)
	msg := SafeMessage(k)
	switch k {
	case KindConfig:
		return apperr.Wrap(err, apperr.CodeDockerHostConfigInvalid, "docker.host.config_invalid", msg)
	case KindAuth:
		return apperr.Wrap(err, apperr.CodeDockerHostAuthFailed, "docker.host.auth_failed", msg)
	case KindEngine:
		return apperr.Wrap(err, apperr.CodeDockerEngineError, "docker.engine.error", msg)
	default:
		return apperr.Wrap(err, apperr.CodeDockerHostUnreachable, "docker.host.unreachable", msg)
	}
}

// SSHConfig builds the SSH client configuration for one host. Only private
// key or password authentication is supported; no agents or external helpers.
func SSHConfig(h *models.DockerHost, m Material) (*ssh.ClientConfig, error) {
	if h == nil || strings.TrimSpace(h.Username) == "" {
		return nil, &Error{Kind: KindConfig, err: errors.New("missing host or username")}
	}
	var auth []ssh.AuthMethod
	switch h.AuthType {
	case AuthSSHKey:
		signer, err := parseSigner(m.PrivateKey, m.Passphrase)
		if err != nil {
			return nil, &Error{Kind: KindConfig, err: err}
		}
		auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case AuthPassword:
		if len(m.Password) == 0 {
			return nil, &Error{Kind: KindConfig, err: errors.New("missing password")}
		}
		pw := string(m.Password)
		auth = []ssh.AuthMethod{
			ssh.Password(pw),
			// Many servers only expose password login as keyboard-interactive.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					if !echos[i] {
						answers[i] = pw
					}
				}
				return answers, nil
			}),
		}
	default:
		return nil, &Error{Kind: KindConfig, err: errors.New("unsupported auth type")}
	}
	return &ssh.ClientConfig{
		User: h.Username,
		Auth: auth,
		// Host key fingerprint pinning is a planned follow-up; until a
		// fingerprint is stored per host, the host key is not verified.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         Timeout,
	}, nil
}

func parseSigner(key, passphrase []byte) (ssh.Signer, error) {
	if len(key) == 0 {
		return nil, errors.New("missing private key")
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, nil
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return nil, err
	}
	if len(passphrase) == 0 {
		return nil, errors.New("private key is encrypted but no passphrase is stored")
	}
	return ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
}

// Dial opens an SSH connection to the host and returns a Docker client whose
// HTTP transport tunnels every request to the remote socket. cleanup closes
// the Docker client and the SSH connection and is safe to call repeatedly.
func Dial(ctx context.Context, h *models.DockerHost, m Material) (*dockerclient.Client, func(), error) {
	cfg, err := SSHConfig(h, m)
	if err != nil {
		return nil, nil, err
	}
	sshClient, err := dialSSH(ctx, net.JoinHostPort(h.Host, strconv.Itoa(h.Port)), cfg)
	if err != nil {
		return nil, nil, err
	}
	socket := h.SocketPath
	if socket == "" {
		socket = DefaultSocketPath
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			conn, err := sshClient.DialContext(ctx, "unix", socket)
			if err != nil {
				return nil, &Error{Kind: KindSocket, err: err}
			}
			return conn, nil
		},
		IdleConnTimeout: 30 * time.Second,
	}
	// WithHost must precede WithHTTPClient: WithHost reconfigures the default
	// transport for a local socket, which the tunnelled client then replaces.
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost("unix://"+socket),
		dockerclient.WithHTTPClient(&http.Client{Transport: transport, CheckRedirect: dockerclient.CheckRedirect}),
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, &Error{Kind: KindConfig, err: err}
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = cli.Close()
			transport.CloseIdleConnections()
			_ = sshClient.Close()
		})
	}
	return cli, cleanup, nil
}

func dialSSH(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, &Error{Kind: KindTimeout, err: err}
		}
		return nil, &Error{Kind: KindUnreachable, err: err}
	}
	// The SSH handshake has no context parameter; closing the socket on
	// expiry unblocks it.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if !stop() {
		if err == nil {
			_ = c.Close()
		}
		return nil, &Error{Kind: KindTimeout, err: ctx.Err()}
	}
	if err != nil {
		_ = conn.Close()
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, &Error{Kind: KindAuth, err: err}
		}
		return nil, &Error{Kind: KindHandshake, err: err}
	}
	return ssh.NewClient(c, chans, reqs), nil
}
