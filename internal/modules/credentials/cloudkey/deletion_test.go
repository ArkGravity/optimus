//go:build dbtest

package cloudkey_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/account/inuse"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
)

func TestDeleteAfterCloudAccountCleanupPreservesHistory(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	ctx := t.Context()
	svc.SetAccountsInUseCounter(inuse.New(db))
	key, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	accounts := account.NewService(account.NewRepo(db), audit.NewRecorder(db), svc)
	row, err := accounts.Create(ctx, 0, "", "", account.CreateRequest{
		Name: "cleanup", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	instance := &models.AWSInstance{CloudAccountID: row.ID, Region: "us-east-1", InstanceID: "i-cleanup"}
	require.NoError(t, db.Create(instance).Error)
	run := &models.AssetsSyncRun{CloudAccountID: row.ID, Region: "us-east-1", ResourceType: "instance", Status: "success", Trigger: "manual"}
	require.NoError(t, db.Create(run).Error)

	err = svc.Delete(ctx, 0, "", "", key.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
	cascaded, err := accounts.Delete(ctx, 0, "", "", row.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), cascaded)

	require.NoError(t, svc.Delete(ctx, 0, "127.0.0.1", "test-agent", key.ID))
	_, err = svc.Get(ctx, key.ID)
	be, ok = apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
	var keyCount int64
	require.NoError(t, db.Model(&models.CredentialCloudKey{}).Where("id = ?", key.ID).Count(&keyCount).Error)
	require.Zero(t, keyCount)
	var history models.CloudAccount
	require.NoError(t, db.Unscoped().First(&history, row.ID).Error)
	require.True(t, history.DeletedAt.Valid)
	require.Equal(t, row.Name, history.Name)
	require.Zero(t, history.CloudKeyID)
	var detached bool
	require.NoError(t, db.Raw("SELECT cloudkey_id IS NULL FROM cloud_accounts WHERE id = ?", row.ID).Scan(&detached).Error)
	require.True(t, detached)
	var historicalInstance models.AWSInstance
	require.NoError(t, db.Unscoped().First(&historicalInstance, instance.ID).Error)
	require.True(t, historicalInstance.DeletedAt.Valid)
	var historicalRun models.AssetsSyncRun
	require.NoError(t, db.First(&historicalRun, run.ID).Error)
	require.Equal(t, row.ID, historicalRun.CloudAccountID)
	list, err := accounts.List(ctx, account.ListQuery{IncludeDeleted: true})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Zero(t, list.Items[0].CloudKeyID)
	require.Empty(t, list.Items[0].CloudKeyName)
	var audits []models.AuditLog
	require.NoError(t, db.Where("action = ? AND target_type = ? AND target_id = ?", "credentials.delete", "credentials.cloud_key", strconv.FormatUint(key.ID, 10)).Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "127.0.0.1", audits[0].IP)
	require.Equal(t, "test-agent", audits[0].UserAgent)
}

func TestDeletePreservesLiveAndUnrelatedCloudAccountReferences(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	svc.SetAccountsInUseCounter(inuse.New(db))
	key, err := svc.Create(t.Context(), 0, "", "", newReq())
	require.NoError(t, err)
	otherReq := newReq()
	otherReq.Name = "other"
	otherKey, err := svc.Create(t.Context(), 0, "", "", otherReq)
	require.NoError(t, err)
	history := &models.CloudAccount{Name: "history", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	live := &models.CloudAccount{Name: "live-disabled", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	other := &models.CloudAccount{Name: "other-history", Provider: "aws", CloudKeyID: otherKey.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	for _, row := range []*models.CloudAccount{history, live, other} {
		require.NoError(t, db.Create(row).Error)
	}
	require.NoError(t, db.Model(live).Update("enabled", false).Error)
	require.NoError(t, db.Delete(history).Error)
	require.NoError(t, db.Delete(other).Error)
	err = svc.Delete(t.Context(), 0, "", "", key.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
	require.NoError(t, db.Unscoped().First(history, history.ID).Error)
	require.Equal(t, key.ID, history.CloudKeyID)
	require.Error(t, db.Model(live).Update("cloudkey_id", nil).Error, "live accounts must retain a credential")
	require.Error(t, db.Delete(&models.CredentialCloudKey{}, key.ID).Error, "the foreign key must remain restrictive")
	require.NoError(t, db.Delete(live).Error)
	require.NoError(t, svc.Delete(t.Context(), 0, "", "", key.ID))
	require.NoError(t, db.Unscoped().First(other, other.ID).Error)
	require.Equal(t, otherKey.ID, other.CloudKeyID)
	require.Error(t, db.Unscoped().Model(history).Update("deleted_at", nil).Error,
		"a detached account must not become live without a credential")
}

func TestDeleteRollsBackCloudKeyCleanupOnFailure(t *testing.T) {
	for _, failure := range []struct{ name, table, operation string }{
		{"detach", "cloud_accounts", "UPDATE"},
		{"delete", "credentials_cloud_keys", "DELETE"},
		{"audit", "audit_logs", "INSERT"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			svc, db, cleanup := newSvcWithDB(t)
			t.Cleanup(cleanup)
			svc.SetAccountsInUseCounter(inuse.New(db))
			key, err := svc.Create(t.Context(), 0, "", "", newReq())
			require.NoError(t, err)
			history := &models.CloudAccount{Name: "history", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
			require.NoError(t, db.Create(history).Error)
			require.NoError(t, db.Delete(history).Error)
			require.NoError(t, db.Exec(`CREATE FUNCTION reject_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced cleanup failure'; END $$`).Error)
			require.NoError(t, db.Exec("CREATE TRIGGER reject_cleanup BEFORE "+failure.operation+" ON "+failure.table+" FOR EACH ROW EXECUTE FUNCTION reject_cleanup()").Error)
			err = svc.Delete(t.Context(), 0, "", "", key.ID)
			be, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, apperr.CodeDBError, be.Code)
			require.Equal(t, "database operation failed", be.Message)
			require.NotNil(t, be.Unwrap())
			_, err = svc.Get(t.Context(), key.ID)
			require.NoError(t, err)
			require.NoError(t, db.Unscoped().First(history, history.ID).Error)
			require.Equal(t, key.ID, history.CloudKeyID)
			var audits int64
			require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ?", "credentials.delete").Count(&audits).Error)
			require.Zero(t, audits)
		})
	}
}

func TestConcurrentCloudAccountReferencePreventsCloudKeyDeletion(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	svc.SetAccountsInUseCounter(inuse.New(db))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	key, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, tx.Create(&models.CloudAccount{
		Name: "racing", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"},
	}).Error)
	done := make(chan error, 1)
	go func() { done <- svc.Delete(ctx, 0, "", "", key.ID) }()
	require.Eventually(t, func() bool {
		var waiting int64
		err := db.WithContext(ctx).Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit().Error)
	err = <-done
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
	_, err = svc.Get(ctx, key.ID)
	require.NoError(t, err)
}

func TestHandlerDeletesCloudKeyAfterAccountCleanup(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	svc.SetAccountsInUseCounter(inuse.New(db))
	key, err := svc.Create(t.Context(), 0, "", "", newReq())
	require.NoError(t, err)
	row := &models.CloudAccount{Name: "cleanup", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.NoError(t, db.Create(row).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := cloudkey.NewHandler(svc)
	router.DELETE("/api/v1/credentials/cloud-keys/:id", handler.HandleDelete())
	path := "/api/v1/credentials/cloud-keys/" + strconv.FormatUint(key.ID, 10)
	response := doJSON(t, router, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), `"code":43001`)
	require.NoError(t, db.Delete(row).Error)
	response = doJSON(t, router, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"code":0`)
	response = doJSON(t, router, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusNotFound, response.Code)
}
