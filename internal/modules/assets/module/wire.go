package module

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/config"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/modules/assets"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/account/inuse"
	assetdatabase "github.com/ArkGravity/optimus/internal/modules/assets/database"
	asseterrs "github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/firewall"
	"github.com/ArkGravity/optimus/internal/modules/assets/instance"
	aliyunprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/aliyun"
	awsprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/aws"
	gcpprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/gcp"
	"github.com/ArkGravity/optimus/internal/modules/assets/securitygroup"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/assets/vpc"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
)

const defaultAWSRequestTimeout = 30 * time.Second

type Input struct {
	DB                  *gorm.DB
	Config              config.AssetsConfig
	Audit               *audit.Recorder
	CredentialsConsumer assetsync.CloudKeyConsumer
	Logger              *slog.Logger
	ClientFactory       assetsync.ClientFactory
	Fetcher             assetsync.Fetcher
	AWSIdentity         awsprovider.Identity
	RegionCatalogs      map[string]account.RegionCatalog
	ProviderRegistry    *assetsync.Registry
}

type Module struct {
	Consumer      assets.Consumer
	InUseCounter  inuse.Counter
	Engine        *assetsync.Engine
	CronScheduler *cron.Cron

	accountHandler       *account.Handler
	instanceHandler      *instance.Handler
	firewallHandler      *firewall.Handler
	vpcHandler           *vpc.Handler
	securityGroupHandler *securitygroup.Handler
	databaseHandler      *assetdatabase.Handler
	runsHandler          *runs.Handler
	cancel               context.CancelFunc
	workers              workerGroup
}

func Wire(rootCtx context.Context, in Input) *Module {
	moduleCtx, cancel := context.WithCancel(rootCtx)
	accountService := account.NewService(account.NewRepo(in.DB), in.Audit, in.CredentialsConsumer)
	timeout := normalizeRequestTimeout(in.Config.RequestTimeout)
	accountTimeout := in.Config.SyncTimeout
	if accountTimeout <= 0 {
		accountTimeout = assetsync.DefaultAccountTimeout
	}
	accountService.SetRegionCatalogs(regionCatalogs(in, timeout))
	runsRepo := runs.NewRepo(in.DB)
	registry := providerRegistry(in, timeout)
	engine := assetsync.NewProviderEngine(in.DB, runsRepo, registry, in.CredentialsConsumer, timeout, accountTimeout)

	module := &Module{
		Consumer:             assets.NewConsumer(in.DB),
		firewallHandler:      firewall.NewHandler(firewall.NewService(firewall.NewRepo(in.DB))),
		InUseCounter:         inuse.New(in.DB),
		Engine:               engine,
		instanceHandler:      instance.NewHandler(instance.NewService(instance.NewRepo(in.DB))),
		vpcHandler:           vpc.NewHandler(vpc.NewService(vpc.NewRepo(in.DB))),
		securityGroupHandler: securitygroup.NewHandler(securitygroup.NewService(securitygroup.NewRepo(in.DB))),
		databaseHandler:      assetdatabase.NewHandler(assetdatabase.NewService(assetdatabase.NewRepo(in.DB))),
		runsHandler:          runs.NewHandler(runs.NewService(runsRepo)),
		cancel:               cancel,
	}
	module.accountHandler = account.NewHandler(accountService)
	module.accountHandler.SetTriggerSync(newManualSyncTrigger(moduleCtx, accountService, engine, in.Logger, &module.workers, accountTimeout))
	module.CronScheduler = assetsync.StartScheduler(moduleCtx, assetsync.Config{
		SyncCron:          in.Config.SyncCron,
		StartupDelay:      in.Config.SyncStartupDelay,
		RetentionDays:     in.Config.SyncRunRetentionDays,
		AWSRequestTimeout: normalizeRequestTimeout(in.Config.AWSRequestTimeout),
	}, engine, in.Logger)
	return module
}

func regionCatalogs(in Input, timeout time.Duration) map[string]account.RegionCatalog {
	catalogs := map[string]account.RegionCatalog{
		"aws":    awsprovider.RegionCatalog{Timeout: normalizeRequestTimeout(in.Config.AWSRequestTimeout)},
		"aliyun": aliyunprovider.RegionCatalog{Timeout: timeout},
		"gcp":    gcpprovider.RegionCatalog{Timeout: timeout},
	}
	for name, catalog := range in.RegionCatalogs {
		catalogs[name] = catalog
	}
	return catalogs
}

func providerRegistry(in Input, timeout time.Duration) *assetsync.Registry {
	if in.ProviderRegistry != nil {
		return in.ProviderRegistry
	}
	// Providers are assembled here, never by the shared engine.
	registry, _ := assetsync.NewRegistry(
		awsprovider.New(awsprovider.Options{ClientFactory: in.ClientFactory, Fetcher: in.Fetcher, Identity: in.AWSIdentity, RequestTimeout: normalizeRequestTimeout(in.Config.AWSRequestTimeout)}),
		aliyunprovider.New(aliyunprovider.Options{RequestTimeout: timeout}),
		gcpprovider.New(gcpprovider.Options{RequestTimeout: timeout}),
	)
	return registry
}

// Shutdown cancels scheduled and manual sync work, then waits for both to
// release their locks before the composition root closes the database.
func (m *Module) Shutdown(ctx context.Context) error {
	m.workers.close()
	m.cancel()
	cronDone := m.CronScheduler.Stop().Done()
	workersDone := make(chan struct{})
	go func() {
		m.workers.wait()
		close(workersDone)
	}()
	for cronDone != nil || workersDone != nil {
		select {
		case <-cronDone:
			cronDone = nil
		case <-workersDone:
			workersDone = nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	group := protected.Group("/assets")
	requirePermission := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(cache, code)
	}
	m.accountHandler.Mount(group, requirePermission)
	m.instanceHandler.Mount(group, requirePermission)
	m.firewallHandler.Mount(group, requirePermission)
	m.vpcHandler.Mount(group, requirePermission)
	m.securityGroupHandler.Mount(group, requirePermission)
	m.databaseHandler.Mount(group, requirePermission)
	m.runsHandler.Mount(group, requirePermission)
}

type accountSyncService interface {
	Get(context.Context, uint64) (*account.Detail, error)
	RecordSyncTrigger(context.Context, *uint64, string, string, uint64)
}

type syncEngine interface {
	TryLock(uint64) bool
	Unlock(uint64)
	RunAccountLocked(context.Context, uint64, string, *uint64) error
}

func newManualSyncTrigger(rootCtx context.Context, service accountSyncService, engine syncEngine, logger *slog.Logger, workers *workerGroup, accountTimeout time.Duration) func(*gin.Context, uint64) {
	if accountTimeout <= 0 {
		accountTimeout = assetsync.DefaultAccountTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context, accountID uint64) {
		detail, err := service.Get(c.Request.Context(), accountID)
		if err != nil {
			response.Error(c, err)
			return
		}
		if !detail.Enabled {
			response.Error(c, apperr.New(asseterrs.CodeAssetsCloudAccountDisabled, asseterrs.KeyCloudAccountDisabled, "cloud account is disabled"))
			return
		}
		if !engine.TryLock(accountID) {
			response.Error(c, apperr.New(asseterrs.CodeAssetsSyncBusy, asseterrs.KeySyncBusy, "asset sync already running"))
			return
		}
		if !workers.add() {
			engine.Unlock(accountID)
			response.Error(c, apperr.New(apperr.CodeInternal, "common.internal_error", "asset sync is shutting down"))
			return
		}

		requestCtx := c.Request.Context()
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		userID := c.GetUint64(middleware.CtxKeyUserID)
		var actor *uint64
		if userID != 0 {
			actor = &userID
		}
		service.RecordSyncTrigger(requestCtx, actor, clientIP, userAgent, accountID)

		go func() {
			defer workers.done()
			defer engine.Unlock(accountID)
			// Planning belongs to the worker. HTTP queues with the account ceiling;
			// the engine derives its bounded unit budget after provider discovery.
			workerCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), accountTimeout)
			stopOnShutdown := context.AfterFunc(rootCtx, cancel)
			defer stopOnShutdown()
			defer cancel()
			if err := engine.RunAccountLocked(workerCtx, accountID, "manual", actor); err != nil {
				logger.Error("assets.sync.manual.failed", "account_id", accountID)
			}
		}()

		response.Success(c, gin.H{"queued": true, "started_at": time.Now().UTC()})
	}
}

type workerGroup struct {
	mu      sync.Mutex
	closing bool
	wg      sync.WaitGroup
}

func (g *workerGroup) add() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing {
		return false
	}
	g.wg.Add(1)
	return true
}

func (g *workerGroup) done() { g.wg.Done() }

func (g *workerGroup) close() {
	g.mu.Lock()
	g.closing = true
	g.mu.Unlock()
}

func (g *workerGroup) wait() { g.wg.Wait() }

func normalizeRequestTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultAWSRequestTimeout
	}
	return timeout
}
