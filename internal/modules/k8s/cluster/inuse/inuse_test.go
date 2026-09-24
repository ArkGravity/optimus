//go:build dbtest

package inuse_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/logic3579/optimus/internal/infra/db"
	"github.com/logic3579/optimus/internal/models"
	"github.com/logic3579/optimus/internal/modules/k8s/cluster/inuse"
)

func TestCountByKubeconfigID(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	defer td()
	ctx := context.Background()

	kc := &models.CredentialKubeconfig{Name: "kc", KubeconfigEnc: []byte{1}}
	require.NoError(t, gdb.Create(kc).Error)

	n, err := inuse.CountByKubeconfigID(ctx, gdb, kc.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)

	require.NoError(t, gdb.Create(&models.Cluster{
		Name: "a", KubeconfigID: kc.ID, Context: "c",
	}).Error)
	n, err = inuse.CountByKubeconfigID(ctx, gdb, kc.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestCountByKubeconfigID_ExcludesSoftDeleted(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	defer td()
	ctx := context.Background()

	kc := &models.CredentialKubeconfig{Name: "kc2", KubeconfigEnc: []byte{1}}
	require.NoError(t, gdb.Create(kc).Error)
	c := &models.Cluster{Name: "x", KubeconfigID: kc.ID, Context: "c"}
	require.NoError(t, gdb.Create(c).Error)
	require.NoError(t, gdb.Delete(c).Error) // soft-delete
	n, err := inuse.CountByKubeconfigID(ctx, gdb, kc.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, n) // soft-deleted row should not count
}
