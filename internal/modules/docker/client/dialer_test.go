package client_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	dockerclient "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

const testPassword = "s3cret-pass"

type fakeSSHServer struct {
	addr      string
	mu        sync.Mutex
	socketsIn []string
}

func (s *fakeSSHServer) sockets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.socketsIn...)
}

// startFakeSSH runs an in-process SSH server that accepts testPassword or
// authorizedKey and forwards direct-streamlocal channels to local sockets.
func startFakeSSH(t *testing.T, authorizedKey ssh.PublicKey) *fakeSSHServer {
	t.Helper()
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	require.NoError(t, err)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == testPassword {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if authorizedKey != nil && string(k.Marshal()) == string(authorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	srv := &fakeSSHServer{addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(conn, cfg)
		}
	}()
	return srv
}

func (s *fakeSSHServer) serve(conn net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "direct-streamlocal@openssh.com" {
			_ = nc.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		var payload struct {
			SocketPath string
			Reserved0  string
			Reserved1  uint32
		}
		if err := ssh.Unmarshal(nc.ExtraData(), &payload); err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, "bad payload")
			continue
		}
		s.mu.Lock()
		s.socketsIn = append(s.socketsIn, payload.SocketPath)
		s.mu.Unlock()
		upstream, err := net.Dial("unix", payload.SocketPath)
		if err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, "connect failed")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			_ = upstream.Close()
			continue
		}
		go ssh.DiscardRequests(creqs)
		go func() {
			defer ch.Close()
			defer upstream.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(upstream, ch); done <- struct{}{} }()
			go func() { _, _ = io.Copy(ch, upstream); done <- struct{}{} }()
			<-done
		}()
	}
}

// startFakeDocker serves a minimal Engine API on a Unix socket.
func startFakeDocker(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dk")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.45")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Version": "27.0.0", "ApiVersion": "1.45"})
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}

func hostFor(t *testing.T, addr, auth, socket string) *models.DockerHost {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	p, err := strconv.Atoi(port)
	require.NoError(t, err)
	return &models.DockerHost{Host: host, Port: p, AuthType: auth, Username: "ops", SocketPath: socket}
}

func genKey(t *testing.T, passphrase string) ([]byte, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var blk *pem.Block
	if passphrase == "" {
		blk, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		blk, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(blk), sshPub
}

func TestDial_PasswordTunnelsToConfiguredSocket(t *testing.T) {
	sock := startFakeDocker(t)
	srv := startFakeSSH(t, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cli, cleanup, err := dockerclient.Dial(ctx, hostFor(t, srv.addr, dockerclient.AuthPassword, sock), dockerclient.Material{Password: []byte(testPassword)})
	require.NoError(t, err)
	defer cleanup()
	v, err := cli.ServerVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, "27.0.0", v.Version)
	require.NotEmpty(t, srv.sockets())
	for _, s := range srv.sockets() {
		require.Equal(t, sock, s)
	}
	cleanup()
	cleanup() // idempotent
}

func TestDial_EncryptedPrivateKey(t *testing.T) {
	sock := startFakeDocker(t)
	key, pub := genKey(t, "pp")
	srv := startFakeSSH(t, pub)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cli, cleanup, err := dockerclient.Dial(ctx, hostFor(t, srv.addr, dockerclient.AuthSSHKey, sock), dockerclient.Material{PrivateKey: key, Passphrase: []byte("pp")})
	require.NoError(t, err)
	defer cleanup()
	_, err = cli.Ping(ctx)
	require.NoError(t, err)
}

func TestDial_WrongPasswordIsSafeAuthFailure(t *testing.T) {
	srv := startFakeSSH(t, nil)
	_, cleanup, err := dockerclient.Dial(t.Context(), hostFor(t, srv.addr, dockerclient.AuthPassword, ""), dockerclient.Material{Password: []byte("wrong-secret")})
	require.Error(t, err)
	require.Nil(t, cleanup)
	require.Equal(t, dockerclient.KindAuth, dockerclient.Classify(err))
	be, ok := apperr.AsBiz(dockerclient.AppError(err))
	require.True(t, ok)
	require.Equal(t, apperr.CodeDockerHostAuthFailed, be.Code)
	require.Equal(t, "SSH authentication failed", be.Message)
	require.NotContains(t, err.Error(), "wrong-secret")
}

func TestDial_MissingSocketIsClassified(t *testing.T) {
	srv := startFakeSSH(t, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cli, cleanup, err := dockerclient.Dial(ctx, hostFor(t, srv.addr, dockerclient.AuthPassword, "/nonexistent/docker.sock"), dockerclient.Material{Password: []byte(testPassword)})
	require.NoError(t, err)
	defer cleanup()
	_, err = cli.ServerVersion(ctx)
	require.Error(t, err)
	require.Equal(t, dockerclient.KindSocket, dockerclient.Classify(err))
	be, ok := apperr.AsBiz(dockerclient.AppError(err))
	require.True(t, ok)
	require.Equal(t, apperr.CodeDockerHostUnreachable, be.Code)
}

func TestDial_UnreachableAndHandshakeTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := ln.Addr().String()
	require.NoError(t, ln.Close())
	_, _, err = dockerclient.Dial(t.Context(), hostFor(t, closedAddr, dockerclient.AuthPassword, ""), dockerclient.Material{Password: []byte("x")})
	require.Equal(t, dockerclient.KindUnreachable, dockerclient.Classify(err))

	// A TCP peer that never speaks SSH must not hang past the context deadline.
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = silent.Close() })
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err = dockerclient.Dial(ctx, hostFor(t, silent.Addr().String(), dockerclient.AuthPassword, ""), dockerclient.Material{Password: []byte("x")})
	require.Equal(t, dockerclient.KindTimeout, dockerclient.Classify(err))
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestSSHConfig_RejectsUnusableMaterial(t *testing.T) {
	encrypted, _ := genKey(t, "pp")
	plain, _ := genKey(t, "")
	cases := []struct {
		name string
		host models.DockerHost
		mat  dockerclient.Material
		ok   bool
	}{
		{"plain key", models.DockerHost{AuthType: "ssh_key", Username: "ops"}, dockerclient.Material{PrivateKey: plain}, true},
		{"encrypted key with passphrase", models.DockerHost{AuthType: "ssh_key", Username: "ops"}, dockerclient.Material{PrivateKey: encrypted, Passphrase: []byte("pp")}, true},
		{"encrypted key without passphrase", models.DockerHost{AuthType: "ssh_key", Username: "ops"}, dockerclient.Material{PrivateKey: encrypted}, false},
		{"garbage key", models.DockerHost{AuthType: "ssh_key", Username: "ops"}, dockerclient.Material{PrivateKey: []byte("not-a-key")}, false},
		{"password", models.DockerHost{AuthType: "password", Username: "ops"}, dockerclient.Material{Password: []byte("pw")}, true},
		{"empty password", models.DockerHost{AuthType: "password", Username: "ops"}, dockerclient.Material{}, false},
		{"missing username", models.DockerHost{AuthType: "password"}, dockerclient.Material{Password: []byte("pw")}, false},
		{"unknown auth", models.DockerHost{AuthType: "agent", Username: "ops"}, dockerclient.Material{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := dockerclient.SSHConfig(&tc.host, tc.mat)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, "ops", cfg.User)
				require.NotEmpty(t, cfg.Auth)
				require.Equal(t, dockerclient.Timeout, cfg.Timeout)
				return
			}
			require.Error(t, err)
			require.Equal(t, dockerclient.KindConfig, dockerclient.Classify(err))
			require.Equal(t, "invalid SSH connection configuration", err.Error())
		})
	}
}

func TestMaterialWipe(t *testing.T) {
	pw := []byte("secret")
	m := dockerclient.Material{Password: pw}
	m.Wipe()
	require.Nil(t, m.Password)
	require.Equal(t, make([]byte, len(pw)), pw)
}
