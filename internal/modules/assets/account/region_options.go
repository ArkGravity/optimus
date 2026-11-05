package account

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/advisorylock"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type RegionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RegionOptionsQuery struct {
	Provider   string `form:"provider" binding:"required"`
	CloudKeyID uint64 `form:"cloudkey_id" binding:"required"`
	ProjectID  string `form:"project_id"`
}

type RegionOptionsResponse struct {
	Items []RegionOption `json:"items"`
}

// RegionCatalog adapters create fresh provider clients using only this selected
// credential. SDK integration belongs to each provider; no default catalog or
// implicit region scan is available. Adapters must honor context cancellation.
type RegionCatalog interface {
	Regions(ctx context.Context, key *credentials.CloudKey, projectID string) ([]RegionOption, error)
}

// SetRegionCatalogs configures adapters during composition, before serving.
func (s *Service) SetRegionCatalogs(catalogs map[string]RegionCatalog) {
	s.catalogs = make(map[string]RegionCatalog, len(catalogs))
	for provider, catalog := range catalogs {
		s.catalogs[provider] = catalog
	}
}

func (s *Service) RegionOptions(ctx context.Context, actorID uint64, query RegionOptionsQuery) (*RegionOptionsResponse, error) {
	if err := validateTarget(query.Provider, query.ProjectID); err != nil {
		return nil, err
	}
	catalog := s.catalogs[query.Provider]
	if catalog == nil || s.consumer == nil {
		return nil, configError("region catalog is not configured for this provider")
	}
	timeout := 30 * time.Second
	if timed, ok := catalog.(interface{ RequestTimeout() time.Duration }); ok && timed.RequestTimeout() > 0 {
		timeout = timed.RequestTimeout()
	}
	ctx, cancel := context.WithTimeout(credentials.WithActor(ctx, actorID), timeout)
	defer cancel()
	if err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := advisorylock.LockCloudKey(ctx, tx, query.CloudKeyID); err != nil {
			return err
		}
		return s.repo.ValidateCloudKeyTx(ctx, tx, query.CloudKeyID, query.Provider)
	}); err != nil {
		return nil, err
	}
	key, err := s.consumer.GetCloudKey(ctx, query.CloudKeyID, "assets.region_options")
	defer credentials.Wipe(key)
	if err != nil {
		return nil, safeRegionCatalogError(err)
	}
	if key == nil || key.Provider != query.Provider ||
		(query.Provider == "gcp" && (key.AuthType != "service_account_json" || len(key.ServiceAccountJSON) == 0)) ||
		(query.Provider != "gcp" && (key.AuthType != "access_key" || key.AccessKeyID == "" || key.SecretAccessKey == "")) {
		return nil, configError("cloud key does not provide usable authentication for this provider")
	}
	items, err := catalog.Regions(ctx, key, query.ProjectID)
	if err != nil {
		return nil, safeRegionCatalogError(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, safeRegionCatalogError(err)
	}
	if items == nil {
		items = []RegionOption{}
	}
	return &RegionOptionsResponse{Items: items}, nil
}

func safeRegionCatalogError(err error) error {
	safe := errs.ProviderError(errs.CodeAssetsConfig)
	if biz, ok := apperr.AsBiz(err); ok {
		switch biz.Code {
		case errs.CodeAssetsUnauthorized, errs.CodeAssetsForbidden, errs.CodeAssetsUnreachable, errs.CodeAssetsThrottled, errs.CodeAssetsConfig, errs.CodeAssetsPartial, errs.CodeAssetsAPIDisabled, errs.CodeAssetsOther, errs.CodeAssetsIdentityMismatch:
			safe = errs.ProviderError(biz.Code)
		case errs.CodeAssetsAWSUnauthorized:
			safe = apperr.New(biz.Code, errs.KeyAWSUnauthorized, "AWS credentials invalid or expired")
		case errs.CodeAssetsAWSForbidden:
			safe = apperr.New(biz.Code, errs.KeyAWSForbidden, "AWS API access denied")
		case errs.CodeAssetsAWSUnreachable:
			safe = apperr.New(biz.Code, errs.KeyAWSUnreachable, "AWS API unreachable")
		case errs.CodeAssetsAWSThrottled:
			safe = apperr.New(biz.Code, errs.KeyAWSThrottled, "AWS API request throttled")
		case errs.CodeAssetsAWSConfig:
			safe = apperr.New(biz.Code, errs.KeyAWSConfig, "AWS SDK configuration failed")
		case errs.CodeAssetsAWSOther:
			safe = apperr.New(biz.Code, errs.KeyAWSOther, "AWS API request failed")
		}
	}
	if errors.Is(err, context.Canceled) {
		return errors.Join(safe, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(safe, context.DeadlineExceeded)
	}
	return safe
}
