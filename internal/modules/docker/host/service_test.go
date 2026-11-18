//go:build dbtest

package host_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	dockerapi "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/db"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/credentials/sshkey"
	"github.com/ArkGravity/optimus/internal/modules/credentials/vault"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
	"github.com/ArkGravity/optimus/internal/modules/docker/host"
)

const hostPassword = "host-pass-123"

type fakeDialer struct {
	mu   sync.Mutex
	err  error
	url  string
	seen []dockerssh.Material
}

func (f *fakeDialer) dial(_ context.Context, _ *models.DockerHost, m dockerssh.Material) (*dockerapi.Client, func(), error) {
	f.mu.Lock()
	// Copy before the service wipes the caller's buffers.
	f.seen = append(f.seen, dockerssh.Material{
		PrivateKey: append([]byte(nil), m.PrivateKey...),
		Passphrase: append([]byte(nil), m.Passphrase...),
		Password:   append([]byte(nil), m.Password...),
	})
	f.mu.Unlock()
	if f.err != nil {
		return nil, nil, f.err
	}
	cli, err := dockerapi.NewClientWithOpts(dockerapi.WithHost(f.url), dockerapi.WithAPIVersionNegotiation())
	if err != nil {
		return nil, nil, err
	}
	return cli, func() { _ = cli.Close() }, nil
}

type fixture struct {
	svc    *host.Service
	keys   *sshkey.Service
	cipher *vault.Cipher
	dialer *fakeDialer
	db     *gorm.DB
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gdb, cleanup := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(cleanup)
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	cipher, err := vault.NewCipher(key)
	require.NoError(t, err)
	rec := audit.NewRecorder(gdb)
	creds := credentials.New(gdb, cipher, rec)

	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.45")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Version": "27.3.1", "ApiVersion": "1.45"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(engine.Close)
	fd := &fakeDialer{url: "tcp://" + strings.TrimPrefix(engine.URL, "http://")}
	return &fixture{
		svc:    host.NewService(host.NewRepo(gdb), cipher, creds.Consumer, fd.dial, rec),
		keys:   creds.SSH,
		cipher: cipher,
		dialer: fd,
		db:     gdb,
	}
}

func genKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	return string(pem.EncodeToMemory(blk))
}

func (f *fixture) createKey(t *testing.T, name, username string) *sshkey.Detail {
	t.Helper()
	k, err := f.keys.Create(t.Context(), 0, "", "", sshkey.CreateRequest{Name: name, Username: username, PrivateKey: genKey(t)})
	require.NoError(t, err)
	return k
}

func (f *fixture) createPasswordHost(t *testing.T, name string) *host.Detail {
	t.Helper()
	d, err := f.svc.Create(t.Context(), 0, "", "", host.CreateRequest{
		Name: name, Host: "10.0.0.5", AuthType: "password", Username: "root", Password: hostPassword,
	})
	require.NoError(t, err)
	return d
}

func requireBiz(t *testing.T, err error, code apperr.Code, key string) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected BizError, got %v", err)
	require.Equal(t, code, be.Code)
	if key != "" {
		require.Equal(t, key, be.MessageKey)
	}
}

func uptr(v uint64) *uint64 { return &v }
func sptr(v string) *string { return &v }

func TestCreate_SSHKeyHostDefaultsAndJoinsKeyName(t *testing.T) {
	f := newFixture(t)
	key := f.createKey(t, "deploy-key", "deploy")

	d, err := f.svc.Create(t.Context(), 0, "127.0.0.1", "ua", host.CreateRequest{
		Name: " edge-1 ", Host: "docker.example.com", AuthType: "ssh_key", SSHKeyID: uptr(key.ID),
	})
	require.NoError(t, err)
	require.Equal(t, "edge-1", d.Name)
	require.Equal(t, 22, d.Port)
	require.Equal(t, "/var/run/docker.sock", d.SocketPath)
	require.Equal(t, "deploy", d.Username)
	require.Equal(t, "deploy-key", d.SSHKeyName)
	require.Equal(t, key.ID, *d.SSHKeyID)
	require.Nil(t, d.LastCheckOK)

	override, err := f.svc.Create(t.Context(), 0, "", "", host.CreateRequest{
		Name: "edge-2", Host: "fd00::1", Port: 2222, SocketPath: "/run/user/1000/docker.sock",
		AuthType: "ssh_key", SSHKeyID: uptr(key.ID), Username: "ops",
	})
	require.NoError(t, err)
	require.Equal(t, "ops", override.Username)
	require.Equal(t, 2222, override.Port)

	list, err := f.svc.List(t.Context(), host.ListQuery{Search: "edge"})
	require.NoError(t, err)
	require.EqualValues(t, 2, list.Total)
	require.Equal(t, "deploy-key", list.Items[0].SSHKeyName)
}

func TestCreate_PasswordIsSealedAndNeverReturned(t *testing.T) {
	f := newFixture(t)
	d := f.createPasswordHost(t, "pw-host")
	require.Equal(t, "password", d.AuthType)
	require.Nil(t, d.SSHKeyID)

	raw, err := json.Marshal(d)
	require.NoError(t, err)
	require.NotContains(t, string(raw), hostPassword)
	require.NotContains(t, string(raw), "password_enc")

	var row models.DockerHost
	require.NoError(t, f.db.First(&row, d.ID).Error)
	require.NotContains(t, string(row.PasswordEnc), hostPassword)
	plain, err := f.cipher.Open(row.PasswordEnc)
	require.NoError(t, err)
	require.Equal(t, hostPassword, string(plain))

	var events []models.AuditLog
	require.NoError(t, f.db.Where("action = ? AND target_id = ?", "docker.host.create", strconv.FormatUint(d.ID, 10)).Find(&events).Error)
	require.Len(t, events, 1)
	require.NotContains(t, string(events[0].Payload), hostPassword)
}

func TestCreate_RejectsInvalidRequests(t *testing.T) {
	f := newFixture(t)
	key := f.createKey(t, "k", "deploy")
	f.createPasswordHost(t, "taken")

	cases := []struct {
		name string
		req  host.CreateRequest
		code apperr.Code
		key  string
	}{
		{"ssh key missing", host.CreateRequest{Name: "a", Host: "h1", AuthType: "ssh_key"}, apperr.CodeValidation, "docker.host.ssh_key_required"},
		{"ssh key unknown", host.CreateRequest{Name: "a", Host: "h1", AuthType: "ssh_key", SSHKeyID: uptr(999999)}, apperr.CodeNotFound, "credentials.not_found"},
		{"password with ssh key", host.CreateRequest{Name: "a", Host: "h1", AuthType: "ssh_key", SSHKeyID: uptr(key.ID), Password: "x"}, apperr.CodeValidation, "docker.host.password_not_allowed"},
		{"password missing", host.CreateRequest{Name: "a", Host: "h1", AuthType: "password", Username: "root"}, apperr.CodeValidation, "docker.host.password_required"},
		{"username missing", host.CreateRequest{Name: "a", Host: "h1", AuthType: "password", Password: "x"}, apperr.CodeValidation, "docker.host.username_required"},
		{"key with password auth", host.CreateRequest{Name: "a", Host: "h1", AuthType: "password", Username: "r", Password: "x", SSHKeyID: uptr(key.ID)}, apperr.CodeValidation, "docker.host.ssh_key_not_allowed"},
		{"bad host", host.CreateRequest{Name: "a", Host: "ssh://h1", AuthType: "password", Username: "r", Password: "x"}, apperr.CodeValidation, "docker.host.host_invalid"},
		{"bad socket", host.CreateRequest{Name: "a", Host: "h1", SocketPath: "docker.sock", AuthType: "password", Username: "r", Password: "x"}, apperr.CodeValidation, "docker.host.socket_path_invalid"},
		{"duplicate name", host.CreateRequest{Name: "taken", Host: "h1", AuthType: "password", Username: "r", Password: "x"}, apperr.CodeConflict, "docker.host.name_taken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Create(t.Context(), 0, "", "", tc.req)
			requireBiz(t, err, tc.code, tc.key)
		})
	}
	var count int64
	require.NoError(t, f.db.Model(&models.DockerHost{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestUpdate_AuthSwitchesAndPasswordRetention(t *testing.T) {
	f := newFixture(t)
	key := f.createKey(t, "k", "deploy")
	d := f.createPasswordHost(t, "switch")
	var before models.DockerHost
	require.NoError(t, f.db.First(&before, d.ID).Error)

	// Metadata-only edit keeps the sealed password.
	upd, err := f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{Description: sptr("rack 4")})
	require.NoError(t, err)
	require.Equal(t, "rack 4", upd.Description)
	var row models.DockerHost
	require.NoError(t, f.db.First(&row, d.ID).Error)
	require.Equal(t, before.PasswordEnc, row.PasswordEnc)

	// Switching to SSH key auth drops the password and adopts the key.
	upd, err = f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{AuthType: sptr("ssh_key"), SSHKeyID: uptr(key.ID), Username: sptr("")})
	require.NoError(t, err)
	require.Equal(t, "ssh_key", upd.AuthType)
	require.Equal(t, "deploy", upd.Username)
	require.Equal(t, "k", upd.SSHKeyName)
	row = models.DockerHost{}
	require.NoError(t, f.db.First(&row, d.ID).Error)
	require.Nil(t, row.PasswordEnc)

	// Switching back requires a new password.
	_, err = f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{AuthType: sptr("password")})
	requireBiz(t, err, apperr.CodeValidation, "docker.host.password_required")
	upd, err = f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{AuthType: sptr("password"), Password: sptr("new-pass")})
	require.NoError(t, err)
	require.Nil(t, upd.SSHKeyID)
	require.Empty(t, upd.SSHKeyName)
	row = models.DockerHost{}
	require.NoError(t, f.db.First(&row, d.ID).Error)
	plain, err := f.cipher.Open(row.PasswordEnc)
	require.NoError(t, err)
	require.Equal(t, "new-pass", string(plain))

	var events []models.AuditLog
	require.NoError(t, f.db.Where("action = ?", "docker.host.update").Order("id").Find(&events).Error)
	require.Len(t, events, 3)
	require.Contains(t, string(events[2].Payload), `"secret_rotated": true`)
	require.Contains(t, string(events[2].Payload), `"password"`)
	require.NotContains(t, string(events[2].Payload), "new-pass")
}

func TestUpdate_ConnectionChangeResetsLastCheck(t *testing.T) {
	f := newFixture(t)
	d := f.createPasswordHost(t, "probe")
	_, err := f.svc.Test(t.Context(), 0, "", "", d.ID)
	require.NoError(t, err)
	got, err := f.svc.Get(t.Context(), d.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastCheckOK)

	_, err = f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{Description: sptr("note")})
	require.NoError(t, err)
	got, err = f.svc.Get(t.Context(), d.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastCheckOK, "metadata edits keep the probe result")

	_, err = f.svc.Update(t.Context(), 0, "", "", d.ID, host.UpdateRequest{Host: sptr("10.0.0.6")})
	require.NoError(t, err)
	got, err = f.svc.Get(t.Context(), d.ID)
	require.NoError(t, err)
	require.Nil(t, got.LastCheckOK)
	require.Nil(t, got.LastCheckedAt)
}

func TestDelete_SoftDeletesDropsPasswordAndFreesName(t *testing.T) {
	f := newFixture(t)
	d := f.createPasswordHost(t, "gone")
	require.NoError(t, f.svc.Delete(t.Context(), 0, "127.0.0.1", "ua", d.ID))

	_, err := f.svc.Get(t.Context(), d.ID)
	requireBiz(t, err, apperr.CodeNotFound, "docker.host.not_found")
	requireBiz(t, f.svc.Delete(t.Context(), 0, "", "", d.ID), apperr.CodeNotFound, "docker.host.not_found")

	var row models.DockerHost
	require.NoError(t, f.db.Unscoped().First(&row, d.ID).Error)
	require.True(t, row.DeletedAt.Valid)
	require.Nil(t, row.PasswordEnc)

	var events int64
	require.NoError(t, f.db.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", "docker.host.delete", strconv.FormatUint(d.ID, 10)).Count(&events).Error)
	require.EqualValues(t, 1, events)

	f.createPasswordHost(t, "gone")
}

func TestProbe_UsesSubmittedMaterialWithoutPersisting(t *testing.T) {
	f := newFixture(t)
	key := f.createKey(t, "probe-key", "deploy")
	pw := f.createPasswordHost(t, "probe-pw")

	ok, err := f.svc.Probe(t.Context(), 0, host.ProbeRequest{
		Host: "10.0.0.9", AuthType: "password", Username: "root", Password: hostPassword,
	})
	require.NoError(t, err)
	require.True(t, ok.OK)
	require.Equal(t, "27.3.1", ok.ServerVersion)

	kept, err := f.svc.Probe(t.Context(), 0, host.ProbeRequest{
		HostID: &pw.ID, Host: pw.Host, AuthType: "password", Username: pw.Username,
	})
	require.NoError(t, err)
	require.True(t, kept.OK)

	keyRes, err := f.svc.Probe(t.Context(), 0, host.ProbeRequest{
		Host: "10.0.0.9", AuthType: "ssh_key", SSHKeyID: uptr(key.ID),
	})
	require.NoError(t, err)
	require.True(t, keyRes.OK)
	require.Len(t, f.dialer.seen, 3)
	require.Equal(t, hostPassword, string(f.dialer.seen[0].Password))
	require.Equal(t, hostPassword, string(f.dialer.seen[1].Password))
	require.Contains(t, string(f.dialer.seen[2].PrivateKey), "OPENSSH PRIVATE KEY")

	got, err := f.svc.Get(t.Context(), pw.ID)
	require.NoError(t, err)
	require.Nil(t, got.LastCheckOK)

	f.dialer.err = &dockerssh.Error{Kind: dockerssh.KindAuth}
	fail, err := f.svc.Probe(t.Context(), 0, host.ProbeRequest{
		Host: "10.0.0.9", AuthType: "password", Username: "root", Password: "bad",
	})
	require.NoError(t, err)
	require.False(t, fail.OK)
	require.Equal(t, "SSH authentication failed", fail.Message)

	_, err = f.svc.Probe(t.Context(), 0, host.ProbeRequest{
		Host: "10.0.0.9", AuthType: "password", Username: "root",
	})
	requireBiz(t, err, apperr.CodeValidation, "docker.host.password_required")
}

func TestTest_PersistsSuccessAndSafeFailure(t *testing.T) {
	f := newFixture(t)
	key := f.createKey(t, "k", "deploy")
	pw := f.createPasswordHost(t, "pw")
	kh, err := f.svc.Create(t.Context(), 0, "", "", host.CreateRequest{Name: "kh", Host: "h2", AuthType: "ssh_key", SSHKeyID: uptr(key.ID)})
	require.NoError(t, err)

	res, err := f.svc.Test(t.Context(), 0, "", "", pw.ID)
	require.NoError(t, err)
	require.True(t, res.OK)
	require.Equal(t, "27.3.1", res.ServerVersion)
	require.Equal(t, "1.45", res.APIVersion)
	got, err := f.svc.Get(t.Context(), pw.ID)
	require.NoError(t, err)
	require.True(t, *got.LastCheckOK)
	require.NotNil(t, got.LastCheckedAt)
	require.Empty(t, got.LastCheckError)

	_, err = f.svc.Test(t.Context(), 0, "", "", kh.ID)
	require.NoError(t, err)
	require.Len(t, f.dialer.seen, 2)
	require.Equal(t, hostPassword, string(f.dialer.seen[0].Password))
	require.Empty(t, f.dialer.seen[0].PrivateKey)
	require.Contains(t, string(f.dialer.seen[1].PrivateKey), "OPENSSH PRIVATE KEY")
	require.Empty(t, f.dialer.seen[1].Password)
	var consumes []models.AuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?", "credentials.consume", "credentials.ssh_key").Find(&consumes).Error)
	require.Len(t, consumes, 1)
	require.Contains(t, string(consumes[0].Payload), "system:docker.host.test")

	f.dialer.err = &dockerssh.Error{Kind: dockerssh.KindAuth}
	res, err = f.svc.Test(t.Context(), 0, "", "", pw.ID)
	require.NoError(t, err)
	require.False(t, res.OK)
	require.Equal(t, "SSH authentication failed", res.Message)
	got, err = f.svc.Get(t.Context(), pw.ID)
	require.NoError(t, err)
	require.False(t, *got.LastCheckOK)
	require.Equal(t, "SSH authentication failed", got.LastCheckError)

	_, err = f.svc.Test(t.Context(), 0, "", "", 999999)
	requireBiz(t, err, apperr.CodeNotFound, "docker.host.not_found")

	var tests int64
	require.NoError(t, f.db.Model(&models.AuditLog{}).Where("action = ?", "docker.host.test").Count(&tests).Error)
	require.EqualValues(t, 3, tests)
}

func TestConnect_ReturnsSafeDialErrors(t *testing.T) {
	f := newFixture(t)
	d := f.createPasswordHost(t, "c")
	f.dialer.err = &dockerssh.Error{Kind: dockerssh.KindSocket}
	m, cli, cleanup, err := f.svc.Connect(t.Context(), 0, d.ID, "docker.host.test")
	require.NotNil(t, m)
	require.Nil(t, cli)
	require.Nil(t, cleanup)
	requireBiz(t, err, apperr.CodeDockerHostUnreachable, "docker.host.unreachable")
}
