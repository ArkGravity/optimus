//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedAllProviderFamilies(t *testing.T, db *gorm.DB, id uint64, region, suffix string) []any {
	t.Helper()
	now := time.Now()
	rows := []any{
		&models.AWSInstance{CloudAccountID: id, Region: region, InstanceID: suffix, LastSeenAt: now},
		&models.AWSVPC{CloudAccountID: id, Region: region, VPCID: suffix, LastSeenAt: now},
		&models.AWSSubnet{CloudAccountID: id, Region: region, SubnetID: suffix, LastSeenAt: now},
		&models.AWSDatabase{CloudAccountID: id, Region: region, DBInstanceID: suffix, LastSeenAt: now},
		&models.AWSSecurityGroup{CloudAccountID: id, Region: region, GroupID: suffix, LastSeenAt: now},
		&models.AliyunInstance{CloudAccountID: id, Region: region, InstanceID: suffix, LastSeenAt: now},
		&models.AliyunVPC{CloudAccountID: id, Region: region, NetworkID: suffix, LastSeenAt: now},
		&models.AliyunVSwitch{CloudAccountID: id, Region: region, SubnetID: suffix, LastSeenAt: now},
		&models.AliyunDatabase{CloudAccountID: id, Region: region, DatabaseID: suffix, LastSeenAt: now},
		&models.AliyunSecurityGroup{CloudAccountID: id, Region: region, FirewallID: suffix, LastSeenAt: now},
		&models.GCPInstance{CloudAccountID: id, Region: region, InstanceID: suffix, LastSeenAt: now},
		&models.GCPNetwork{CloudAccountID: id, Region: "", NetworkID: suffix, LastSeenAt: now},
		&models.GCPSubnet{CloudAccountID: id, Region: region, SubnetID: suffix, LastSeenAt: now},
		&models.GCPDatabase{CloudAccountID: id, Region: region, DatabaseID: suffix, LastSeenAt: now},
		&models.GCPFirewall{CloudAccountID: id, Region: "", FirewallID: suffix, LastSeenAt: now},
	}
	for _, row := range rows {
		require.NoError(t, db.Create(row).Error)
	}
	return rows
}
func TestRepo_CascadeAllNativeFamilies(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, db, "cascade-all")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "all", "us-east-1", "us-west-2")
	other := dbtest.SeedCloudAccount(t, db, key.ID, "other", "us-east-1")
	rows := seedAllProviderFamilies(t, db, row.ID, "us-east-1", "removed")
	seedAllProviderFamilies(t, db, row.ID, "us-west-2", "kept")
	seedAllProviderFamilies(t, db, other.ID, "us-east-1", "other")
	require.NoError(t, repo.Transaction(ctx, func(tx *gorm.DB) error {
		count, err := repo.CascadeSoftDeleteResources(ctx, tx, row.ID, []string{"us-east-1"})
		require.EqualValues(t, 13, count)
		return err
	}))
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", row.ID).Count(&count).Error)
		want := int64(1)
		switch model.(type) {
		case *models.GCPNetwork, *models.GCPFirewall:
			want = 2
		}
		require.Equal(t, want, count)
	}
	require.NoError(t, repo.Transaction(ctx, func(tx *gorm.DB) error {
		count, err := repo.CascadeSoftDeleteResources(ctx, tx, row.ID, nil)
		require.EqualValues(t, 17, count)
		return err
	}))
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", row.ID).Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", other.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}
func TestRepo_CascadeNativeFailureRollsBack(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, db, "cascade-rollback")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "rollback-all", "us-east-1")
	rows := seedAllProviderFamilies(t, db, row.ID, "us-east-1", "rollback")
	// A real failure late in the static cascade must restore every earlier family.
	require.NoError(t, db.Exec(`CREATE FUNCTION reject_firewall_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test cleanup rejected'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_firewall_cleanup BEFORE UPDATE ON gcp_firewalls FOR EACH ROW EXECUTE FUNCTION reject_firewall_cleanup()`).Error)
	require.Error(t, repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := repo.CascadeSoftDeleteResources(ctx, tx, row.ID, nil); return err }))
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", row.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}

func TestService_CascadeNativeRegionAndAccountLifecycle(t *testing.T) {
	svc, repo, _, key := setupSvc(t)
	db := repo.DB()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 7, "", "", CreateRequest{Name: "native-lifecycle", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1", "us-west-2"}})
	require.NoError(t, err)
	rows := seedAllProviderFamilies(t, db, detail.ID, "us-east-1", "removed")
	seedAllProviderFamilies(t, db, detail.ID, "us-west-2", "remaining")
	run := models.AssetsSyncRun{CloudAccountID: detail.ID, Region: "us-east-1", Provider: "aws", LocationType: "region", Location: "us-east-1", ScopeRevision: 1, ResourceType: "instance", StartedAt: time.Now(), Status: "success", Trigger: "manual"}
	require.NoError(t, db.Create(&run).Error)
	detail, err = svc.Update(ctx, 7, "", "", detail.ID, UpdateRequest{EnabledRegions: []string{"us-west-2"}})
	require.NoError(t, err)
	require.EqualValues(t, 2, detail.ScopeRevision)
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", detail.ID).Count(&count).Error)
		want := int64(1)
		switch model.(type) {
		case *models.GCPNetwork, *models.GCPFirewall:
			want = 2
		}
		require.Equal(t, want, count)
	}
	cascaded, err := svc.Delete(ctx, 7, "", "", detail.ID)
	require.NoError(t, err)
	require.EqualValues(t, 17, cascaded)
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", detail.ID).Count(&count).Error)
		require.Zero(t, count)
	}
	var historical models.CloudAccount
	require.NoError(t, db.Unscoped().First(&historical, detail.ID).Error)
	require.True(t, historical.DeletedAt.Valid)
	require.Equal(t, key.ID, historical.CloudKeyID)
	require.NoError(t, db.First(&run, run.ID).Error)
}

func TestRepo_CascadeUnscopedKeepsHistory(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, db, "unscoped-cleanup")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "unscoped", "us-east-1")
	rows := seedAllProviderFamilies(t, db, row.ID, "us-east-1", "historical")
	require.NoError(t, repo.Transaction(ctx, func(tx *gorm.DB) error {
		_, err := repo.CascadeSoftDeleteResources(ctx, tx.Session(&gorm.Session{PropagateUnscoped: true}).Unscoped(), row.ID, nil)
		return err
	}))
	for _, model := range rows {
		var count int64
		require.NoError(t, db.Unscoped().Model(model).Where("cloud_account_id = ? AND deleted_at IS NOT NULL", row.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}

func TestService_RegionShrinkNativeFailureRollsBack(t *testing.T) {
	svc, repo, recorder, key := setupSvc(t)
	db := repo.DB()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 7, "", "", CreateRequest{Name: "region-rollback", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1", "us-west-2"}})
	require.NoError(t, err)
	other := dbtest.SeedCloudAccount(t, db, key.ID, "other-scope", "us-east-1")
	rows := seedAllProviderFamilies(t, db, detail.ID, "us-east-1", "removed")
	seedAllProviderFamilies(t, db, detail.ID, "us-west-2", "kept")
	seedAllProviderFamilies(t, db, other.ID, "us-east-1", "other")
	var before models.CloudAccount
	require.NoError(t, db.First(&before, detail.ID).Error)
	auditCount := len(recorder.snapshot())
	// The last regional family fails after the account revision and earlier
	// AWS/Aliyun/GCP families have already changed inside the transaction.
	require.NoError(t, db.Exec(`CREATE FUNCTION reject_regional_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test regional cleanup rejected'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_regional_cleanup BEFORE UPDATE ON gcp_databases FOR EACH ROW WHEN (OLD.region = 'us-east-1') EXECUTE FUNCTION reject_regional_cleanup()`).Error)
	_, err = svc.Update(ctx, 7, "", "", detail.ID, UpdateRequest{EnabledRegions: []string{"us-west-2"}})
	require.Error(t, err)
	var after models.CloudAccount
	require.NoError(t, db.First(&after, detail.ID).Error)
	require.Equal(t, before, after, "scope revision and enabled regions must roll back with resource cleanup")
	require.Equal(t, auditCount, len(recorder.snapshot()), "failed region change must not record a completed update")
	for _, model := range rows {
		var live, deleted int64
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", detail.ID).Count(&live).Error)
		require.EqualValues(t, 2, live, "both original scopes or global rows must survive")
		require.NoError(t, db.Unscoped().Model(model).Where("cloud_account_id = ? AND deleted_at IS NOT NULL", detail.ID).Count(&deleted).Error)
		require.Zero(t, deleted, "no earlier regional family may retain a partial cleanup")
		require.NoError(t, db.Model(model).Where("cloud_account_id = ?", other.ID).Count(&live).Error)
		require.EqualValues(t, 1, live, "the other account must stay untouched")
	}
}
