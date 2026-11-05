//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLockScopeTx(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, db, "publish-scope")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "scope", "us-east-1")
	loc := resource.Location{Type: "region", Name: "us-east-1", Region: "us-east-1"}
	scope := CaptureScope(*row)
	_, err := LockScopeTx(ctx, db, scope, loc)
	require.Error(t, err, "an unlocked standalone handle cannot publish")
	for _, tc := range []struct {
		name   string
		change func(*Scope, *resource.Location)
	}{
		{"eligible", func(*Scope, *resource.Location) {}},
		{"account", func(s *Scope, _ *resource.Location) { s.AccountID++ }},
		{"provider", func(s *Scope, _ *resource.Location) { s.Provider = "aliyun" }},
		{"project", func(s *Scope, _ *resource.Location) { s.ProjectID = "wrong" }},
		{"binding", func(s *Scope, _ *resource.Location) { s.CloudKeyID++ }},
		{"identity", func(s *Scope, _ *resource.Location) { s.ExternalAccountID = "wrong" }},
		{"revision", func(s *Scope, _ *resource.Location) { s.Revision++ }},
		{"captured-disabled", func(s *Scope, _ *resource.Location) { s.Enabled = false }},
		{"removed-region", func(_ *Scope, l *resource.Location) { l.Name = "us-west-2"; l.Region = l.Name }},
		{"invalid-location", func(_ *Scope, l *resource.Location) { l.Zone = "wrong" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured, location := scope, loc
			tc.change(&captured, &location)
			err := repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, captured, location); return err })
			if tc.name == "eligible" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrScopeIneligible)
			}
		})
	}
	require.NoError(t, db.Model(row).Update("enabled", false).Error)
	require.ErrorIs(t, repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, scope, loc); return err }), ErrScopeIneligible)
	require.NoError(t, db.Model(row).Update("enabled", true).Error)
	require.NoError(t, db.Delete(row).Error)
	require.ErrorIs(t, repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx.Unscoped(), scope, loc); return err }), ErrScopeIneligible)

	require.ErrorIs(t, repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, scope, loc); return err }), ErrScopeIneligible)
}

func TestLockScopeTxGlobalAndZonal(t *testing.T) {
	_, db := setupRepo(t)
	ctx := context.Background()
	key := &models.CredentialCloudKey{Name: "gcp-scope", Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSONEnc: []byte("sealed"), ServiceAccountEmail: "reader@example.test", CredentialProjectID: "home-project"}
	require.NoError(t, db.Create(key).Error)
	row := &models.CloudAccount{Name: "gcp", Provider: "gcp", ProjectID: "target-project", CloudKeyID: key.ID, Enabled: true, EnabledRegions: models.StringArray{"us-central1"}}
	require.NoError(t, db.Create(row).Error)
	for _, loc := range []resource.Location{{Type: "global", Name: "global"}, {Type: "zone", Name: "us-central1-a", Zone: "us-central1-a", Region: "us-central1"}} {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, CaptureScope(*row), loc); return err }))
	}
	require.NoError(t, db.Model(row).Update("enabled_regions", models.StringArray{"europe-west1"}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := LockScopeTx(ctx, tx, CaptureScope(*row), resource.Location{Type: "global", Name: "global"})
		return err
	}))
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := LockScopeTx(ctx, tx, CaptureScope(*row), resource.Location{Type: "zone", Name: "us-central1-a", Zone: "us-central1-a", Region: "us-central1"})
		return err
	}), ErrScopeIneligible)
}

func TestLockScopeTxWaitsAndRechecksAccount(t *testing.T) {
	_, db := setupRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := dbtest.SeedCloudKey(t, db, "waiting-scope")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "waiting", "us-east-1")
	scope := CaptureScope(*row)
	loc := resource.Location{Type: "region", Name: "us-east-1", Region: "us-east-1"}
	blocker := db.WithContext(ctx).Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	require.NoError(t, blocker.Model(row).Update("enabled", false).Error)
	done := make(chan error, 1)
	go func() {
		done <- db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, scope, loc); return err })
	}()
	require.Eventually(t, func() bool {
		var count int64
		err := db.WithContext(ctx).Raw("SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'").Scan(&count).Error
		return err == nil && count > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, blocker.Commit().Error)
	require.ErrorIs(t, <-done, ErrScopeIneligible)
}

func TestLockScopeTxAcceptsAWSZoneName(t *testing.T) {
	repo, db := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, db, "aws-zone")
	row := dbtest.SeedCloudAccount(t, db, key.ID, "aws-zone", "us-east-1")
	loc := resource.Location{Type: "zone", Name: "us-east-1a", Region: "us-east-1", Zone: "us-east-1a"}
	require.NoError(t, repo.Transaction(ctx, func(tx *gorm.DB) error { _, err := LockScopeTx(ctx, tx, CaptureScope(*row), loc); return err }))
}
