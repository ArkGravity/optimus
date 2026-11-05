package cloudkey

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/advisorylock"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account/inuse"
	"github.com/ArkGravity/optimus/internal/modules/audit"
)

// Cipher is the subset of vault.Cipher the service depends on.
type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

// AccountsInUseCounter reports live CloudAccounts that reference a cloud key.
type AccountsInUseCounter interface {
	CountByCloudKeyID(ctx context.Context, id uint64) (int64, error)
}

type transactionalAccountsInUseCounter interface {
	CountByCloudKeyIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error)
}

type Service struct {
	repo                 *Repo
	cipher               Cipher
	audit                *audit.Recorder
	accountsInUseCounter AccountsInUseCounter
}

func NewService(repo *Repo, cipher Cipher, rec *audit.Recorder) *Service {
	return &Service{repo: repo, cipher: cipher, audit: rec}
}

// SetAccountsInUseCounter is a composition-time setter. Call it before route
// serving begins; it is not intended to race with Delete requests.
func (s *Service) SetAccountsInUseCounter(counter AccountsInUseCounter) {
	s.accountsInUseCounter = counter
}

func (s *Service) Repo() *Repo { return s.repo }

// --- queries ---------------------------------------------------------------

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, normalizeError(err)
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: out, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, normalizeError(err)
	}
	d := Detail(toSummary(*m))
	return &d, nil
}

// Exists checks whether a cloud key can be referenced without decrypting it
// or emitting a credential-consumption audit event.
func (s *Service) Exists(ctx context.Context, id uint64) (bool, error) {
	exists, err := s.repo.Exists(ctx, id)
	return exists, normalizeError(err)
}

// --- mutations -------------------------------------------------------------

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	authType, err := ResolveAuthType(req.Provider, req.AuthType)
	if err != nil {
		return nil, err
	}
	fields, _, err := s.secretFields(authType, secretInput{
		accessKeyID: optionalSecret(req.AccessKeyID), secretAccessKey: optionalSecret(req.SecretAccessKey),
		serviceAccountJSON: optionalSecret(req.ServiceAccountJSON),
	}, true)
	if err != nil {
		return nil, err
	}
	m := &models.CredentialCloudKey{
		Name: strings.TrimSpace(req.Name), Description: req.Description,
		Provider: req.Provider, Region: req.Region, AuthType: authType,
	}
	if m.Name == "" {
		return nil, validationError()
	}
	m.AccessKeyIDEnc, _ = fields["access_key_id_enc"].([]byte)
	m.SecretAccessKeyEnc, _ = fields["secret_access_key_enc"].([]byte)
	m.ServiceAccountJSONEnc, _ = fields["service_account_json_enc"].([]byte)
	m.ServiceAccountEmail, _ = fields["service_account_email"].(string)
	m.CredentialProjectID, _ = fields["credential_project_id"].(string)
	m.CreatedByUserID = ptrIfNonZero(actorID)
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateTx(ctx, tx, m); err != nil {
			return err
		}
		return s.writeAuditTx(ctx, tx, ptrIfNonZero(actorID), "credentials.create", m.ID, ip, ua, map[string]any{"name": m.Name})
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	detail := Detail(toSummary(*m))
	return &detail, nil
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	var detail Detail
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := advisorylock.LockCloudKey(ctx, tx, id); err != nil {
			return err
		}
		m, err := s.repo.GetForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		provider := m.Provider
		if req.Provider != nil {
			provider = *req.Provider
		}
		requestedAuth := ""
		if req.AuthType != nil {
			requestedAuth = *req.AuthType
		}
		authType, err := ResolveAuthType(provider, requestedAuth)
		if err != nil {
			return err
		}
		if m.AuthType == AuthTypeLegacyInvalid && provider == m.Provider && req.AuthType == nil && req.ServiceAccountJSON == nil {
			authType = AuthTypeLegacyInvalid
		}
		conversion := provider != m.Provider || authType != m.AuthType
		if conversion {
			if err := s.checkConversionReferences(ctx, tx, id); err != nil {
				return err
			}
		}
		fields, changed, err := s.secretFields(authType, secretInput{
			accessKeyID: req.AccessKeyID, secretAccessKey: req.SecretAccessKey, serviceAccountJSON: req.ServiceAccountJSON,
		}, conversion)
		if err != nil {
			return err
		}
		rotated := len(changed) > 0
		if conversion {
			fields["provider"], fields["auth_type"] = provider, authType
			if provider != m.Provider {
				changed = append(changed, "provider")
			}
			if authType != m.AuthType {
				changed = append(changed, "auth_type")
			}
		}
		finalName := m.Name
		if req.Name != nil {
			finalName = strings.TrimSpace(*req.Name)
			if finalName == "" {
				return validationError()
			}
			if finalName != m.Name {
				fields["name"] = finalName
				changed = append(changed, "name")
			}
		}
		if req.Description != nil && *req.Description != m.Description {
			fields["description"] = *req.Description
			changed = append(changed, "description")
		}
		if req.Region != nil && *req.Region != m.Region {
			fields["region"] = *req.Region
			changed = append(changed, "region")
		}
		if len(changed) > 0 {
			if err := s.repo.UpdateTx(ctx, tx, id, fields); err != nil {
				return err
			}
			action := "credentials.update"
			if rotated {
				action = "credentials.rotate"
			}
			if err := s.writeAuditTx(ctx, tx, ptrIfNonZero(actorID), action, id, ip, ua, map[string]any{
				"name": finalName, "changed_fields": changed, "secret_rotated": rotated,
			}); err != nil {
				return err
			}
		}
		m, err = s.repo.GetTx(ctx, tx, id)
		if err != nil {
			return err
		}
		detail = Detail(toSummary(*m))
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	return &detail, nil
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := advisorylock.LockCloudKey(ctx, tx, id); err != nil {
			return err
		}
		m, err := s.repo.GetForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if s.accountsInUseCounter != nil {
			var n int64
			if counter, ok := s.accountsInUseCounter.(transactionalAccountsInUseCounter); ok {
				n, err = counter.CountByCloudKeyIDTx(ctx, tx, id)
			} else {
				n, err = s.accountsInUseCounter.CountByCloudKeyID(ctx, id)
			}
			if err != nil {
				return err
			}
			if n > 0 {
				return apperr.New(
					apperr.CodeAssetsCloudAccountInUse,
					"assets.cloud_account.in_use",
					fmt.Sprintf("cloud key referenced by %d cloud account(s)", n),
				)
			}
			if err := inuse.DetachDeletedByCloudKeyID(ctx, tx, id); err != nil {
				return err
			}
		}
		rows, err := s.repo.DeleteTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return gorm.ErrRecordNotFound
		}
		if s.audit != nil {
			return s.audit.WithTx(tx).Record(ctx, audit.Event{
				UserID: ptrIfNonZero(actorID), Action: "credentials.delete",
				TargetType: "credentials.cloud_key", TargetID: strconv.FormatUint(id, 10),
				Payload: map[string]any{"name": m.Name}, IP: ip, UserAgent: ua,
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		if _, ok := apperr.AsBiz(err); ok {
			return err
		}
		return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	return nil
}

// --- consume ---------------------------------------------------------------

type ConsumeRecord struct {
	Name               string
	Provider           string
	Region             string
	AccessKeyID        string
	SecretAccessKey    string
	AuthType           string
	ServiceAccountJSON []byte
}

func (s *Service) Consume(ctx context.Context, actor *uint64, id uint64, purpose string) (*ConsumeRecord, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_purpose", "purpose required")
	}
	if actor == nil && !strings.HasPrefix(purpose, "system:") {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.system_purpose_required", "system caller purpose must start with system:")
	}

	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, normalizeError(err)
	}
	authType, err := ResolveAuthType(m.Provider, m.AuthType)
	if err != nil || authType != m.AuthType {
		return nil, validationError()
	}
	record := &ConsumeRecord{Name: m.Name, Provider: m.Provider, Region: m.Region, AuthType: m.AuthType}
	if authType == AuthTypeServiceAccountJSON {
		record.ServiceAccountJSON, err = s.cipher.Open(m.ServiceAccountJSONEnc)
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
		}
	} else {
		ak, err := s.cipher.Open(m.AccessKeyIDEnc)
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
		}
		defer clear(ak)
		sk, err := s.cipher.Open(m.SecretAccessKeyEnc)
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
		}
		defer clear(sk)
		record.AccessKeyID, record.SecretAccessKey = string(ak), string(sk)
	}

	s.writeAudit(ctx, actor, "credentials.consume", id, "", "", map[string]any{
		"name":    m.Name,
		"purpose": purpose,
	})

	return record, nil
}

// --- helpers ---------------------------------------------------------------

func (s *Service) seal(data []byte) ([]byte, error) {
	sealed, err := s.cipher.Seal(data)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	return sealed, nil
}

func (s *Service) checkConversionReferences(ctx context.Context, tx *gorm.DB, id uint64) error {
	var count int64
	var err error
	if counter, ok := s.accountsInUseCounter.(transactionalAccountsInUseCounter); ok {
		count, err = counter.CountByCloudKeyIDTx(ctx, tx, id)
	} else {
		if s.accountsInUseCounter != nil {
			count, err = s.accountsInUseCounter.CountByCloudKeyID(ctx, id)
			if err != nil {
				return err
			}
		}
		if count == 0 {
			// Conversion must remain protected before the assets counter is wired.
			count, err = inuse.New(tx).CountByCloudKeyID(ctx, id)
		}
	}
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.New(apperr.CodeAssetsCloudAccountInUse, "assets.cloud_account.in_use", fmt.Sprintf("cloud key referenced by %d cloud account(s)", count))
	}
	return nil
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := apperr.AsBiz(err); ok {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credentials_cloud_keys_name_key" {
		return apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	}
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}

func (s *Service) writeAuditTx(ctx context.Context, tx *gorm.DB, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.WithTx(tx).Record(ctx, audit.Event{
		UserID: actor, Action: action, TargetType: "credentials.cloud_key", TargetID: strconv.FormatUint(id, 10),
		Payload: payload, IP: ip, UserAgent: ua,
	})
}

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "credentials.cloud_key",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func ptrIfNonZero(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toSummary(m models.CredentialCloudKey) Summary {
	out := Summary{
		ID:                  m.ID,
		Name:                m.Name,
		Description:         m.Description,
		Provider:            m.Provider,
		Region:              m.Region,
		AuthType:            m.AuthType,
		ServiceAccountEmail: m.ServiceAccountEmail,
		CredentialProjectID: m.CredentialProjectID,
		NeedsConfiguration:  m.AuthType == AuthTypeLegacyInvalid,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
	if m.CreatedByUserID != nil {
		out.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return out
}
