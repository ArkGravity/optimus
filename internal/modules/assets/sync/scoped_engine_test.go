//go:build dbtest

package sync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/db"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

type scopedConsumer struct {
	provider    string
	purposes    []string
	keys        []*credentials.CloudKey
	jsonBuffers [][]byte
}

func (c *scopedConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	key := &credentials.CloudKey{Provider: c.provider, AuthType: "access_key", AccessKeyID: "selected-access", SecretAccessKey: "selected-secret"}
	if c.provider == "gcp" {
		key.AuthType, key.AccessKeyID, key.SecretAccessKey = "service_account_json", "", ""
		key.ServiceAccountJSON = []byte(`{"private_key":"selected-private-material"}`)
		c.jsonBuffers = append(c.jsonBuffers, key.ServiceAccountJSON)
	}
	c.purposes = append(c.purposes, purpose)
	c.keys = append(c.keys, key)
	return key, nil
}

func regionUnit(resourceType, region string) Unit {
	return Unit{Key: resourceType + "/" + region, ResourceType: resourceType, LocationType: "region", Location: region, Region: region, Budget: time.Second}
}

func setupScopedEngine(t *testing.T, provider *fixtureProvider) (*gorm.DB, *models.CloudAccount, *Engine, *scopedConsumer) {
	t.Helper()
	gdb, row := setupScopedAccount(t)
	updates := map[string]any{"provider": provider.name}
	if provider.name == "gcp" {
		updates["project_id"] = "explicit-target"
	}
	require.NoError(t, gdb.Model(row).Updates(updates).Error)
	require.NoError(t, gdb.First(row, row.ID).Error)
	consumer := &scopedConsumer{provider: provider.name}
	registry, err := NewRegistry(provider)
	require.NoError(t, err)
	engine := NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, DefaultAccountTimeout)
	return gdb, row, engine, consumer
}

func TestScopedPlanningFailureRecordsTerminalRunAndNoResourceWrites(t *testing.T) {
	p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
		return Plan{}, errors.New("secret raw STS or zone-catalog failure")
	}, fetch: func(context.Context, Unit, *credentials.CloudKey) (Snapshot, error) {
		t.Fatal("fetch after failed planning")
		return Snapshot{}, nil
	}}
	gdb, row, engine, consumer := setupScopedEngine(t, p)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
	var found []models.AssetsSyncRun
	require.NoError(t, gdb.Find(&found).Error)
	require.Len(t, found, 1)
	require.Equal(t, "planning", found[0].ResourceType)
	require.Equal(t, "failed", found[0].Status)
	require.Equal(t, "aliyun", found[0].Provider)
	require.Equal(t, "global", found[0].LocationType)
	require.NotNil(t, found[0].FinishedAt)
	require.NotContains(t, found[0].Error, "secret")
	require.Len(t, consumer.keys, 1)
	require.Empty(t, consumer.keys[0].SecretAccessKey)
	var count int64
	require.NoError(t, gdb.Model(&models.AliyunInstance{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestScopedIdentityFirstPinRefreshesUnitsThenMismatchStopsFetch(t *testing.T) {
	identity := "verified-account-1"
	fetches := 0
	p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
		return Plan{ExternalAccountID: identity, Units: []Unit{regionUnit("instance", "us-east-1")}}, nil
	}, fetch: func(_ context.Context, unit Unit, key *credentials.CloudKey) (Snapshot, error) {
		fetches++
		require.Equal(t, identity, unit.Scope.ExternalAccountID)
		require.Equal(t, "aliyun", key.Provider)
		return Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (Stats, error) {
			err := tx.WithContext(ctx).Create(&models.AliyunInstance{CloudAccountID: unit.Scope.AccountID, Region: unit.Region, InstanceID: "new", LastSeenAt: time.Now()}).Error
			return Stats{Seen: 1}, err
		}}, nil
	}}
	gdb, row, engine, consumer := setupScopedEngine(t, p)
	require.NoError(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
	require.Equal(t, 1, fetches)
	require.NoError(t, gdb.First(row, row.ID).Error)
	require.Equal(t, identity, row.ExternalAccountID)
	require.Len(t, consumer.keys, 2)
	require.Contains(t, consumer.purposes[0], ".aliyun.planning")
	require.Contains(t, consumer.purposes[1], ".aliyun.instance/us-east-1")
	identity = "different-account"
	err := engine.RunAccount(context.Background(), row.ID, "cron", nil)
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errs.CodeAssetsIdentityMismatch, biz.Code)
	require.Equal(t, 1, fetches)
	var count int64
	require.NoError(t, gdb.Model(&models.AliyunInstance{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestScopedIndependentUnitsContinueAndFailedSnapshotsNeverPublish(t *testing.T) {
	for _, failure := range []string{"fetch", "incomplete", "missing publisher", "publish rollback"} {
		t.Run(failure, func(t *testing.T) {
			published := 0
			failedPublishCalls := 0
			p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
				return Plan{ExternalAccountID: "verified", Units: []Unit{regionUnit("instance", "us-east-1"), regionUnit("database", "us-east-1")}}, nil
			}, fetch: func(_ context.Context, unit Unit, _ *credentials.CloudKey) (Snapshot, error) {
				if unit.ResourceType == "database" {
					return Snapshot{Complete: true, Publish: func(context.Context, *gorm.DB) (Stats, error) { published++; return Stats{}, nil }}, nil
				}
				publish := func(ctx context.Context, tx *gorm.DB) (Stats, error) {
					failedPublishCalls++
					err := tx.WithContext(ctx).Create(&models.AliyunInstance{CloudAccountID: unit.Scope.AccountID, Region: unit.Region, InstanceID: "must-rollback", LastSeenAt: time.Now()}).Error
					if err != nil {
						return Stats{}, err
					}
					return Stats{Seen: math.MaxInt64, SoftDeleted: math.MaxInt64}, errors.New("raw secret DB failure")
				}
				switch failure {
				case "fetch":
					return Snapshot{Complete: true, Publish: publish}, errors.New("secret provider failure")
				case "incomplete":
					return Snapshot{Publish: publish}, nil
				case "missing publisher":
					return Snapshot{Complete: true}, nil
				default:
					return Snapshot{Complete: true, Publish: publish}, nil
				}
			}}
			gdb, row, engine, consumer := setupScopedEngine(t, p)
			require.NoError(t, gdb.Create(&models.AliyunInstance{CloudAccountID: row.ID, Region: "us-east-1", InstanceID: "previous", LastSeenAt: time.Now().Add(-time.Hour)}).Error)
			err := engine.RunAccount(context.Background(), row.ID, "cron", nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
			require.Equal(t, 1, published)
			if failure == "publish rollback" {
				require.Equal(t, 1, failedPublishCalls)
			} else {
				require.Zero(t, failedPublishCalls)
			}
			require.Len(t, consumer.keys, 3)
			var found []models.AssetsSyncRun
			require.NoError(t, gdb.Order("id").Find(&found).Error)
			require.Len(t, found, 3)
			require.Equal(t, "success", found[0].Status)
			require.Equal(t, "failed", found[1].Status)
			require.Zero(t, found[1].ItemsSeen)
			require.Zero(t, found[1].ItemsSoftDeleted)
			require.Equal(t, "success", found[2].Status)
			var count int64
			require.NoError(t, gdb.Model(&models.AliyunInstance{}).Count(&count).Error)
			require.EqualValues(t, 1, count, "a failed unit must preserve the previous complete snapshot")
		})
	}
}

func TestScopedPlansAreProviderOwnedAndEveryCredentialIsWiped(t *testing.T) {
	for _, name := range []string{"aws", "aliyun", "gcp"} {
		t.Run(name, func(t *testing.T) {
			var units []Unit
			regions := []string{"us-east-1", "us-west-2"}
			if name == "gcp" {
				regions = []string{"us-central1", "europe-west1"}
				for _, family := range []string{"network", "firewall"} {
					units = append(units, Unit{Key: family + "/global", ResourceType: family, LocationType: "global", Location: "global", Budget: time.Second})
				}
				for i, region := range regions {
					units = append(units, regionUnit("subnet", region), regionUnit("database", region))
					// Deliberately different zone counts: the engine must use the plan.
					for z := 0; z <= i+1; z++ {
						zone := fmt.Sprintf("%s-%c", region, 'a'+z)
						units = append(units, Unit{Key: "instance/" + zone, ResourceType: "instance", LocationType: "zone", Location: zone, Region: region, Zone: zone, Budget: time.Second})
					}
				}
			} else {
				for _, region := range regions {
					for _, family := range []string{"instance", "network", "database", "security_group"} {
						units = append(units, regionUnit(family, region))
					}
				}
			}
			fetched := 0
			p := &fixtureProvider{name: name, plan: func(_ context.Context, row models.CloudAccount, key *credentials.CloudKey) (Plan, error) {
				require.Equal(t, name, key.Provider)
				if name == "gcp" {
					require.Equal(t, "explicit-target", row.ProjectID)
					require.NotEmpty(t, key.ServiceAccountJSON)
				}
				return Plan{ExternalAccountID: "verified", Units: units}, nil
			}, fetch: func(ctx context.Context, unit Unit, key *credentials.CloudKey) (Snapshot, error) {
				fetched++
				require.Equal(t, name, key.Provider)
				require.Positive(t, unit.Scope.Revision)
				_, bounded := ctx.Deadline()
				require.True(t, bounded)
				return Snapshot{Complete: true, Publish: func(context.Context, *gorm.DB) (Stats, error) {
					return Stats{Seen: math.MaxInt64, SoftDeleted: -1}, nil
				}}, nil
			}}
			gdb, row, engine, consumer := setupScopedEngine(t, p)
			require.NoError(t, gdb.Model(row).Update("enabled_regions", models.StringArray(regions)).Error)
			require.NoError(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
			require.Equal(t, len(units), fetched)
			if name == "gcp" {
				require.Equal(t, 11, fetched)
			} else {
				require.Equal(t, 8, fetched)
			}
			require.Len(t, consumer.keys, len(units)+1)
			for i, key := range consumer.keys {
				require.Empty(t, key.AccessKeyID)
				require.Empty(t, key.SecretAccessKey)
				require.Nil(t, key.ServiceAccountJSON)
				require.True(t, strings.HasPrefix(consumer.purposes[i], "system:assets.sync.cron.account-"))
			}
			for _, bytes := range consumer.jsonBuffers {
				require.Equal(t, make([]byte, len(bytes)), bytes)
			}
			var found []models.AssetsSyncRun
			require.NoError(t, gdb.Where("resource_type <> ?", "planning").Find(&found).Error)
			for _, run := range found {
				require.Equal(t, int32(math.MaxInt32), run.ItemsSeen)
				require.Zero(t, run.ItemsSoftDeleted)
				require.Equal(t, name, run.Provider)
			}
		})
	}
}

func TestScopedFetchCannotPublishAfterAccountScopeChanges(t *testing.T) {
	for _, change := range []string{"revision", "region removed then restored", "disabled", "deleted"} {
		t.Run(change, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			published := false
			p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
				return Plan{ExternalAccountID: "verified", Units: []Unit{regionUnit("instance", "us-east-1")}}, nil
			}, fetch: func(context.Context, Unit, *credentials.CloudKey) (Snapshot, error) {
				close(started)
				<-release
				return Snapshot{Complete: true, Publish: func(context.Context, *gorm.DB) (Stats, error) { published = true; return Stats{}, nil }}, nil
			}}
			gdb, row, engine, _ := setupScopedEngine(t, p)
			done := make(chan error, 1)
			go func() { done <- engine.RunAccount(context.Background(), row.ID, "cron", nil) }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("fetch did not start")
			}
			switch change {
			case "revision":
				require.NoError(t, gdb.Model(row).Update("scope_revision", gorm.Expr("scope_revision + 1")).Error)
			case "region removed then restored":
				require.NoError(t, gdb.Model(row).Updates(map[string]any{"enabled_regions": models.StringArray{"us-west-2"}, "scope_revision": gorm.Expr("scope_revision + 1")}).Error)
				require.NoError(t, gdb.Model(row).Updates(map[string]any{"enabled_regions": models.StringArray{"us-east-1"}, "scope_revision": gorm.Expr("scope_revision + 1")}).Error)
			case "disabled":
				require.NoError(t, gdb.Model(row).Update("enabled", false).Error)
			case "deleted":
				require.NoError(t, gdb.Delete(row).Error)
			}
			close(release)
			require.Error(t, <-done)
			require.False(t, published)
			var run models.AssetsSyncRun
			require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
			require.Equal(t, "failed", run.Status)
			require.NotNil(t, run.FinishedAt)
		})
	}
}

func TestScopedCancellationFinalizesRunAndReleasesLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
		return Plan{ExternalAccountID: "verified", Units: []Unit{regionUnit("instance", "us-east-1"), regionUnit("database", "us-east-1")}}, nil
	}, fetch: func(ctx context.Context, _ Unit, _ *credentials.CloudKey) (Snapshot, error) {
		cancel()
		<-ctx.Done()
		return Snapshot{}, ctx.Err()
	}}
	gdb, row, engine, consumer := setupScopedEngine(t, p)
	require.ErrorIs(t, engine.RunAccount(ctx, row.ID, "cron", nil), context.Canceled)
	require.Len(t, consumer.keys, 2)
	require.True(t, engine.TryLock(row.ID))
	engine.Unlock(row.ID)
	var found []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&found).Error)
	require.Len(t, found, 2)
	for _, run := range found {
		require.NotEqual(t, "running", run.Status)
		require.NotNil(t, run.FinishedAt)
	}
}

func TestScopedUnknownProviderNeverFallsBackToAWS(t *testing.T) {
	p := &fixtureProvider{name: "aws"}
	gdb, row, _, consumer := setupScopedEngine(t, p)
	engine := NewProviderEngine(gdb, runs.NewRepo(gdb), nil, consumer, time.Second, DefaultAccountTimeout)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
	require.Empty(t, consumer.keys)
	var run models.AssetsSyncRun
	require.NoError(t, gdb.First(&run).Error)
	require.Equal(t, "planning", run.ResourceType)
	require.Equal(t, "failed", run.Status)
}

func TestScopedUnitIdentityFailureStopsFurtherCalls(t *testing.T) {
	fetches := 0
	p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
		return Plan{ExternalAccountID: "verified", Units: []Unit{regionUnit("instance", "us-east-1"), regionUnit("database", "us-east-1")}}, nil
	}, fetch: func(context.Context, Unit, *credentials.CloudKey) (Snapshot, error) {
		fetches++
		return Snapshot{}, apperr.New(errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch, "unsafe identity failure")
	}}
	_, row, engine, consumer := setupScopedEngine(t, p)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
	require.Equal(t, 1, fetches)
	require.Len(t, consumer.keys, 2)
}

func TestPublishSnapshotReturnsZeroStatsAfterRollback(t *testing.T) {
	gdb, row := setupScopedAccount(t)
	unit := regionUnit("instance", "us-east-1")
	unit.Scope = account.CaptureScope(*row)
	stats, err := PublishSnapshot(context.Background(), gdb, unit, Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (Stats, error) {
		err := tx.WithContext(ctx).Create(&models.AWSInstance{CloudAccountID: row.ID, Region: unit.Region, InstanceID: "rollback", LastSeenAt: time.Now()}).Error
		require.NoError(t, err)
		return Stats{Seen: 99, SoftDeleted: 8}, errors.New("rollback")
	}})
	require.Error(t, err)
	require.Equal(t, Stats{}, stats)
	var count int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Count(&count).Error)
	require.Zero(t, count)
}

func setupScopedAccount(t *testing.T) (*gorm.DB, *models.CloudAccount) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "scoped-key-"+t.Name())
	row := dbtest.SeedCloudAccount(t, gdb, key.ID, "scoped-account-"+t.Name(), "us-east-1")
	return gdb, row
}

func TestScopedPlanBudgetConsumesPlanningElapsed(t *testing.T) {
	var planningEntered time.Time
	fetched := false
	p := &fixtureProvider{name: "aliyun", plan: func(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
		planningEntered = time.Now()
		return Plan{ExternalAccountID: "verified", Units: []Unit{regionUnit("instance", "us-east-1")}}, nil
	}, fetch: func(ctx context.Context, _ Unit, _ *credentials.CloudKey) (Snapshot, error) {
		fetched = true
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		// Planning started after the account budget began. Its elapsed time must
		// consume that budget, rather than restarting the sum when fetching begins.
		require.True(t, deadline.Before(planningEntered.Add(time.Second)))
		return Snapshot{Complete: true, Publish: func(context.Context, *gorm.DB) (Stats, error) { return Stats{}, nil }}, nil
	}}
	_, row, engine, _ := setupScopedEngine(t, p)
	require.NoError(t, engine.RunAccount(t.Context(), row.ID, "cron", nil))
	require.True(t, fetched)
}
