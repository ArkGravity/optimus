package kubeconfig

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/advisorylock"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/gke"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
)

func invalidGKE() error {
	return apperr.New(apperr.CodeValidation, "credentials.kubeconfig.invalid_gke", "invalid GKE connection configuration")
}

func gkeConfig(m models.CredentialKubeconfig) *gke.Config {
	if m.AuthType != AuthGKE {
		return nil
	}
	return &gke.Config{Project: m.GKEProject, Location: m.GKELocation, Cluster: m.GKECluster, Endpoint: m.GKEEndpoint}
}

// Binding and Cloud Key deletion/conversion share the Cloud Key advisory lock.
// Metadata validation never decrypts the key or calls a provider.
func (s *Service) validateGKEKey(ctx context.Context, tx *gorm.DB, id *uint64) error {
	if id == nil || *id == 0 {
		return invalidGKE()
	}
	if err := advisorylock.LockCloudKey(ctx, tx, *id); err != nil {
		return err
	}
	key, err := s.repo.CloudKeyMetadataTx(ctx, tx, *id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidGKE()
		}
		return err
	}
	if key.Provider != "gcp" || key.AuthType != "service_account_json" {
		return invalidGKE()
	}
	return nil
}

func (s *Service) createGKE(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	if req.GKE == nil || !req.GKE.Valid() || req.Kubeconfig != "" || strings.TrimSpace(req.Name) == "" {
		return nil, invalidGKE()
	}
	enc, err := s.cipher.Seal(nil)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	m := &models.CredentialKubeconfig{
		Name: strings.TrimSpace(req.Name), Description: req.Description, DefaultNamespace: req.DefaultNamespace,
		AuthType: AuthGKE, CloudKeyID: req.CloudKeyID, GKEProject: req.GKE.Project,
		GKELocation: req.GKE.Location, GKECluster: req.GKE.Cluster, GKEEndpoint: req.GKE.Endpoint,
		KubeconfigEnc: enc, CreatedByUserID: ptrIfNonZero(actorID),
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.validateGKEKey(ctx, tx, req.CloudKeyID); err != nil {
			return err
		}
		if _, err := s.repo.FindByNameTx(ctx, tx, m.Name); err == nil {
			return apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.repo.CreateTx(ctx, tx, m); err != nil {
			return err
		}
		return s.auditGKE(ctx, tx, actorID, "credentials.create", m.ID, ip, ua, map[string]any{"name": m.Name, "auth_type": AuthGKE})
	})
	if err != nil {
		return nil, safeGKEStoreError(err)
	}
	return s.Get(ctx, m.ID)
}

func (s *Service) updateGKE(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	if req.Kubeconfig != nil || (req.AuthType != nil && *req.AuthType != AuthGKE) {
		return nil, invalidGKE()
	}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if req.CloudKeyID != nil {
			if err := s.validateGKEKey(ctx, tx, req.CloudKeyID); err != nil {
				return err
			}
		}
		m, err := s.repo.GetForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if m.AuthType != AuthGKE || (req.GKE != nil && *req.GKE != *gkeConfig(*m)) {
			return invalidGKE()
		}
		fields := map[string]any{}
		var changed []string
		for _, field := range []struct {
			name    string
			value   *string
			current string
		}{
			{"name", req.Name, m.Name}, {"description", req.Description, m.Description}, {"default_namespace", req.DefaultNamespace, m.DefaultNamespace},
		} {
			if field.value == nil {
				continue
			}
			value := *field.value
			if field.name == "name" {
				value = strings.TrimSpace(value)
				if value == "" {
					return invalidGKE()
				}
			}
			if value != field.current {
				fields[field.name] = value
				changed = append(changed, field.name)
			}
		}
		if req.CloudKeyID != nil && (m.CloudKeyID == nil || *req.CloudKeyID != *m.CloudKeyID) {
			fields["cloud_key_id"] = *req.CloudKeyID
			changed = append(changed, "cloud_key_id")
		}
		if len(fields) == 0 {
			return nil
		}
		if err := s.repo.UpdateTx(ctx, tx, id, fields); err != nil {
			return err
		}
		name := m.Name
		if updated, ok := fields["name"].(string); ok {
			name = updated
		}
		return s.auditGKE(ctx, tx, actorID, "credentials.update", id, ip, ua, map[string]any{"name": name, "changed_fields": changed})
	})
	if err != nil {
		return nil, safeGKEStoreError(err)
	}
	return s.Get(ctx, id)
}

func safeGKEStoreError(err error) error {
	if _, ok := apperr.AsBiz(err); ok {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credentials_kubeconfigs_name_key" {
		return apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	}
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}

func (s *Service) auditGKE(ctx context.Context, tx *gorm.DB, actorID uint64, action string, id uint64, ip, ua string, payload map[string]any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.WithTx(tx).Record(ctx, audit.Event{
		UserID: ptrIfNonZero(actorID), Action: action, TargetType: "credentials.kubeconfig",
		TargetID: strconv.FormatUint(id, 10), IP: ip, UserAgent: ua, Payload: payload,
	})
}
