package host

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// preloadKeyMeta joins only non-secret SSH key columns.
func preloadKeyMeta(db *gorm.DB) *gorm.DB { return db.Select("id", "name", "username") }

func (r *Repo) Create(ctx context.Context, m *models.DockerHost) error {
	return mapWriteError(r.db.WithContext(ctx).Omit("SSHKey").Create(m).Error)
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.DockerHost, error) {
	var m models.DockerHost
	if err := r.db.WithContext(ctx).Preload("SSHKey", preloadKeyMeta).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) GetForUpdate(ctx context.Context, tx *gorm.DB, id uint64) (*models.DockerHost, error) {
	var m models.DockerHost
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.DockerHost, error) {
	var m models.DockerHost
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// SSHKeyMeta returns the non-secret columns of an SSH key credential.
func (r *Repo) SSHKeyMeta(ctx context.Context, id uint64) (*models.CredentialSSHKey, error) {
	var k models.CredentialSSHKey
	if err := r.db.WithContext(ctx).Select("id", "name", "username").First(&k, id).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.DockerHost, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.DockerHost{})
	if s := strings.TrimSpace(q.Search); s != "" {
		pat := "%" + s + "%"
		tx = tx.Where("name ILIKE ? OR description ILIKE ? OR host ILIKE ?", pat, pat, pat)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.DockerHost
	if err := tx.Preload("SSHKey", preloadKeyMeta).
		Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Update applies a fields map. A nil value clears a nullable column.
func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return mapWriteError(r.db.WithContext(ctx).
		Model(&models.DockerHost{}).
		Where("id = ?", id).
		Updates(fields).Error)
}

// SoftDeleteTx marks the row deleted and drops its sealed password in the same
// statement, so historical rows never retain secret material.
func (r *Repo) SoftDeleteTx(ctx context.Context, tx *gorm.DB, id uint64) error {
	return tx.WithContext(ctx).Model(&models.DockerHost{}).
		Where("id = ?", id).
		Updates(map[string]any{"deleted_at": time.Now(), "password_enc": nil}).Error
}

func (r *Repo) UpdateCheck(ctx context.Context, id uint64, ok bool, msg string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.DockerHost{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_checked_at":  at,
			"last_check_ok":    ok,
			"last_check_error": msg,
		}).Error
}

// mapWriteError translates races that slip past service validation: the SSH
// key disappearing before the FK write, or a concurrent duplicate live name.
func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.Code == "23503" && pgErr.ConstraintName == "docker_hosts_ssh_key_id_fkey":
		return apperr.Wrap(err, apperr.CodeNotFound, "credentials.not_found", "credential not found")
	case pgErr.Code == "23505" && pgErr.ConstraintName == "docker_hosts_name_unique":
		return apperr.Wrap(err, apperr.CodeConflict, "docker.host.name_taken", "Docker host name already exists")
	}
	return err
}
