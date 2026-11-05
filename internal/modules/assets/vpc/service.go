package vpc

import (
	"context"
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

const (
	defaultPage = 1
	defaultSize = 20
	maxSize     = 200
)

type repository interface {
	List(context.Context, ListFilter) ([]Summary, int64, error)
	FindByID(context.Context, uint64) (*models.AWSVPC, error)
	ListSubnets(context.Context, SubnetListFilter) ([]SubnetSummary, int64, error)
}

type Service struct{ repo repository }

func NewService(repo repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	filter, err := validateListQuery(query)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, databaseError(err)
	}
	if items == nil {
		items = []Summary{}
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func (s *Service) ListSubnetsByVPCRowID(ctx context.Context, vpcRowID uint64, query SubnetListQuery) (*SubnetListResponse, error) {
	if vpcRowID == 0 || vpcRowID > math.MaxInt64 {
		return nil, validationError("vpc id is outside the supported range")
	}
	page, size, err := validatePagination(query.Page, query.Size)
	if err != nil {
		return nil, err
	}
	vpc, err := s.repo.FindByID(ctx, vpcRowID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && vpc == nil) {
		return nil, apperr.Wrap(err, errs.CodeAssetsVPCNotFound, errs.KeyVPCNotFound, "VPC not found")
	}
	if err != nil {
		return nil, databaseError(err)
	}
	items, total, err := s.repo.ListSubnets(ctx, SubnetListFilter{
		CloudAccountID: vpc.CloudAccountID, Region: vpc.Region, VPCID: vpc.VPCID,
		Q: strings.TrimSpace(query.Q), IncludeDeleted: query.IncludeDeleted, Page: page, Size: size,
	})
	if err != nil {
		return nil, databaseError(err)
	}
	if items == nil {
		items = []SubnetSummary{}
	}
	return &SubnetListResponse{Items: items, Total: total}, nil
}

func validatePagination(page, size int) (int, int, error) {
	if page == 0 {
		page = defaultPage
	}
	if size == 0 {
		size = defaultSize
	}
	if page < 1 {
		return 0, 0, validationError("page must be at least 1")
	}
	if size < 1 || size > maxSize {
		return 0, 0, validationError("size must be between 1 and 200")
	}
	if page-1 > math.MaxInt/size {
		return 0, 0, validationError("pagination offset exceeds supported range")
	}
	return page, size, nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}

func validateListQuery(input ListQuery) (ListFilter, error) {
	if input.Page == 0 {
		input.Page = 1
	}
	if input.Size == 0 {
		input.Size = 20
	}
	f := ListFilter{
		Provider:       strings.TrimSpace(input.Provider),
		AccountID:      input.AccountID,
		ProjectID:      strings.TrimSpace(input.ProjectID),
		Region:         strings.TrimSpace(input.Region),
		Zone:           strings.TrimSpace(input.Zone),
		Q:              strings.TrimSpace(input.Q),
		IncludeDeleted: input.IncludeDeleted,
		Page:           input.Page,
		Size:           input.Size,
	}
	if err := f.queryFilter().Validate(); err != nil {
		return ListFilter{}, err
	}
	return f, nil
}

// FindByKey accepts canonical provider keys and legacy numeric AWS IDs.
func (s *Service) FindByKey(ctx context.Context, key string, includeDeleted bool) (*Summary, error) {
	if _, err := resource.ParseAssetKey(key, "vpc"); err != nil {
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
		return nil, apperr.New(errs.CodeAssetsVPCNotFound, errs.KeyVPCNotFound, "VPC not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	return row, nil
}

// ListSubnetsByVPCKey scopes children through the full provider-native parent identity.
func (s *Service) ListSubnetsByVPCKey(ctx context.Context, key string, input ListQuery) (*SubnetListResponse, error) {
	if _, err := resource.ParseAssetKey(key, "vpc"); err != nil {
		return nil, validationError("invalid asset key")
	}
	filter, err := validateListQuery(input)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repo.(interface {
		ListSubnetsByVPC(context.Context, string, ListFilter) ([]SubnetSummary, int64, error)
	})
	if !ok {
		return nil, apperr.New(apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	rows, total, err := repo.ListSubnetsByVPC(ctx, key, filter)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(errs.CodeAssetsVPCNotFound, errs.KeyVPCNotFound, "VPC not found")
	}
	if err != nil {
		return nil, databaseError(err)
	}
	if rows == nil {
		rows = []SubnetSummary{}
	}
	return &SubnetListResponse{Items: rows, Total: total}, nil
}
