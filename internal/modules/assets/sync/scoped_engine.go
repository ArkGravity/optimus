package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

// NewProviderEngine receives an immutable registry assembled by composition.
// Providers own planning, fresh SDK clients and typed snapshot publishers.
func NewProviderEngine(db *gorm.DB, runRepo *runs.Repo, registry *Registry, consumer CloudKeyConsumer, requestTimeout, accountTimeout time.Duration) *Engine {
	if registry == nil {
		registry = &Registry{}
	}
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	return &Engine{db: db, runs: runRepo, registry: registry, consumer: consumer, timeout: requestTimeout, accountTimeout: normalizeAccountTimeout(accountTimeout)}
}

func normalizeAccountTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultAccountTimeout
	}
	return timeout
}

func (e *Engine) runScopedAccount(ctx context.Context, row models.CloudAccount, trigger string, actor *uint64) error {
	started := time.Now()
	// Planning is account-level metadata, independent of a resource location.
	planning := Unit{ResourceType: "planning", LocationType: "global", Location: "global", Scope: account.CaptureScope(row)}
	runID, err := e.insertScopedRun(ctx, planning, trigger, actor)
	if err != nil {
		return err
	}
	provider, err := e.registry.Lookup(row.Provider)
	if err != nil {
		return e.finishScopedFailure(ctx, runID, err)
	}
	if !row.Enabled || row.CloudKeyID == 0 || (row.Provider == "gcp" && row.ProjectID == "") {
		return e.finishScopedFailure(ctx, runID, planConfigError())
	}
	planCtx, cancel := context.WithTimeout(ctx, e.planningTimeout(provider))
	plan, err := e.providerPlan(planCtx, provider, row, trigger, actor == nil)
	if err == nil {
		err = planCtx.Err()
	}
	cancel()
	if err == nil {
		err = validatePlan(row, plan)
	}
	if err == nil {
		err = e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			updated, pinErr := account.NewRepo(e.db).PinIdentityTx(ctx, tx, account.CaptureScope(row), plan.ExternalAccountID)
			if pinErr != nil {
				return pinErr
			}
			// First pin changes ExternalAccountID. Every unit must capture the
			// returned row so publication compares the newly verified identity.
			row = *updated
			return nil
		})
	}
	if err != nil {
		return e.finishScopedFailure(ctx, runID, err)
	}
	if err = e.finishRun(ctx, runID, runs.FinishRequest{Status: "success"}); err != nil {
		return err
	}
	for i := range plan.Units {
		plan.Units[i].Scope = account.CaptureScope(row)
	}
	// The inherited account deadline accounts for time already spent planning.
	// Saturation bounds even a very large provider plan without duration overflow.
	ctx, cancelUnits := context.WithDeadline(ctx, started.Add(boundedPlanBudget(plan, e.accountTimeout)))
	defer cancelUnits()
	var failures []error
	for _, unit := range plan.Units {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if err := e.runUnit(ctx, provider, unit, trigger, actor); err != nil {
			failures = append(failures, err)
			if errors.Is(err, ErrIdentityVerification) {
				break
			}
			if biz, ok := apperr.AsBiz(err); ok && biz.Code == errs.CodeAssetsIdentityMismatch {
				break
			}
		}
	}
	return errors.Join(failures...)
}

func (e *Engine) providerPlan(ctx context.Context, provider Provider, row models.CloudAccount, trigger string, system bool) (Plan, error) {
	key, err := e.consumer.GetCloudKey(ctx, row.CloudKeyID, providerSyncPurpose(row.ID, row.Provider, "planning", trigger, system))
	defer credentials.Wipe(key)
	if err != nil {
		return Plan{}, err
	}
	if err := validateProviderKey(row.Provider, key); err != nil {
		return Plan{}, err
	}
	return provider.Plan(ctx, row, key)
}

func (e *Engine) runUnit(ctx context.Context, provider Provider, unit Unit, trigger string, actor *uint64) error {
	runID, err := e.insertScopedRun(ctx, unit, trigger, actor)
	if err != nil {
		return err
	}
	unitCtx, cancel := context.WithTimeout(ctx, unit.Budget)
	defer cancel()
	snapshot, err := e.providerFetch(unitCtx, provider, unit, trigger, actor == nil)
	if err == nil {
		err = unitCtx.Err()
	}
	if err != nil {
		finished := e.finishScopedFailure(unitCtx, runID, err)
		if errors.Is(err, ErrIdentityVerification) {
			return errors.Join(finished, ErrIdentityVerification)
		}
		return finished
	}
	stats, err := PublishSnapshot(unitCtx, e.db, unit, snapshot)
	if err != nil {
		return e.finishScopedFailure(unitCtx, runID, err)
	}
	return e.finishRun(unitCtx, runID, runs.FinishRequest{Status: "success", ItemsSeen: saturatingInt32(stats.Seen), ItemsSoftDeleted: saturatingInt32(stats.SoftDeleted)})
}

func (e *Engine) providerFetch(ctx context.Context, provider Provider, unit Unit, trigger string, system bool) (Snapshot, error) {
	key, err := e.consumer.GetCloudKey(ctx, unit.Scope.CloudKeyID, providerSyncPurpose(unit.Scope.AccountID, unit.Scope.Provider, unit.Key, trigger, system))
	defer credentials.Wipe(key)
	if err != nil {
		return Snapshot{}, err
	}
	if err := validateProviderKey(unit.Scope.Provider, key); err != nil {
		return Snapshot{}, err
	}
	return provider.Fetch(ctx, unit, key)
}

func validateProviderKey(provider string, key *credentials.CloudKey) error {
	if key == nil || key.Provider != provider {
		return planConfigError()
	}
	switch provider {
	case "aws", "aliyun":
		if key.AuthType == "access_key" && key.AccessKeyID != "" && key.SecretAccessKey != "" && len(key.ServiceAccountJSON) == 0 {
			return nil
		}
	case "gcp":
		if key.AuthType == "service_account_json" && len(key.ServiceAccountJSON) > 0 && key.AccessKeyID == "" && key.SecretAccessKey == "" {
			return nil
		}
	}
	return planConfigError()
}

func providerSyncPurpose(accountID uint64, provider, unit, trigger string, system bool) string {
	prefix := ""
	if system {
		prefix = "system:"
	}
	return fmt.Sprintf("%sassets.sync.%s.account-%d.%s.%s", prefix, trigger, accountID, provider, unit)
}

func (e *Engine) insertScopedRun(ctx context.Context, unit Unit, trigger string, actor *uint64) (uint64, error) {
	id, err := e.runs.Insert(ctx, runs.InsertRequest{CloudAccountID: unit.Scope.AccountID, Provider: unit.Scope.Provider, ScopeRevision: unit.Scope.Revision, Region: unit.Region, LocationType: unit.LocationType, Location: unit.Location, ResourceType: unit.ResourceType, Trigger: trigger, TriggeredByUserID: actor})
	if err != nil {
		return 0, safeDatabaseError()
	}
	return id, nil
}

func (e *Engine) finishScopedFailure(ctx context.Context, runID uint64, cause error) error {
	safe := safeProviderError(cause)
	finishErr := e.finishRun(ctx, runID, runs.FinishRequest{Status: "failed", Error: safe.Message, ErrorCode: saturatingInt32(int64(safe.Code))})
	var canceled error
	if errors.Is(cause, context.Canceled) {
		canceled = context.Canceled
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		canceled = context.DeadlineExceeded
	}
	return errors.Join(safe, finishErr, canceled)
}

func safeProviderError(cause error) *apperr.BizError {
	if errors.Is(cause, ErrIncompleteSnapshot) {
		return apperr.New(errs.CodeAssetsPartial, errs.KeyPartial, "asset snapshot is incomplete")
	}
	if errors.Is(cause, account.ErrScopeIneligible) {
		return apperr.New(errs.CodeAssetsConfig, errs.KeyConfig, "asset collection scope changed")
	}
	if biz, ok := apperr.AsBiz(cause); ok {
		switch biz.Code {
		case errs.CodeAssetsUnauthorized, errs.CodeAssetsForbidden, errs.CodeAssetsUnreachable, errs.CodeAssetsThrottled, errs.CodeAssetsAPIDisabled:
			return errs.ProviderError(biz.Code)
		case errs.CodeAssetsConfig:
			return planConfigError()
		case errs.CodeAssetsPartial:
			return apperr.New(errs.CodeAssetsPartial, errs.KeyPartial, "asset snapshot is incomplete")
		case errs.CodeAssetsIdentityMismatch:
			return apperr.New(errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch, "verified provider identity does not match this cloud account")
		case errs.CodeAssetsProviderUnsupported:
			return apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "cloud provider not supported")
		case apperr.CodeDBError:
			return apperr.New(apperr.CodeDBError, "common.database_error", "asset persistence failed")
		case errs.CodeAssetsAWSUnauthorized, errs.CodeAssetsAWSForbidden, errs.CodeAssetsAWSUnreachable, errs.CodeAssetsAWSThrottled, errs.CodeAssetsAWSOther, errs.CodeAssetsAWSConfig:
			code, message := safeSyncRunError(cause)
			keys := map[apperr.Code]string{errs.CodeAssetsAWSUnauthorized: errs.KeyAWSUnauthorized, errs.CodeAssetsAWSForbidden: errs.KeyAWSForbidden, errs.CodeAssetsAWSUnreachable: errs.KeyAWSUnreachable, errs.CodeAssetsAWSThrottled: errs.KeyAWSThrottled, errs.CodeAssetsAWSOther: errs.KeyAWSOther, errs.CodeAssetsAWSConfig: errs.KeyAWSConfig}
			return apperr.New(code, keys[code], message)
		}
	}
	return apperr.New(errs.CodeAssetsOther, errs.KeyOther, "asset collection failed")
}

// planningTimeout lets adapters preserve provider-specific request overrides
// without coupling the shared engine to any provider package.
func (e *Engine) planningTimeout(provider Provider) time.Duration {
	if timed, ok := provider.(interface{ RequestTimeout() time.Duration }); ok && timed.RequestTimeout() > 0 {
		return timed.RequestTimeout()
	}
	return e.timeout
}
