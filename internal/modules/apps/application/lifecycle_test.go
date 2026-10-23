//go:build dbtest

package application_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/apps/application"
)

func TestLifecycleLockSerializesIndependentServicesAndDeletion(t *testing.T) {
	svc, repo, clusterID, chartRepoID := setupSvc(t)
	app := &models.AppsApplication{Name: "locked", ClusterID: clusterID, Namespace: "default", ReleaseName: "locked", ChartRepoID: chartRepoID, ChartName: "demo"}
	require.NoError(t, repo.Create(t.Context(), app))
	other := application.NewService(application.NewRepo(repo.DB()), nil)
	started := make(chan struct{})
	release := make(chan struct{})
	releaseLock := sync.OnceFunc(func() { close(release) })
	defer releaseLock()
	done := make(chan error, 1)
	go func() {
		done <- svc.WithLifecycleLock(t.Context(), app.ID, func() error {
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("initial lock failed: %v", err)
	}
	called := false
	err := other.WithLifecycleLock(t.Context(), app.ID, func() error { called = true; return nil })
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
	require.False(t, called)
	err = other.Delete(t.Context(), 0, "", "", app.ID)
	be, ok = apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
	releaseLock()
	require.NoError(t, <-done)
	require.NoError(t, other.Delete(t.Context(), 0, "", "", app.ID))
}
