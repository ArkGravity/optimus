//go:build dbtest

package account

import (
	"context"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/infra/advisorylock"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestService_MulticloudBindings(t *testing.T) {
	for _, tc := range []struct {
		provider, auth, project, region string
		valid                           bool
	}{
		{"aws", "access_key", "", "us-gov-west-1", true},
		{"aliyun", "access_key", "", "cn-hangzhou", true},
		{"gcp", "service_account_json", "target-project", "us-central1", true},
		{"gcp", "legacy_invalid", "target-project", "us-central1", false},
	} {
		t.Run(tc.provider+tc.auth, func(t *testing.T) {
			svc, repo, _, key := setupSvc(t)
			fields := map[string]any{"provider": tc.provider, "auth_type": tc.auth}
			if tc.auth == "service_account_json" {
				fields["access_key_id_enc"] = nil
				fields["secret_access_key_enc"] = nil
				fields["service_account_json_enc"] = []byte("sealed-json")
				fields["service_account_email"] = "collector@example.test"
				fields["credential_project_id"] = "credential-home"
			}
			require.NoError(t, repo.DB().Model(key).Updates(fields).Error)
			got, err := svc.Create(context.Background(), 7, "", "", CreateRequest{Name: "connection", Provider: tc.provider, ProjectID: tc.project, CloudKeyID: key.ID, EnabledRegions: []string{tc.region}})
			if !tc.valid {
				requireBizCode(t, err, errs.CodeAssetsConfig)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.project, got.ProjectID)
			require.EqualValues(t, 1, got.ScopeRevision)
			_, err = svc.Create(context.Background(), 7, "", "", CreateRequest{Name: "wrong-provider", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1"}})
			if tc.provider != "aws" {
				requireBizCode(t, err, errs.CodeAssetsConfig)
			}
		})
	}
}

func TestService_BindingRechecksMetadataAfterConversionLock(t *testing.T) {
	svc, repo, _, key := setupSvc(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := repo.DB().WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, advisorylock.LockCloudKey(ctx, tx, key.ID))
	require.NoError(t, tx.Model(key).Update("provider", "aliyun").Error)
	done := make(chan error, 1)
	go func() {
		_, err := svc.Create(ctx, 0, "", "", CreateRequest{Name: "after-conversion", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1"}})
		done <- err
	}()
	require.Eventually(t, func() bool {
		var waiting int64
		err := repo.DB().WithContext(ctx).Raw("SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'").Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit().Error)
	requireBizCode(t, <-done, errs.CodeAssetsConfig)
	count, err := repo.CountByCloudKeyID(ctx, key.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestRepo_GCPIdentityUsesExplicitProject(t *testing.T) {
	repo, gdb := setupRepo(t)
	key := &models.CredentialCloudKey{Name: "gcp-pin", Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSONEnc: []byte("sealed"), ServiceAccountEmail: "collector@example.test", CredentialProjectID: "credential-home"}
	require.NoError(t, gdb.Create(key).Error)
	row := &models.CloudAccount{Name: "target", Provider: "gcp", ProjectID: "target-project", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-central1"}, Enabled: true}
	require.NoError(t, repo.Create(context.Background(), row))
	require.NoError(t, repo.Transaction(context.Background(), func(tx *gorm.DB) error {
		pinned, err := repo.PinIdentityTx(context.Background(), tx, CaptureScope(*row), "credential-home")
		require.NoError(t, err)
		require.Equal(t, "target-project", pinned.ProjectID)
		require.Empty(t, pinned.ExternalAccountID)
		return err
	}))
}

func TestService_ScopeRevisionAndImmutableBinding(t *testing.T) {
	svc, repo, _, key := setupSvc(t)
	ctx := context.Background()
	got, err := svc.Create(ctx, 7, "", "", CreateRequest{Name: "scope", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-west-2", "us-east-1", "us-east-1"}})
	require.NoError(t, err)
	require.Equal(t, []string{"us-east-1", "us-west-2"}, got.EnabledRegions)
	name := "renamed"
	got, err = svc.Update(ctx, 7, "", "", got.ID, UpdateRequest{Name: &name, EnabledRegions: []string{"us-west-2", "us-east-1"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, got.ScopeRevision)
	got, err = svc.Update(ctx, 7, "", "", got.ID, UpdateRequest{EnabledRegions: []string{"us-east-1"}})
	require.NoError(t, err)
	require.EqualValues(t, 2, got.ScopeRevision)
	provider, project, keyID := "aliyun", "other-project", key.ID+1
	for _, req := range []UpdateRequest{{Provider: &provider}, {ProjectID: &project}, {CloudKeyID: &keyID}} {
		_, err = svc.Update(ctx, 7, "", "", got.ID, req)
		requireBizCode(t, err, errs.CodeAssetsConfig)
	}
	_, err = svc.Update(ctx, 7, "", "", got.ID, UpdateRequest{EnabledRegions: []string{}})
	requireBizCode(t, err, errs.CodeAssetsRegionInvalid)
	row, err := repo.FindByID(ctx, got.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, row.ScopeRevision)
}

func TestRepo_PinIdentityAndRejectStaleScope(t *testing.T) {
	svc, repo, _, key := setupSvc(t)
	ctx := context.Background()
	got, err := svc.Create(ctx, 7, "", "", CreateRequest{Name: "identity", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1"}})
	require.NoError(t, err)
	row, err := repo.FindByID(ctx, got.ID)
	require.NoError(t, err)
	scope := CaptureScope(*row)
	pin := func(snapshot Scope, identity string) (*models.CloudAccount, error) {
		var pinned *models.CloudAccount
		err := repo.Transaction(ctx, func(tx *gorm.DB) error {
			var err error
			pinned, err = repo.PinIdentityTx(ctx, tx, snapshot, identity)
			return err
		})
		return pinned, err
	}
	_, err = pin(scope, "")
	requireBizCode(t, err, errs.CodeAssetsConfig)
	pinned, err := pin(scope, "123456789012")
	require.NoError(t, err)
	require.Equal(t, "123456789012", pinned.ExternalAccountID)
	pinned, err = pin(scope, "123456789012")
	require.NoError(t, err)
	require.Equal(t, "123456789012", CaptureScope(*pinned).ExternalAccountID)
	_, err = pin(scope, "999999999999")
	requireBizCode(t, err, errs.CodeAssetsIdentityMismatch)
	_, err = svc.Update(ctx, 7, "", "", got.ID, UpdateRequest{EnabledRegions: []string{"us-west-2"}})
	require.NoError(t, err)
	_, err = pin(scope, "123456789012")
	requireBizCode(t, err, errs.CodeAssetsSyncBusy)
	row, err = repo.FindByID(ctx, got.ID)
	require.NoError(t, err)
	scope = CaptureScope(*row)
	enabled := false
	_, err = svc.Update(ctx, 7, "", "", got.ID, UpdateRequest{Enabled: &enabled})
	require.NoError(t, err)
	_, err = pin(scope, "123456789012")
	requireBizCode(t, err, errs.CodeAssetsSyncBusy)
	require.NoError(t, repo.SoftDelete(ctx, got.ID))
	_, err = pin(scope, "123456789012")
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNotFound)
}
