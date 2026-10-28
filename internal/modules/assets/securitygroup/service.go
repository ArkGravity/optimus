package securitygroup

import (
	"context"
	"errors"
	"math"
	"strings"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"gorm.io/gorm"
)

type repository interface {
	List(context.Context, ListQuery) ([]Summary, int64, error)
	Get(context.Context, uint64) (*Detail, error)
}
type Service struct{ repo repository }

func NewService(repo repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Size == 0 {
		q.Size = 20
	}
	if q.AccountID > math.MaxInt64 || q.Page < 1 || q.Size < 1 || q.Size > 200 || q.Page-1 > math.MaxInt/q.Size {
		return nil, apperr.New(apperr.CodeValidation, "common.validation", "invalid resource query")
	}
	q.Q, q.Region, q.VPCID = strings.TrimSpace(q.Q), strings.TrimSpace(q.Region), strings.TrimSpace(q.VPCID)
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, databaseError(err)
	}
	if items == nil {
		items = []Summary{}
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	if id == 0 || id > math.MaxInt64 {
		return nil, apperr.New(apperr.CodeValidation, "common.validation", "invalid security group id")
	}
	item, err := s.repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "security group not found")
	}
	if err != nil {
		return nil, databaseError(err)
	}
	return item, nil
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}
