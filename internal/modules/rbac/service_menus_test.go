//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
	"github.com/ArkGravity/optimus/internal/seed"
)

func TestMeService_ListMenus_ViewerSeesOnlyPermittedNodes(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x"})
	require.NoError(t, err)

	u := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID}).Error)

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute), nopUserWriter{})
	tree, err := svc.ListMenus(context.Background(), u.ID)
	require.NoError(t, err)

	var foundDashboard, foundSystem, foundCredentials bool
	for _, top := range tree {
		if top.Code == "dashboard" {
			foundDashboard = true
		}
		if top.Code == "system" {
			foundSystem = true
		}
		if top.Code == "credentials" {
			foundCredentials = true
		}
	}
	require.True(t, foundDashboard)
	require.False(t, foundSystem)
	require.False(t, foundCredentials)
	require.Equal(t, []string{"dashboard", "assets", "containers", "apps", "observability"}, codes(tree))
	for _, top := range tree {
		if top.Code != "containers" {
			continue
		}
		require.Equal(t, "menu.containers_group", top.Name)
		require.Equal(t, "/containers", top.Path)
		require.Equal(t, []string{"containers.kubernetes", "containers.docker"}, codes(top.Children))
		require.Equal(t, "/containers/kubernetes", top.Children[0].Path)
		require.Equal(t, "containers/Kubernetes", top.Children[0].Component)
		require.Nil(t, top.Children[0].PermissionCode)
		require.Equal(t, "/containers/docker", top.Children[1].Path)
		require.Equal(t, "containers/Docker", top.Children[1].Component)
		require.NotNil(t, top.Children[1].PermissionCode)
		require.Equal(t, "docker:environment:read", *top.Children[1].PermissionCode)
	}
}

func codes(nodes []rbac.MeMenuNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Code)
	}
	return out
}
