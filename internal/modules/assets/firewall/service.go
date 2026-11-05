package firewall

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type repository interface {
	List(context.Context, query.Filter) ([]Summary, int64, error)
	FindByKey(context.Context, string, bool) (*Detail, error)
}
type Service struct{ repo repository }

func NewService(repo repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, input ListQuery) (*ListResponse, error) {
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Size == 0 {
		input.Size = 20
	}
	f := query.Filter{
		Provider: strings.TrimSpace(input.Provider), AccountID: input.AccountID,
		ProjectID: strings.TrimSpace(input.ProjectID), Region: strings.TrimSpace(input.Region),
		Zone: strings.TrimSpace(input.Zone), Q: strings.TrimSpace(input.Q),
		NetworkRef: strings.TrimSpace(input.NetworkRef), IncludeDeleted: input.IncludeDeleted,
		Page: input.Page, Size: input.Size,
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	rows, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, databaseError(err)
	}
	if rows == nil {
		rows = []Summary{}
	}
	return &ListResponse{Items: rows, Total: total}, nil
}

func (s *Service) FindByKey(ctx context.Context, key string, deleted bool) (*Detail, error) {
	if _, err := resource.ParseAssetKey(key, "firewall"); err != nil {
		return nil, apperr.New(apperr.CodeValidation, "common.validation", "invalid asset key")
	}
	row, err := s.repo.FindByKey(ctx, key, deleted)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && row == nil) {
		return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "asset not found")
	}
	if err != nil {
		return nil, databaseError(err)
	}
	return row, nil
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}
