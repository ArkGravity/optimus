package host

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	dockerapi "github.com/docker/docker/client"
	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

const defaultPort = 22

var (
	hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	socketPattern   = regexp.MustCompile(`^/[A-Za-z0-9._/@-]+$`)
)

// Cipher is the subset of vault.Cipher the service depends on.
type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

// DialFunc opens a tunnelled Docker client; production uses dockerssh.Dial.
type DialFunc func(ctx context.Context, h *models.DockerHost, m dockerssh.Material) (*dockerapi.Client, func(), error)

type Service struct {
	repo     *Repo
	cipher   Cipher
	consumer credentials.Consumer
	dial     DialFunc
	audit    *audit.Recorder
}

func NewService(repo *Repo, cipher Cipher, consumer credentials.Consumer, dial DialFunc, rec *audit.Recorder) *Service {
	if dial == nil {
		dial = dockerssh.Dial
	}
	return &Service{repo: repo, cipher: cipher, consumer: consumer, dial: dial, audit: rec}
}

func (s *Service) Repo() *Repo { return s.repo }

// --- queries ---------------------------------------------------------------

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 20
	}
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for i := range rows {
		out = append(out, toSummary(&rows[i]))
	}
	return &ListResponse{Items: out, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	d := toSummary(m)
	return &d, nil
}

// --- mutations -------------------------------------------------------------

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	m := &models.DockerHost{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Host:        strings.TrimSpace(req.Host),
		Port:        req.Port,
		SocketPath:  strings.TrimSpace(req.SocketPath),
		AuthType:    req.AuthType,
		Username:    strings.TrimSpace(req.Username),
	}
	if m.Port == 0 {
		m.Port = defaultPort
	}
	if m.SocketPath == "" {
		m.SocketPath = dockerssh.DefaultSocketPath
	}
	if err := validateConnection(m.Name, m.Host, m.SocketPath); err != nil {
		return nil, err
	}
	switch req.AuthType {
	case dockerssh.AuthSSHKey:
		if req.Password != "" {
			return nil, validation("docker.host.password_not_allowed", "password is only accepted for password authentication")
		}
		key, err := s.sshKeyMeta(ctx, req.SSHKeyID)
		if err != nil {
			return nil, err
		}
		m.SSHKeyID = &key.ID
		if m.Username == "" {
			m.Username = key.Username
		}
	case dockerssh.AuthPassword:
		if req.SSHKeyID != nil {
			return nil, validation("docker.host.ssh_key_not_allowed", "ssh_key_id is only accepted for SSH key authentication")
		}
		if req.Password == "" {
			return nil, validation("docker.host.password_required", "password is required for password authentication")
		}
		enc, err := s.seal(req.Password)
		if err != nil {
			return nil, err
		}
		m.PasswordEnc = enc
	default:
		return nil, validation("docker.host.auth_type_invalid", "auth_type must be ssh_key or password")
	}
	if m.Username == "" {
		return nil, validation("docker.host.username_required", "username is required")
	}
	if err := s.ensureNameFree(ctx, m.Name); err != nil {
		return nil, err
	}
	m.CreatedByUserID = ptrIfNonZero(actorID)
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, actorID, "docker.host.create", m.ID, ip, ua, map[string]any{
		"name":       m.Name,
		"host":       m.Host,
		"port":       m.Port,
		"auth_type":  m.AuthType,
		"ssh_key_id": m.SSHKeyID,
	})
	return s.Get(ctx, m.ID)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	final := *m
	fields := map[string]any{}
	var changed []string
	set := func(column string, value any) {
		fields[column] = value
		changed = append(changed, column)
	}

	if req.Name != nil {
		if n := strings.TrimSpace(*req.Name); n != m.Name {
			final.Name = n
			set("name", n)
		}
	}
	if req.Description != nil && *req.Description != m.Description {
		set("description", *req.Description)
	}
	if req.Host != nil {
		if h := strings.TrimSpace(*req.Host); h != m.Host {
			final.Host = h
			set("host", h)
		}
	}
	if req.Port != nil && *req.Port != m.Port {
		final.Port = *req.Port
		set("port", *req.Port)
	}
	if req.SocketPath != nil {
		sp := strings.TrimSpace(*req.SocketPath)
		if sp == "" {
			sp = dockerssh.DefaultSocketPath
		}
		if sp != m.SocketPath {
			final.SocketPath = sp
			set("socket_path", sp)
		}
	}
	if err := validateConnection(final.Name, final.Host, final.SocketPath); err != nil {
		return nil, err
	}

	if req.AuthType != nil {
		final.AuthType = *req.AuthType
	}
	newPassword := ""
	if req.Password != nil {
		newPassword = *req.Password
	}
	username := m.Username
	if req.Username != nil {
		username = strings.TrimSpace(*req.Username)
	}
	rotated := false
	switch final.AuthType {
	case dockerssh.AuthSSHKey:
		if newPassword != "" {
			return nil, validation("docker.host.password_not_allowed", "password is only accepted for password authentication")
		}
		keyID := m.SSHKeyID
		if req.SSHKeyID != nil {
			keyID = req.SSHKeyID
		}
		key, err := s.sshKeyMeta(ctx, keyID)
		if err != nil {
			return nil, err
		}
		if m.SSHKeyID == nil || *m.SSHKeyID != key.ID {
			set("ssh_key_id", key.ID)
		}
		if len(m.PasswordEnc) > 0 {
			set("password_enc", nil)
		}
		if username == "" {
			username = key.Username
		}
	case dockerssh.AuthPassword:
		if req.SSHKeyID != nil {
			return nil, validation("docker.host.ssh_key_not_allowed", "ssh_key_id is only accepted for SSH key authentication")
		}
		if newPassword == "" && len(m.PasswordEnc) == 0 {
			return nil, validation("docker.host.password_required", "password is required for password authentication")
		}
		if newPassword != "" {
			enc, err := s.seal(newPassword)
			if err != nil {
				return nil, err
			}
			set("password_enc", enc)
			rotated = true
		}
		if m.SSHKeyID != nil {
			set("ssh_key_id", nil)
		}
	default:
		return nil, validation("docker.host.auth_type_invalid", "auth_type must be ssh_key or password")
	}
	if final.AuthType != m.AuthType {
		set("auth_type", final.AuthType)
	}
	if username == "" {
		return nil, validation("docker.host.username_required", "username is required")
	}
	if username != m.Username {
		set("username", username)
	}

	if final.Name != m.Name {
		if err := s.ensureNameFree(ctx, final.Name); err != nil {
			return nil, err
		}
	}
	if len(fields) == 0 {
		return s.Get(ctx, id)
	}
	if connectionChanged(fields) {
		// A stale probe result must not describe the new endpoint.
		fields["last_checked_at"] = nil
		fields["last_check_ok"] = nil
		fields["last_check_error"] = ""
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	for i, c := range changed {
		if c == "password_enc" {
			changed[i] = "password"
		}
	}
	s.writeAudit(ctx, actorID, "docker.host.update", id, ip, ua, map[string]any{
		"name":           final.Name,
		"changed_fields": changed,
		"secret_rotated": rotated,
	})
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		m, err := s.repo.GetForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.repo.SoftDeleteTx(ctx, tx, id); err != nil {
			return err
		}
		if s.audit != nil {
			return s.audit.WithTx(tx).Record(ctx, audit.Event{
				UserID: ptrIfNonZero(actorID), Action: "docker.host.delete",
				TargetType: "docker.host", TargetID: strconv.FormatUint(id, 10),
				Payload: map[string]any{"name": m.Name}, IP: ip, UserAgent: ua,
			})
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound()
	}
	return err
}

// Test dials the host, queries the Engine version, and persists the outcome.
// Connection failures are reported in the result, never as raw errors.
func (s *Service) Test(ctx context.Context, actorID uint64, ip, ua string, id uint64) (*TestResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, dockerssh.Timeout)
	defer cancel()
	m, cli, cleanup, err := s.Connect(probeCtx, actorID, id, "docker.host.test")
	if err != nil && m == nil {
		return nil, err
	}
	res := &TestResult{}
	if err == nil {
		v, verr := cli.ServerVersion(probeCtx)
		cleanup()
		if verr != nil {
			err = dockerssh.AppError(verr)
		} else {
			res.OK = true
			res.ServerVersion = v.Version
			res.APIVersion = v.APIVersion
		}
	}
	if err != nil {
		be, ok := apperr.AsBiz(err)
		if !ok {
			return nil, err
		}
		res.Message = be.Message
	}
	res.CheckedAt = time.Now()
	// Persist even when the probe consumed the request deadline.
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelPersist()
	if err := s.repo.UpdateCheck(persistCtx, id, res.OK, res.Message, res.CheckedAt); err != nil {
		return nil, err
	}
	s.writeAudit(persistCtx, actorID, "docker.host.test", id, ip, ua, map[string]any{
		"name":           m.Name,
		"ok":             res.OK,
		"server_version": res.ServerVersion,
		"error":          res.Message,
	})
	return res, nil
}

// Connect loads the host, resolves its secret material, and opens a
// tunnelled Docker client. The host is returned whenever it was loaded, so
// callers can distinguish lookup failures from connection failures. Every
// returned error is safe for clients; cleanup is non-nil only on success.
func (s *Service) Connect(ctx context.Context, actorID, id uint64, purpose string) (*models.DockerHost, *dockerapi.Client, func(), error) {
	m, err := s.load(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	mat, err := s.material(ctx, actorID, m, purpose)
	if err != nil {
		return m, nil, nil, err
	}
	cli, cleanup, err := s.dial(ctx, m, mat)
	mat.Wipe()
	if err != nil {
		return m, nil, nil, dockerssh.AppError(err)
	}
	return m, cli, cleanup, nil
}

// --- helpers ---------------------------------------------------------------

func (s *Service) load(ctx context.Context, id uint64) (*models.DockerHost, error) {
	m, err := s.repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	return m, err
}

func (s *Service) material(ctx context.Context, actorID uint64, m *models.DockerHost, purpose string) (dockerssh.Material, error) {
	switch m.AuthType {
	case dockerssh.AuthSSHKey:
		if m.SSHKeyID == nil {
			return dockerssh.Material{}, configInvalid()
		}
		if actorID != 0 {
			ctx = credentials.WithActor(ctx, actorID)
		} else {
			purpose = "system:" + purpose
		}
		key, err := s.consumer.GetSSHKey(ctx, *m.SSHKeyID, purpose)
		if err != nil {
			return dockerssh.Material{}, err
		}
		return dockerssh.Material{PrivateKey: key.PrivateKey, Passphrase: key.Passphrase}, nil
	case dockerssh.AuthPassword:
		pw, err := s.cipher.Open(m.PasswordEnc)
		if err != nil || len(pw) == 0 {
			return dockerssh.Material{}, configInvalid()
		}
		return dockerssh.Material{Password: pw}, nil
	default:
		return dockerssh.Material{}, configInvalid()
	}
}

func (s *Service) sshKeyMeta(ctx context.Context, id *uint64) (*models.CredentialSSHKey, error) {
	if id == nil || *id == 0 {
		return nil, validation("docker.host.ssh_key_required", "ssh_key_id is required for SSH key authentication")
	}
	key, err := s.repo.SSHKeyMeta(ctx, *id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
	}
	return key, err
}

func (s *Service) ensureNameFree(ctx context.Context, name string) error {
	_, err := s.repo.FindByName(ctx, name)
	switch {
	case err == nil:
		return apperr.New(apperr.CodeConflict, "docker.host.name_taken", "Docker host name already exists")
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	default:
		return err
	}
}

func (s *Service) seal(password string) ([]byte, error) {
	enc, err := s.cipher.Seal([]byte(password))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	return enc, nil
}

func (s *Service) writeAudit(ctx context.Context, actorID uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     ptrIfNonZero(actorID),
		Action:     action,
		TargetType: "docker.host",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func validateConnection(name, host, socket string) error {
	if name == "" {
		return validation("docker.host.name_required", "name is required")
	}
	if net.ParseIP(host) == nil && (len(host) > 253 || !hostnamePattern.MatchString(host)) {
		return validation("docker.host.host_invalid", "host must be a hostname or IP address")
	}
	if !socketPattern.MatchString(socket) {
		return validation("docker.host.socket_path_invalid", "socket_path must be an absolute path")
	}
	return nil
}

func connectionChanged(fields map[string]any) bool {
	for _, c := range []string{"host", "port", "socket_path", "auth_type", "ssh_key_id", "username", "password_enc"} {
		if _, ok := fields[c]; ok {
			return true
		}
	}
	return false
}

func validation(key, msg string) error { return apperr.New(apperr.CodeValidation, key, msg) }

func notFound() error {
	return apperr.New(apperr.CodeNotFound, "docker.host.not_found", "Docker host not found")
}

func configInvalid() error {
	return apperr.New(apperr.CodeDockerHostConfigInvalid, "docker.host.config_invalid", dockerssh.SafeMessage(dockerssh.KindConfig))
}

func ptrIfNonZero(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toSummary(m *models.DockerHost) Summary {
	out := Summary{
		ID:             m.ID,
		Name:           m.Name,
		Description:    m.Description,
		Host:           m.Host,
		Port:           m.Port,
		SocketPath:     m.SocketPath,
		AuthType:       m.AuthType,
		SSHKeyID:       m.SSHKeyID,
		Username:       m.Username,
		LastCheckedAt:  m.LastCheckedAt,
		LastCheckOK:    m.LastCheckOK,
		LastCheckError: m.LastCheckError,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	if m.SSHKey != nil {
		out.SSHKeyName = m.SSHKey.Name
	}
	return out
}
