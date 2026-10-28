//go:build dbtest

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type securityGroupTestFetcher struct {
	fakeFetcher
	groups []models.AWSSecurityGroup
	err    error
}

func (f *securityGroupTestFetcher) FetchSecurityGroups(context.Context, *Clients) ([]models.AWSSecurityGroup, error) {
	return f.groups, f.err
}

func TestSecurityGroupSyncFailureProtectionAndRestoration(t *testing.T) {
	fetcher := &securityGroupTestFetcher{groups: []models.AWSSecurityGroup{{GroupID: "sg-keep"}, {GroupID: "sg-remove"}}}
	db, acc, engine, _, _ := setupEngine(t, fetcher)
	require.NoError(t, engine.RunAccount(t.Context(), acc.ID, "cron", nil))
	var row models.AWSSecurityGroup
	require.NoError(t, db.Where("group_id = ?", "sg-remove").First(&row).Error)
	firstID := row.ID
	fetcher.err = errors.New("upstream secret failure")
	fetcher.groups = nil
	require.NoError(t, engine.RunAccount(t.Context(), acc.ID, "manual", nil))
	var alive int64
	require.NoError(t, db.Model(&models.AWSSecurityGroup{}).Count(&alive).Error)
	require.EqualValues(t, 2, alive)
	var run models.AssetsSyncRun
	require.NoError(t, db.Where("resource_type = ?", "security_group").Order("id DESC").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.NotContains(t, run.Error, "secret")
	fetcher.err = nil
	fetcher.groups = []models.AWSSecurityGroup{{GroupID: "sg-keep", GroupName: "updated"}}
	require.NoError(t, engine.RunAccount(t.Context(), acc.ID, "cron", nil))
	var deleted models.AWSSecurityGroup
	require.NoError(t, db.Unscoped().First(&deleted, firstID).Error)
	require.True(t, deleted.DeletedAt.Valid)
	fetcher.groups = []models.AWSSecurityGroup{{GroupID: "sg-remove"}}
	require.NoError(t, engine.RunAccount(t.Context(), acc.ID, "cron", nil))
	var restored models.AWSSecurityGroup
	require.NoError(t, db.First(&restored, firstID).Error)
	require.Equal(t, firstID, restored.ID)
	fetcher.groups = nil
	require.NoError(t, engine.RunAccount(t.Context(), acc.ID, "cron", nil))
	require.NoError(t, db.Model(&models.AWSSecurityGroup{}).Count(&alive).Error)
	require.Zero(t, alive)
}

func TestSecurityGroupScopeAndAccountCleanup(t *testing.T) {
	db, acc, _, _, _ := setupEngine(t, &fakeFetcher{})
	require.NoError(t, db.Model(acc).Update("enabled_regions", models.StringArray{"us-east-1", "eu-west-1"}).Error)
	for _, region := range []string{"us-east-1", "eu-west-1"} {
		_, err := UpsertSecurityGroups(t.Context(), db, acc.ID, region, time.Now(), []models.AWSSecurityGroup{{GroupID: "sg-same"}})
		require.NoError(t, err)
	}
	_, err := UpsertSecurityGroups(t.Context(), db, acc.ID, "us-east-1", time.Now(), nil)
	require.NoError(t, err)
	var alive int64
	require.NoError(t, db.Model(&models.AWSSecurityGroup{}).Count(&alive).Error)
	require.EqualValues(t, 1, alive)
	repo := account.NewRepo(db)
	require.NoError(t, repo.Transaction(t.Context(), func(tx *gorm.DB) error {
		_, err := repo.CascadeSoftDeleteResources(t.Context(), tx, acc.ID, []string{"eu-west-1"})
		return err
	}))
	require.NoError(t, db.Model(&models.AWSSecurityGroup{}).Count(&alive).Error)
	require.Zero(t, alive)
	_, err = UpsertSecurityGroups(t.Context(), db, acc.ID, "eu-west-1", time.Now(), []models.AWSSecurityGroup{{GroupID: "sg-live"}})
	require.NoError(t, err)
	require.NoError(t, repo.Transaction(t.Context(), func(tx *gorm.DB) error {
		_, err := repo.CascadeSoftDeleteResources(t.Context(), tx, acc.ID, nil)
		return err
	}))
	require.NoError(t, db.Model(&models.AWSSecurityGroup{}).Count(&alive).Error)
	require.Zero(t, alive)
	require.NoError(t, db.Model(acc).Update("enabled", false).Error)
	_, err = UpsertSecurityGroups(t.Context(), db, acc.ID, "eu-west-1", time.Now(), []models.AWSSecurityGroup{{GroupID: "sg-new"}})
	require.ErrorIs(t, err, ErrSweepIneligible)
}
