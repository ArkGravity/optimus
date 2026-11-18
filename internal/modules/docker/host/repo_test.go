//go:build dbtest

package host_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
)

func TestMigration_DockerHostsAndWritePermission(t *testing.T) {
	gdb, cleanup := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(cleanup)
	require.True(t, gdb.Migrator().HasTable("docker_hosts"))
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	var count int64
	require.NoError(t, gdb.Model(&models.Permission{}).Where("code = ?", "docker:environment:write").Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, gdb.Model(&models.Permission{}).Where("code = ? AND description = ?", "docker:environment:read", "Read Docker hosts, images and containers").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
