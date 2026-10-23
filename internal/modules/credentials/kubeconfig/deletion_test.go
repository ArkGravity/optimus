//go:build dbtest

package kubeconfig_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/credentials/kubeconfig"
	"github.com/ArkGravity/optimus/internal/modules/k8s/cluster"
)

func TestDeleteDetachesHistoricalReferencesAndPreservesLiveProtection(t *testing.T) {
	svc, cleanup := newSvc(t)
	t.Cleanup(cleanup)
	ctx := t.Context()
	credential, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "cleanup", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)
	db := svc.Repo().DB()
	first := &models.Cluster{Name: "first", KubeconfigID: credential.ID, Context: "first"}
	second := &models.Cluster{Name: "second", KubeconfigID: credential.ID, Context: "second"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)
	require.NoError(t, db.Delete(first).Error)
	err = svc.Delete(ctx, 0, "127.0.0.1", "test-agent", credential.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
	require.Equal(t, "credentials.kubeconfig.in_use", be.MessageKey)
	var historical models.Cluster
	require.NoError(t, db.Unscoped().First(&historical, first.ID).Error)
	require.Equal(t, credential.ID, historical.KubeconfigID)
	require.NoError(t, db.Delete(second).Error)
	require.NoError(t, svc.Delete(ctx, 0, "127.0.0.1", "test-agent", credential.ID))
	var credentials int64
	require.NoError(t, db.Unscoped().Model(&models.CredentialKubeconfig{}).Where("id = ?", credential.ID).Count(&credentials).Error)
	require.Zero(t, credentials)
	var history []models.Cluster
	require.NoError(t, db.Unscoped().Where("id IN ?", []uint64{first.ID, second.ID}).Order("id").Find(&history).Error)
	require.Len(t, history, 2)
	for _, row := range history {
		require.True(t, row.DeletedAt.Valid)
		require.Zero(t, row.KubeconfigID)
	}
	require.Equal(t, "first", history[0].Name)
	require.Error(t, db.Unscoped().Model(&models.Cluster{}).Where("id = ?", first.ID).Update("deleted_at", nil).Error,
		"a detached cluster must not become live without a credential")
	var events []models.AuditLog
	require.NoError(t, db.Where("action = ? AND target_type = ? AND target_id = ?", "credentials.delete", "credentials.kubeconfig", strconv.FormatUint(credential.ID, 10)).Find(&events).Error)
	require.Len(t, events, 1)
	require.Equal(t, "127.0.0.1", events[0].IP)
	_, err = svc.Get(ctx, credential.ID)
	be, ok = apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestDeleteRollsBackDetachmentWhenDeleteOrAuditFails(t *testing.T) {
	for _, failure := range []struct{ name, table, operation string }{
		{"delete", "credentials_kubeconfigs", "DELETE"},
		{"audit", "audit_logs", "INSERT"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			svc, cleanup := newSvc(t)
			t.Cleanup(cleanup)
			credential, err := svc.Create(t.Context(), 0, "", "", kubeconfig.CreateRequest{Name: "rollback", Kubeconfig: validKubeconfigYAML})
			require.NoError(t, err)
			db := svc.Repo().DB()
			history := &models.Cluster{Name: "history", KubeconfigID: credential.ID, Context: "history"}
			require.NoError(t, db.Create(history).Error)
			require.NoError(t, db.Delete(history).Error)
			require.NoError(t, db.Exec(`CREATE FUNCTION reject_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced cleanup failure'; END $$`).Error)
			require.NoError(t, db.Exec("CREATE TRIGGER reject_cleanup BEFORE "+failure.operation+" ON "+failure.table+" FOR EACH ROW EXECUTE FUNCTION reject_cleanup()").Error)
			err = svc.Delete(t.Context(), 0, "", "", credential.ID)
			be, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, apperr.CodeDBError, be.Code)
			require.Equal(t, "database operation failed", be.Message)
			require.NoError(t, db.Unscoped().First(history, history.ID).Error)
			require.Equal(t, credential.ID, history.KubeconfigID)
			_, err = svc.Get(t.Context(), credential.ID)
			require.NoError(t, err)
			var count int64
			require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ?", "credentials.delete").Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestConcurrentClusterReferencePreventsCredentialDeletion(t *testing.T) {
	svc, cleanup := newSvc(t)
	t.Cleanup(cleanup)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	credential, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "concurrent", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)
	db := svc.Repo().DB()
	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, cluster.NewRepo(tx).Create(ctx, &models.Cluster{Name: "racing", KubeconfigID: credential.ID, Context: "racing"}))
	done := make(chan error, 1)
	go func() { done <- svc.Delete(ctx, 0, "", "", credential.ID) }()
	require.Eventually(t, func() bool {
		var waiting int64
		err := db.WithContext(ctx).Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit().Error)
	err = <-done
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
	_, err = svc.Get(ctx, credential.ID)
	require.NoError(t, err)
}
