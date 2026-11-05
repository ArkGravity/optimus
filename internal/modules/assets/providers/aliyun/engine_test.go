//go:build dbtest

package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	assetdatabase "github.com/ArkGravity/optimus/internal/modules/assets/database"
	assetfirewall "github.com/ArkGravity/optimus/internal/modules/assets/firewall"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type engineConsumer struct {
	keys     []*credentials.CloudKey
	purposes []string
	rotateAt int
}

func (c *engineConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	key := fixtureKey()
	if c.rotateAt > 0 && len(c.keys)+1 >= c.rotateAt {
		key.AccessKeyID = "rotated-id"
	}
	c.keys = append(c.keys, key)
	c.purposes = append(c.purposes, purpose)
	return key, nil
}

func TestEngineRealSDKFixturesPinScopePublishAndWipe(t *testing.T) {
	gdb, row := setupPersistence(t)
	row.EnabledRegions = models.StringArray{"cn-hangzhou"}
	require.NoError(t, gdb.Model(&row).Update("enabled_regions", row.EnabledRegions).Error)
	// A complete four-family sweep replaces old rows only in its own native unit.
	require.NoError(t, gdb.Create(&models.AliyunDatabase{CloudAccountID: row.ID, Region: "cn-hangzhou", DatabaseID: "existing-db", LastSeenAt: time.Now().Add(-time.Hour)}).Error)
	require.NoError(t, gdb.Create(&models.AliyunSecurityGroup{CloudAccountID: row.ID, Region: "cn-hangzhou", FirewallID: "existing-sg", LastSeenAt: time.Now().Add(-time.Hour)}).Error)
	provider := New(Options{Transport: allFamiliesFixture(t, nil)})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	consumer := &engineConsumer{}
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, time.Minute)
	require.NoError(t, engine.RunAccount(context.Background(), row.ID, "cron", nil))
	require.NoError(t, gdb.First(&row, row.ID).Error)
	require.Equal(t, "1234567890123456", row.ExternalAccountID)
	for _, item := range []struct {
		table string
		count int64
	}{{"aliyun_instances", 3}, {"aliyun_vpcs", 2}, {"aliyun_vswitches", 1}, {"aliyun_databases", 2}, {"aliyun_security_groups", 1}} {
		var count int64
		require.NoError(t, gdb.Table(item.table).Where("deleted_at IS NULL").Count(&count).Error)
		require.Equal(t, item.count, count, item.table)
	}
	var recorded []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&recorded).Error)
	require.Len(t, recorded, 5)
	for _, run := range recorded {
		require.Equal(t, "success", run.Status)
		require.NotContains(t, run.Error, "synthetic-secret")
	}
	require.Len(t, consumer.keys, 5)
	for i, key := range consumer.keys {
		require.Empty(t, key.AccessKeyID)
		require.Empty(t, key.SecretAccessKey)
		require.True(t, strings.HasPrefix(consumer.purposes[i], "system:assets.sync.cron.account-"))
	}
}

func TestEngineRotatedIdentityStopsBeforeResourcesAndPreservesRows(t *testing.T) {
	gdb, row := setupPersistence(t)
	require.NoError(t, gdb.Create(&models.AliyunInstance{CloudAccountID: row.ID, Region: "cn-hangzhou", InstanceID: "old", LastSeenAt: time.Now().Add(-time.Hour)}).Error)
	identityCalls := 0
	resourceCalls := 0
	provider := New(Options{Transport: fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-acs-action") != "GetCallerIdentity" {
			resourceCalls++
			http.Error(w, "must not enumerate", 500)
			return
		}
		identityCalls++
		id := "1234567890123456"
		if strings.Contains(r.Header.Get("Authorization"), "rotated-id") {
			id = "9999999999999999"
		}
		_, _ = w.Write([]byte(`{"AccountId":"` + id + `"}`))
	})})
	registry, err := assetsync.NewRegistry(provider)
	require.NoError(t, err)
	consumer := &engineConsumer{rotateAt: 2}
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, time.Minute)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "manual", nil))
	require.Equal(t, 2, identityCalls)
	require.Zero(t, resourceCalls)
	var stored []models.AliyunInstance
	require.NoError(t, gdb.Find(&stored).Error)
	require.Len(t, stored, 1)
	require.Equal(t, "old", stored[0].InstanceID)
	var recorded []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&recorded).Error)
	require.Len(t, recorded, 2)
	require.EqualValues(t, 43209, recorded[1].ErrorCode)
	for _, key := range consumer.keys {
		require.Empty(t, key.AccessKeyID)
		require.Empty(t, key.SecretAccessKey)
	}
}

func TestNetworkFixturePublicationFailureAndStaleScopePreservePriorRows(t *testing.T) {
	gdb, row := setupPersistence(t)
	row.ExternalAccountID = "1234567890123456"
	require.NoError(t, gdb.Model(&row).Update("external_account_id", row.ExternalAccountID).Error)
	unit := fixtureUnit("network")
	unit.Scope = account.CaptureScope(row)
	provider := New(Options{Transport: networkFixture(t, nil)})
	snapshot, err := provider.Fetch(context.Background(), unit, fixtureKey())
	require.NoError(t, err)
	var stats assetsync.Stats
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		var err error
		stats, err = snapshot.Publish(context.Background(), tx)
		return err
	}))
	require.EqualValues(t, 3, stats.Seen)
	var original models.AliyunVPC
	require.NoError(t, gdb.Where("network_id = ?", "vpc-a").First(&original).Error)
	failing := New(Options{Transport: networkFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("x-acs-action") != "DescribeVSwitches" {
			return false
		}
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
		return true
	})})
	partial, err := failing.Fetch(context.Background(), unit, fixtureKey())
	require.Error(t, err)
	require.False(t, partial.Complete)
	require.Nil(t, partial.Publish)
	var after models.AliyunVPC
	require.NoError(t, gdb.First(&after, original.ID).Error)
	require.Equal(t, original, after)
	// The deferred closure rechecks the exact captured revision at publication.
	snapshot, err = provider.Fetch(context.Background(), unit, fixtureKey())
	require.NoError(t, err)
	require.NoError(t, gdb.Model(&row).Update("scope_revision", row.ScopeRevision+1).Error)
	require.ErrorIs(t, gdb.Transaction(func(tx *gorm.DB) error { _, err := snapshot.Publish(context.Background(), tx); return err }), account.ErrScopeIneligible)
	require.NoError(t, gdb.First(&after, original.ID).Error)
	require.Equal(t, original, after)
}

func TestCompleteEmptyNetworkPrunesOnlyCapturedRegionAndFamily(t *testing.T) {
	gdb, row := setupPersistence(t)
	row.ExternalAccountID = "1234567890123456"
	require.NoError(t, gdb.Model(&row).Update("external_account_id", row.ExternalAccountID).Error)
	old := time.Now().Add(-time.Hour)
	require.NoError(t, gdb.Create(&models.AliyunVPC{CloudAccountID: row.ID, Region: "cn-hangzhou", NetworkID: "old-vpc", LastSeenAt: old}).Error)
	require.NoError(t, gdb.Create(&models.AliyunVSwitch{CloudAccountID: row.ID, Region: "cn-hangzhou", SubnetID: "old-subnet", LastSeenAt: old}).Error)
	require.NoError(t, gdb.Create(&models.AliyunVPC{CloudAccountID: row.ID, Region: "cn-shanghai", NetworkID: "other-region", LastSeenAt: old}).Error)
	require.NoError(t, gdb.Create(&models.AliyunInstance{CloudAccountID: row.ID, Region: "cn-hangzhou", InstanceID: "other-family", LastSeenAt: old}).Error)
	provider := New(Options{Transport: networkFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Header.Get("x-acs-action") {
		case "DescribeVpcs":
			_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":50,"TotalCount":0,"Vpcs":{"Vpc":[]}}`))
			return true
		case "DescribeVSwitches":
			_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":50,"TotalCount":0,"VSwitches":{"VSwitch":[]}}`))
			return true
		}
		return false
	})})
	unit := fixtureUnit("network")
	unit.Scope = account.CaptureScope(row)
	snapshot, err := provider.Fetch(context.Background(), unit, fixtureKey())
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		stats, err := snapshot.Publish(context.Background(), tx)
		require.Zero(t, stats.Seen)
		require.EqualValues(t, 2, stats.SoftDeleted)
		return err
	}))
	var vpcs []models.AliyunVPC
	require.NoError(t, gdb.Find(&vpcs).Error)
	require.Len(t, vpcs, 1)
	require.Equal(t, "other-region", vpcs[0].NetworkID)
	var instances []models.AliyunInstance
	require.NoError(t, gdb.Find(&instances).Error)
	require.Len(t, instances, 1)
}

func allFamiliesFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	instances := instanceFixture(t, override)
	network := networkFixture(t, override)
	databases := databaseFixture(t, override)
	firewalls := firewallFixture(t, override)
	return transportFunc(func(r *http.Request) (*http.Response, error) {
		action := ""
		for name, values := range r.Header {
			if strings.EqualFold(name, "x-acs-action") && len(values) > 0 {
				action = values[0]
			}
		}
		switch {
		case strings.HasPrefix(action, "DescribeDB"):
			return databases.RoundTrip(r)
		case strings.HasPrefix(action, "DescribeSecurityGroup"):
			return firewalls.RoundTrip(r)
		case action == "DescribeVpcs" || action == "DescribeVSwitches":
			return network.RoundTrip(r)
		default:
			return instances.RoundTrip(r)
		}
	})
}

func TestEngineDetailFailurePreservesPreviousSnapshotAndIndependentUnitsContinue(t *testing.T) {
	for _, kind := range []string{"database", "security_group"} {
		t.Run(kind, func(t *testing.T) {
			gdb, row := setupPersistence(t)
			row.EnabledRegions = models.StringArray{"cn-hangzhou"}
			require.NoError(t, gdb.Model(&row).Update("enabled_regions", row.EnabledRegions).Error)
			old := time.Now().Add(-time.Hour)
			originalDB := models.AliyunDatabase{CloudAccountID: row.ID, Region: "cn-hangzhou", DatabaseID: "old-db", Name: "previous db", LastSeenAt: old}
			require.NoError(t, gdb.Create(&originalDB).Error)
			originalSG := models.AliyunSecurityGroup{CloudAccountID: row.ID, Region: "cn-hangzhou", FirewallID: "old-sg", InboundRules: jsonValue([]map[string]string{{"direction": "inbound", "target": "previous-rules"}}), LastSeenAt: old}
			require.NoError(t, gdb.Create(&originalSG).Error)
			require.NoError(t, gdb.First(&originalDB, originalDB.ID).Error)
			require.NoError(t, gdb.First(&originalSG, originalSG.ID).Error)
			provider := New(Options{Transport: allFamiliesFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				_ = r.ParseForm()
				action := r.Header.Get("x-acs-action")
				if (kind == "database" && action == "DescribeDBInstanceNetInfo") || (kind == "security_group" && action == "DescribeSecurityGroupAttribute" && r.FormValue("NextToken") != "") {
					w.WriteHeader(403)
					_, _ = w.Write([]byte(`{"Code":"Forbidden.RAM","Message":"synthetic-secret signed token body"}`))
					return true
				}
				return false
			})})
			registry, err := assetsync.NewRegistry(provider)
			require.NoError(t, err)
			consumer := &engineConsumer{}
			engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, time.Minute)
			require.Error(t, engine.RunAccount(context.Background(), row.ID, "manual", nil))
			var recorded []models.AssetsSyncRun
			require.NoError(t, gdb.Order("id").Find(&recorded).Error)
			require.Len(t, recorded, 5)
			for _, run := range recorded {
				if run.ResourceType == kind {
					require.Equal(t, "failed", run.Status)
					require.EqualValues(t, 43202, run.ErrorCode)
					require.Equal(t, "cloud API access denied", run.Error)
				} else {
					require.Equal(t, "success", run.Status)
				}
				require.NotContains(t, run.Error, "synthetic")
			}
			if kind == "database" {
				var after models.AliyunDatabase
				require.NoError(t, gdb.First(&after, originalDB.ID).Error)
				require.Equal(t, originalDB, after)
			} else {
				var after models.AliyunSecurityGroup
				require.NoError(t, gdb.First(&after, originalSG.ID).Error)
				require.Equal(t, originalSG, after)
			}
		})
	}
}

func TestDatabaseAndFirewallSnapshotsPruneOnlyOwnUnitAndReviveStableIDs(t *testing.T) {
	for _, kind := range []string{"database", "security_group"} {
		t.Run(kind, func(t *testing.T) {
			gdb, row := setupPersistence(t)
			row.ExternalAccountID = "1234567890123456"
			require.NoError(t, gdb.Model(&row).Update("external_account_id", row.ExternalAccountID).Error)
			old := time.Now().Add(-time.Hour)
			require.NoError(t, gdb.Create(&models.AliyunDatabase{CloudAccountID: row.ID, DatabaseID: "other-db", Region: "cn-shanghai", LastSeenAt: old}).Error)
			require.NoError(t, gdb.Create(&models.AliyunSecurityGroup{CloudAccountID: row.ID, FirewallID: "other-sg", Region: "cn-shanghai", LastSeenAt: old}).Error)
			require.NoError(t, gdb.Create(&models.AliyunInstance{CloudAccountID: row.ID, InstanceID: "other-instance", Region: "cn-hangzhou", LastSeenAt: old}).Error)
			unit := fixtureUnit(kind)
			unit.Scope = account.CaptureScope(row)
			populated := New(Options{Transport: allFamiliesFixture(t, nil)})
			publish := func(snapshot assetsync.Snapshot) assetsync.Stats {
				var stats assetsync.Stats
				require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
					var err error
					stats, err = snapshot.Publish(context.Background(), tx)
					return err
				}))
				return stats
			}
			for _, resourceType := range []string{"database", "security_group"} {
				current := unit
				current.ResourceType = resourceType
				snapshot, err := populated.Fetch(context.Background(), current, fixtureKey())
				require.NoError(t, err)
				publish(snapshot)
			}
			var originalDB models.AliyunDatabase
			var originalSG models.AliyunSecurityGroup
			require.NoError(t, gdb.Where("database_id = ?", "rm-a").First(&originalDB).Error)
			require.NoError(t, gdb.Where("firewall_id = ?", "sg-a").First(&originalSG).Error)
			databaseDetail, err := assetdatabase.NewRepo(gdb).FindByKey(context.Background(), fmt.Sprintf("aliyun:database:%d", originalDB.ID), false)
			require.NoError(t, err)
			rawDatabase, err := json.Marshal(databaseDetail)
			require.NoError(t, err)
			require.Contains(t, string(rawDatabase), `"memory_mb":8589934592`)
			require.Contains(t, string(rawDatabase), `"cpu_count":8`)
			require.NotContains(t, string(rawDatabase), "discard-me")
			firewallDetail, err := assetfirewall.NewRepo(gdb).FindByKey(context.Background(), fmt.Sprintf("aliyun:firewall:%d", originalSG.ID), false)
			require.NoError(t, err)
			require.Equal(t, "owner@example.invalid", firewallDetail.NativeDetails.AliyunOutboundRuleFacts[0].DestinationGroupOwnerAccount)
			require.Equal(t, "sg-other", firewallDetail.NativeDetails.AliyunOutboundRuleFacts[0].DestinationGroupID)

			empty := New(Options{Transport: allFamiliesFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch r.Header.Get("x-acs-action") {
				case "DescribeDBInstances":
					_, _ = w.Write([]byte(`{"PageNumber":1,"PageRecordCount":0,"TotalRecordCount":0,"Items":{"DBInstance":[]}}`))
					return true
				case "DescribeSecurityGroups":
					_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":100,"TotalCount":0,"SecurityGroups":{"SecurityGroup":[]}}`))
					return true
				}
				return false
			})})
			snapshot, err := empty.Fetch(context.Background(), unit, fixtureKey())
			require.NoError(t, err)
			stats := publish(snapshot)
			require.Zero(t, stats.Seen)
			expectedDeleted := int64(1)
			if kind == "database" {
				expectedDeleted = 2
			}
			require.Equal(t, expectedDeleted, stats.SoftDeleted)
			for _, check := range []struct {
				table string
				want  int64
			}{{"aliyun_databases", 3}, {"aliyun_security_groups", 2}, {"aliyun_instances", 1}} {
				expected := check.want
				if kind == "database" && check.table == "aliyun_databases" {
					expected = 1
				}
				if kind == "security_group" && check.table == "aliyun_security_groups" {
					expected = 1
				}
				var count int64
				require.NoError(t, gdb.Table(check.table).Where("deleted_at IS NULL").Count(&count).Error)
				require.Equal(t, expected, count, check.table)
			}
			snapshot, err = populated.Fetch(context.Background(), unit, fixtureKey())
			require.NoError(t, err)
			publish(snapshot)
			var revivedDB models.AliyunDatabase
			var revivedSG models.AliyunSecurityGroup
			require.NoError(t, gdb.Where("database_id = ?", "rm-a").First(&revivedDB).Error)
			require.NoError(t, gdb.Where("firewall_id = ?", "sg-a").First(&revivedSG).Error)
			require.Equal(t, originalDB.ID, revivedDB.ID)
			require.Equal(t, originalSG.ID, revivedSG.ID)
			// Deferred publication must honor a revision change after SDK collection.
			snapshot, err = populated.Fetch(context.Background(), unit, fixtureKey())
			require.NoError(t, err)
			require.NoError(t, gdb.Model(&row).Update("scope_revision", row.ScopeRevision+1).Error)
			require.ErrorIs(t, gdb.Transaction(func(tx *gorm.DB) error { _, err := snapshot.Publish(context.Background(), tx); return err }), account.ErrScopeIneligible)
		})
	}
}
