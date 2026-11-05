package instance

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type listRepository interface {
	List(context.Context, ListFilter) ([]Summary, int64, error)
}

type Service struct{ repo listRepository }

func NewService(repo listRepository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	filter, err := validateListQuery(query)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	if items == nil {
		items = []Summary{}
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func validateListQuery(input ListQuery) (ListFilter, error) {
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Size == 0 {
		input.Size = 20
	}
	f := ListFilter{
		AccountID:      input.AccountID,
		IncludeDeleted: input.IncludeDeleted,
		Page:           input.Page,
		Size:           input.Size,
		Provider:       strings.TrimSpace(input.Provider),
		ProjectID:      strings.TrimSpace(input.ProjectID),
		Region:         strings.TrimSpace(input.Region),
		Zone:           strings.TrimSpace(input.Zone),
		Q:              strings.TrimSpace(input.Q),
		State:          strings.TrimSpace(input.State),
		VPCID:          strings.TrimSpace(input.VPCID),
		IP:             strings.TrimSpace(input.IP),
		TagKey:         strings.TrimSpace(input.TagKey),
		TagValue:       strings.TrimSpace(input.TagValue),
	}
	if err := f.queryFilter().Validate(); err != nil {
		return ListFilter{}, err
	}
	f.Offset = (f.Page - 1) * f.Size
	return f, nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

// FindByKey accepts canonical provider keys and legacy numeric AWS IDs.
func (s *Service) FindByKey(ctx context.Context, key string, includeDeleted bool) (*Summary, error) {
	if _, err := resource.ParseAssetKey(key, "instance"); err != nil {
		return nil, validationError("invalid asset key")
	}
	repo, ok := s.repo.(interface {
		FindByKey(context.Context, string, bool) (*Summary, error)
	})
	if !ok {
		return nil, apperr.New(apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	row, err := repo.FindByKey(ctx, key, includeDeleted)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && row == nil) {
		return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "asset not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	return row, nil
}
