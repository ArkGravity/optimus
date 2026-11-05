//go:build dbtest

package aws

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/db"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

type fakeFetcher struct {
	db            *gorm.DB
	accountID     uint64
	disableOnCall bool
	instances     []models.AWSInstance
	instanceErr   error
	cancel        context.CancelFunc
}

func (f *fakeFetcher) FetchInstances(context.Context, *Clients) ([]models.AWSInstance, error) {
	if f.cancel != nil {
		f.cancel()
	}
	if f.disableOnCall {
		if err := f.db.Model(&models.CloudAccount{}).Where("id = ?", f.accountID).Update("enabled", false).Error; err != nil {
			return nil, err
		}
	}
	return f.instances, f.instanceErr
}

func (*fakeFetcher) FetchVPCsAndSubnets(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	return nil, nil, nil
}

func (*fakeFetcher) FetchSecurityGroups(context.Context, *Clients) ([]models.AWSSecurityGroup, error) {
	return nil, nil
}

func (*fakeFetcher) FetchDatabases(context.Context, *Clients) ([]models.AWSDatabase, error) {
	return nil, nil
}

type countingConsumer struct {
	mu       stdsync.Mutex
	purposes []string
	keys     []*credentials.CloudKey
	rotateAt int
}

func (c *countingConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "access", SecretAccessKey: "secret"}
	if c.rotateAt > 0 && len(c.keys)+1 >= c.rotateAt {
		key.AccessKeyID = "rotated-access"
	}
	c.purposes = append(c.purposes, purpose)
	c.keys = append(c.keys, key)
	return key, nil
}

type countingFactory struct {
	calls int
	err   error
}

func (f *countingFactory) For(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &Clients{}, nil
}

func setupEngine(t *testing.T, fetcher assetsync.Fetcher) (*gorm.DB, *models.CloudAccount, *assetsync.Engine, *countingConsumer, *countingFactory) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "engine-key-"+t.Name())
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "engine-account-"+t.Name(), "us-east-1")
	consumer := &countingConsumer{}
	factory := &countingFactory{}
	engine := newFixtureEngine(gdb, runs.NewRepo(gdb), factory, fetcher, consumer, time.Second)
	return gdb, account, engine, consumer, factory
}

func TestEngineRunAccountUsesFreshCredentialsAndClientsPerSweep(t *testing.T) {
	fetcher := &fakeFetcher{instances: []models.AWSInstance{{InstanceID: "i-a"}}}
	gdb, account, engine, consumer, factory := setupEngine(t, fetcher)

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	require.Equal(t, 4, factory.calls)
	require.Len(t, consumer.purposes, 5)
	for _, purpose := range consumer.purposes {
		require.True(t, strings.HasPrefix(purpose, "system:assets.sync.cron.account-"), purpose)
	}
	for _, key := range consumer.keys {
		require.Empty(t, key.AccessKeyID)
		require.Empty(t, key.SecretAccessKey)
	}
	var runsFound []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&runsFound).Error)
	require.Len(t, runsFound, 5)
	for _, run := range runsFound {
		require.Equal(t, "success", run.Status)
	}
}

func TestEngineFetcherFailureIsSanitizedAndDoesNotSoftDelete(t *testing.T) {
	secretError := errors.New("upstream failed with secret-access-material")
	fetcher := &fakeFetcher{instanceErr: secretError}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)
	dbtest.SeedAWSInstance(t, gdb, account.ID, "us-east-1", "i-old")

	require.Error(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.NotContains(t, run.Error, "secret-access-material")
	var alive int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("instance_id = ?", "i-old").Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}

func TestEngineRejectsPublicationWhenPersistenceGateClosesAfterFetch(t *testing.T) {
	fetcher := &fakeFetcher{disableOnCall: true, instances: []models.AWSInstance{{InstanceID: "i-new"}}}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)
	fetcher.db, fetcher.accountID = gdb, account.ID

	require.Error(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	var count int64
	require.NoError(t, gdb.Unscoped().Model(&models.AWSInstance{}).Where("instance_id = ?", "i-new").Count(&count).Error)
	require.Zero(t, count)
}

func TestEngineLockedRunWritesOneSkippedRowWithoutCredentialAudit(t *testing.T) {
	gdb, account, engine, consumer, _ := setupEngine(t, &fakeFetcher{})
	require.True(t, engine.TryLock(account.ID))
	defer engine.Unlock(account.ID)

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var rows []models.AssetsSyncRun
	require.NoError(t, gdb.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "skipped", rows[0].Status)
	require.Empty(t, consumer.purposes)
}

func TestEngineCancellationFinalizesRunningRunAsFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &fakeFetcher{instanceErr: context.Canceled, cancel: cancel}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)

	err := engine.RunAccount(ctx, account.ID, "cron", nil)
	require.ErrorIs(t, err, context.Canceled)

	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.NotNil(t, run.FinishedAt)
	var running int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("status = ?", "running").Count(&running).Error)
	require.Zero(t, running)
}

func TestEngineFactoryConfigErrorKeepsSafe43107Code(t *testing.T) {
	gdb, account, engine, _, factory := setupEngine(t, &fakeFetcher{})
	factory.err = apperr.Wrap(errors.New("secret-config-cause"), errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "failed to load AWS SDK configuration")

	require.Error(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var found []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&found).Error)
	require.Len(t, found, 5)
	require.Equal(t, "success", found[0].Status)
	for _, run := range found[1:] {
		require.Equal(t, "failed", run.Status)
		require.EqualValues(t, errs.CodeAssetsAWSConfig, run.ErrorCode)
		require.NotContains(t, run.Error, "secret-config-cause")
	}
}

func TestEngineRunAllContinuesAfterAccountErrorAndReturnsAggregate(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "run-all-key-"+t.Name())
	broken := dbtest.SeedCloudAccount(t, gdb, key.ID, "run-all-broken-"+t.Name(), strings.Repeat("r", 33))
	healthy := dbtest.SeedCloudAccount(t, gdb, key.ID, "run-all-healthy-"+t.Name(), "us-east-1")
	consumer := &countingConsumer{}
	engine := newFixtureEngine(gdb, runs.NewRepo(gdb), &countingFactory{}, &fakeFetcher{}, consumer, time.Second)

	err := engine.RunAll(context.Background(), "cron")
	require.Error(t, err)
	var healthyRuns int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("cloud_account_id = ? AND status = ?", healthy.ID, "success").Count(&healthyRuns).Error)
	require.EqualValues(t, 5, healthyRuns)
	var brokenRuns int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("cloud_account_id = ?", broken.ID).Count(&brokenRuns).Error)
	require.EqualValues(t, 1, brokenRuns)
}

// Aliases retain the existing fetcher/factory fixtures while exercising the real adapter.
type Clients = assetsync.Clients

func newFixtureEngine(db *gorm.DB, repo *runs.Repo, factory assetsync.ClientFactory, fetcher assetsync.Fetcher, consumer assetsync.CloudKeyConsumer, timeout time.Duration) *assetsync.Engine {
	registry, err := assetsync.NewRegistry(New(Options{ClientFactory: factory, Fetcher: fetcher, RequestTimeout: timeout, Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "111111111111", nil })}))
	if err != nil {
		panic(err)
	}
	return assetsync.NewProviderEngine(db, repo, registry, consumer, timeout, assetsync.DefaultAccountTimeout)
}

func TestAWSSecondRunRejectsChangedAccountBeforeAnyAssetChanges(t *testing.T) {
	gdb, row, _, consumer, _ := setupEngine(t, &fakeFetcher{})
	fixture := &nativeFixture{}
	identity := "111111111111"
	provider := New(Options{ClientFactory: assetsync.ClientFactoryFunc(fixture.clients), Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return identity, nil })})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, assetsync.DefaultAccountTimeout)
	require.NoError(t, engine.RunAccount(t.Context(), row.ID, "cron", nil))
	require.NoError(t, gdb.First(row, row.ID).Error)
	require.Equal(t, "111111111111", row.ExternalAccountID)
	// Preserve full native snapshots, including lifecycle timestamps and row IDs.
	before := nativeRows(t, gdb)
	var instance models.AWSInstance
	require.NoError(t, gdb.First(&instance).Error)
	require.Equal(t, "legacy-name", instance.Name)
	identity = "222222222222"
	fixture.calls = nil
	err = engine.RunAccount(t.Context(), row.ID, "cron", nil)
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errs.CodeAssetsIdentityMismatch, biz.Code)
	require.Empty(t, fixture.calls)
	require.Equal(t, before, nativeRows(t, gdb))
	var latest models.AssetsSyncRun
	require.NoError(t, gdb.Order("id DESC").First(&latest).Error)
	require.Equal(t, "planning", latest.ResourceType)
	require.Equal(t, "failed", latest.Status)
}

func TestAWSKeyRotationDuringRunStopsBeforeNextResourceEnumeration(t *testing.T) {
	gdb, row, _, consumer, _ := setupEngine(t, &fakeFetcher{})
	consumer.rotateAt = 3 // planning and first unit use account 111; next key belongs to 222.
	fixture := &nativeFixture{}
	identityCalls := 0
	provider := New(Options{ClientFactory: assetsync.ClientFactoryFunc(fixture.clients), Identity: identityFunc(func(_ context.Context, key *credentials.CloudKey, _ string) (string, error) {
		identityCalls++
		if key.AccessKeyID == "rotated-access" {
			return "222222222222", nil
		}
		return "111111111111", nil
	})})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, assetsync.DefaultAccountTimeout)
	err = engine.RunAccount(t.Context(), row.ID, "cron", nil)
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errs.CodeAssetsIdentityMismatch, biz.Code)
	require.Equal(t, 3, identityCalls)
	require.Equal(t, map[string]int{"DescribeInstances": 2}, fixture.calls)
	require.Len(t, consumer.keys, 3)
	for _, key := range consumer.keys {
		require.Empty(t, key.AccessKeyID)
		require.Empty(t, key.SecretAccessKey)
	}
	var found []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&found).Error)
	require.Len(t, found, 3)
	require.Equal(t, "success", found[1].Status)
	require.Equal(t, "network", found[2].ResourceType)
	require.EqualValues(t, errs.CodeAssetsIdentityMismatch, found[2].ErrorCode)
	var networkCount int64
	require.NoError(t, gdb.Model(&models.AWSVPC{}).Count(&networkCount).Error)
	require.Zero(t, networkCount)
}

func TestAWSUnitIdentityFailureStopsFurtherCalls(t *testing.T) {
	gdb, row, _, consumer, factory := setupEngine(t, &fakeFetcher{})
	identityCalls := 0
	provider := New(Options{ClientFactory: factory, Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) {
		identityCalls++
		if identityCalls > 1 {
			return "", apperr.New(errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden, "unsafe STS error")
		}
		return "111111111111", nil
	})})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, assetsync.DefaultAccountTimeout)
	err = engine.RunAccount(t.Context(), row.ID, "cron", nil)
	require.Error(t, err)
	require.Equal(t, 2, identityCalls, "STS failure stops calls to all later units")
	require.Zero(t, factory.calls)
	require.NotContains(t, err.Error(), "unsafe STS error")
}

func TestAWSAdapterEmptyFullSweepsDeleteAllFamiliesAndRestoreNaturalKeys(t *testing.T) {
	gdb, row, _, consumer, _ := setupEngine(t, &fakeFetcher{})
	fixture := &nativeFixture{}
	provider := New(Options{ClientFactory: assetsync.ClientFactoryFunc(fixture.clients), Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "111111111111", nil })})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, assetsync.DefaultAccountTimeout)
	require.NoError(t, engine.RunAccount(t.Context(), row.ID, "cron", nil))
	first := nativeRows(t, gdb)
	fixture.empty = true
	require.NoError(t, engine.RunAccount(t.Context(), row.ID, "cron", nil))
	for _, model := range []any{&models.AWSInstance{}, &models.AWSVPC{}, &models.AWSSubnet{}, &models.AWSDatabase{}, &models.AWSSecurityGroup{}} {
		var count int64
		require.NoError(t, gdb.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
	fixture.empty = false
	require.NoError(t, engine.RunAccount(t.Context(), row.ID, "cron", nil))
	for i, model := range []any{&models.AWSInstance{}, &models.AWSVPC{}, &models.AWSSubnet{}, &models.AWSDatabase{}, &models.AWSSecurityGroup{}} {
		var ids []uint64
		require.NoError(t, gdb.Model(model).Order("id").Pluck("id", &ids).Error)
		require.Equal(t, []uint64{1, 2}, ids, "natural keys retain original IDs for family %d; first snapshot: %v", i, first[i])
	}
	var networkRun models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ? AND items_softdeleted > 0", "network").First(&networkRun).Error)
	require.EqualValues(t, 4, networkRun.ItemsSoftDeleted)
}

func nativeRows(t *testing.T, db *gorm.DB) []any {
	t.Helper()
	instances, vpcs, subnets, databases, groups := []models.AWSInstance{}, []models.AWSVPC{}, []models.AWSSubnet{}, []models.AWSDatabase{}, []models.AWSSecurityGroup{}
	for _, rows := range []any{&instances, &vpcs, &subnets, &databases, &groups} {
		require.NoError(t, db.Unscoped().Order("id").Find(rows).Error)
	}
	return []any{instances, vpcs, subnets, databases, groups}
}
