package sync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/awsclient"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type Clients = awsclient.Clients

type ClientFactory interface {
	For(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error)
}

type ClientFactoryFunc func(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error)

func (f ClientFactoryFunc) For(ctx context.Context, key *credentials.CloudKey, region string, timeout time.Duration) (*Clients, error) {
	return f(ctx, key, region, timeout)
}

type CloudKeyConsumer interface {
	GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error)
}

type Fetcher interface {
	FetchInstances(context.Context, *Clients) ([]models.AWSInstance, error)
	FetchVPCsAndSubnets(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error)
	FetchDatabases(context.Context, *Clients) ([]models.AWSDatabase, error)
	FetchSecurityGroups(context.Context, *Clients) ([]models.AWSSecurityGroup, error)
}

type Engine struct {
	db             *gorm.DB
	runs           *runs.Repo
	consumer       CloudKeyConsumer
	timeout        time.Duration
	locks          sync.Map
	registry       *Registry
	accountTimeout time.Duration
}

const syncRunFinishTimeout = 5 * time.Second

func (e *Engine) TryLock(accountID uint64) bool {
	_, loaded := e.locks.LoadOrStore(accountID, struct{}{})
	return !loaded
}

func (e *Engine) Unlock(accountID uint64) { e.locks.Delete(accountID) }

func (e *Engine) RunAll(ctx context.Context, trigger string) error {
	var accounts []models.CloudAccount
	if err := e.db.WithContext(ctx).Where("enabled = ?", true).Find(&accounts).Error; err != nil {
		return safeDatabaseError()
	}
	var accountErrors []error
	for i := range accounts {
		if err := ctx.Err(); err != nil {
			accountErrors = append(accountErrors, err)
			break
		}
		if err := e.RunAccount(ctx, accounts[i].ID, trigger, nil); err != nil {
			// A failed account cannot prevent the remaining eligible accounts from
			// receiving their scheduled sweep, but the scheduler must still be
			// told that this RunAll invocation was incomplete.
			accountErrors = append(accountErrors, fmt.Errorf("cloud account %d: %w", accounts[i].ID, err))
		}
	}
	return errors.Join(accountErrors...)
}

func (e *Engine) RunAccount(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	if !e.TryLock(accountID) {
		return e.recordLockedSkip(ctx, accountID, trigger, triggeredBy)
	}
	defer e.Unlock(accountID)
	return e.RunAccountLocked(ctx, accountID, trigger, triggeredBy)
}

// RunAccountLocked runs an account whose lock is already owned by the caller.
// The asynchronous manual-sync handler uses this entry point so the worker,
// rather than the HTTP handler, owns release of the account lock.
func (e *Engine) RunAccountLocked(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	ctx, cancel := context.WithTimeout(ctx, normalizeAccountTimeout(e.accountTimeout))
	defer cancel()
	if triggeredBy != nil {
		ctx = credentials.WithActor(ctx, *triggeredBy)
	}
	var account models.CloudAccount
	if err := e.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(errs.CodeAssetsCloudAccountNotFound, errs.KeyCloudAccountNotFound, "cloud account not found")
		}
		return safeDatabaseError()
	}
	return e.runScopedAccount(ctx, account, trigger, triggeredBy)
}

func (e *Engine) insertRun(ctx context.Context, accountID uint64, region, resourceType, trigger string, triggeredBy *uint64) (uint64, error) {
	id, err := e.runs.Insert(ctx, runs.InsertRequest{CloudAccountID: accountID, Region: region, ResourceType: resourceType, Trigger: trigger, TriggeredByUserID: triggeredBy})
	if err != nil {
		return 0, safeDatabaseError()
	}
	return id, nil
}

func (e *Engine) finishRun(ctx context.Context, runID uint64, request runs.FinishRequest) error {
	// The request/fetch context commonly carries the cancellation that caused
	// the sweep to fail. Finalization gets a small independent budget so a run
	// does not remain "running" forever after cancellation or timeout.
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	finishCtx, cancel := context.WithTimeout(base, syncRunFinishTimeout)
	defer cancel()
	if err := e.runs.Finish(finishCtx, runID, request); err != nil {
		return safeDatabaseError()
	}
	return nil
}

func safeSyncRunError(cause error) (apperr.Code, string) {
	if biz, ok := apperr.AsBiz(cause); ok {
		switch biz.Code {
		case errs.CodeAssetsRegionInvalid:
			return biz.Code, "AWS region is invalid"
		case errs.CodeAssetsProviderUnsupported:
			return biz.Code, "cloud provider not supported"
		case errs.CodeAssetsCloudKeyNotFound:
			return biz.Code, "cloud key not found"
		case errs.CodeAssetsAWSUnauthorized:
			return biz.Code, "AWS credentials invalid or expired"
		case errs.CodeAssetsAWSForbidden:
			return biz.Code, "AWS API access denied"
		case errs.CodeAssetsAWSUnreachable:
			return biz.Code, "AWS API unreachable"
		case errs.CodeAssetsAWSThrottled:
			return biz.Code, "AWS API request throttled"
		case errs.CodeAssetsAWSOther:
			return biz.Code, "AWS API request failed"
		case errs.CodeAssetsAWSConfig:
			return biz.Code, "AWS SDK configuration failed"
		}
	}
	code, _, message := awsclient.MapError(cause)
	return code, message
}

func (e *Engine) recordLockedSkip(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	var account models.CloudAccount
	if err := e.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return safeDatabaseError()
	}
	region := ""
	if len(account.EnabledRegions) > 0 {
		region = account.EnabledRegions[0]
	}
	runID, err := e.insertRun(ctx, accountID, region, "instance", trigger, triggeredBy)
	if err != nil {
		return err
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "skipped"})
}

func (e *Engine) PruneSyncRuns(ctx context.Context, days int) (int64, error) {
	count, err := e.runs.Prune(ctx, days)
	if err != nil {
		return 0, safeDatabaseError()
	}
	return count, nil
}

func safeDatabaseError() error {
	return apperr.New(apperr.CodeDBError, "common.database_error", "database operation failed")
}

func saturatingInt32(value int64) int32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func saturatingAdd(left, right int64) int64 {
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	if left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
