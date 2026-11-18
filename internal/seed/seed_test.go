//go:build dbtest

package seed_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/seed"
)

func TestRun_IsIdempotent(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	r1, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.NotEmpty(t, r1.AdminInitialPassword)

	r2, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.Empty(t, r2.AdminInitialPassword, "second seed must not print a password")

	var users int64
	gdb.Model(&models.User{}).Where("username = ?", "admin").Count(&users)
	require.Equal(t, int64(1), users)

	var roles int64
	gdb.Model(&models.Role{}).Where("is_builtin").Count(&roles)
	require.Equal(t, int64(3), roles, "expected admin + viewer + editor builtin roles")
}

func TestRun_ContainerMenusUpgradePreservesIDsCustomChildrenAndGrants(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	opts := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com", BcryptCost: 4}
	_, err = seed.Run(ctx, gdb, opts)
	require.NoError(t, err)
	var parent, product models.Menu
	require.NoError(t, gdb.Where("code = ?", "containers").First(&parent).Error)
	require.NoError(t, gdb.Where("code = ?", "containers.kubernetes").First(&product).Error)
	require.NoError(t, gdb.Model(&parent).Updates(map[string]any{"code": "k8s", "name": "menu.k8s_group", "path": "/k8s"}).Error)
	require.NoError(t, gdb.Model(&product).Updates(map[string]any{"code": "k8s.clusters", "path": "/k8s/clusters", "component": "k8s/clusters/List", "permission_code": "k8s:cluster:read"}).Error)
	var custom []models.Menu
	for _, code := range []string{"k8s.workloads", "k8s.network", "k8s.config", "k8s.cluster_resources"} {
		old := models.Menu{Code: code, Name: code, ParentID: &parent.ID}
		require.NoError(t, gdb.Create(&old).Error)
		child := models.Menu{Code: "custom." + code, Name: "custom", ParentID: &old.ID}
		require.NoError(t, gdb.Create(&child).Error)
		custom = append(custom, child)
	}
	var grantsBefore []models.RolePermission
	customRole := models.Role{Code: "custom_container_reader", Name: "Custom container reader"}
	require.NoError(t, gdb.Create(&customRole).Error)
	var networkPermission models.Permission
	require.NoError(t, gdb.Where("code = ?", "k8s:network:read").First(&networkPermission).Error)
	customGrant := models.RolePermission{
		RoleID: customRole.ID, PermissionID: networkPermission.ID,
		CreatedAt: time.Date(2026, time.November, 12, 0, 0, 0, 123456789, time.UTC),
	}
	require.NoError(t, gdb.Create(&customGrant).Error)
	// Compare database snapshots so sub-microsecond precision loss at insertion
	// cannot be mistaken for a grant rewrite during seed.
	var persistedGrant models.RolePermission
	require.NoError(t, gdb.Where("role_id = ? AND permission_id = ?", customRole.ID, networkPermission.ID).First(&persistedGrant).Error)
	require.NoError(t, gdb.Select("role_id, permission_id").Order("role_id, permission_id").Find(&grantsBefore).Error)
	for range 2 {
		_, err = seed.Run(ctx, gdb, opts)
		require.NoError(t, err)
	}
	var currentParent, currentProduct models.Menu
	require.NoError(t, gdb.Where("code = ?", "containers").First(&currentParent).Error)
	require.NoError(t, gdb.Where("code = ?", "containers.kubernetes").First(&currentProduct).Error)
	require.Equal(t, parent.ID, currentParent.ID)
	require.Equal(t, product.ID, currentProduct.ID)
	require.Equal(t, "/containers/kubernetes", currentProduct.Path)
	require.Nil(t, currentProduct.PermissionCode)
	for _, child := range custom {
		var got models.Menu
		require.NoError(t, gdb.First(&got, child.ID).Error)
		require.Equal(t, &product.ID, got.ParentID)
	}
	var oldCount int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code LIKE ?", "k8s%").Count(&oldCount).Error)
	require.Zero(t, oldCount)
	var grantsAfter []models.RolePermission
	require.NoError(t, gdb.Select("role_id, permission_id").Order("role_id, permission_id").Find(&grantsAfter).Error)
	require.Equal(t, grantsBefore, grantsAfter)
	var retainedGrant models.RolePermission
	require.NoError(t, gdb.Where("role_id = ? AND permission_id = ?", customRole.ID, networkPermission.ID).First(&retainedGrant).Error)
	require.True(t, persistedGrant.CreatedAt.Equal(retainedGrant.CreatedAt), "custom grants must not be rewritten")
}

func TestRun_ContainerMenuCollisionRollsBack(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	oldParent := models.Menu{Code: "k8s", Name: "old"}
	require.NoError(t, gdb.Create(&oldParent).Error)
	oldProduct := models.Menu{Code: "k8s.clusters", Name: "old", ParentID: &oldParent.ID}
	require.NoError(t, gdb.Create(&oldProduct).Error)
	collision := models.Menu{Code: "containers.kubernetes", Name: "operator-owned"}
	require.NoError(t, gdb.Create(&collision).Error)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", BcryptCost: 4})
	require.ErrorContains(t, err, "container menu collision")
	var after models.Menu
	require.NoError(t, gdb.First(&after, oldParent.ID).Error)
	require.Equal(t, "k8s", after.Code, "parent rename must roll back with child collision")
	var roles int64
	require.NoError(t, gdb.Model(&models.Role{}).Count(&roles).Error)
	require.Zero(t, roles, "permission and role initialization is atomic with menus")
}

func TestRun_AdminRoleHasAllPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var adminRole models.Role
	require.NoError(t, gdb.Where("code = ?", "admin").First(&adminRole).Error)
	var bound int64
	gdb.Model(&models.RolePermission{}).Where("role_id = ?", adminRole.ID).Count(&bound)
	require.Equal(t, int64(len(permissions.All)), bound)
}

func TestRun_FailsLoudlyWhenNoPermissionsRegistered(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	// Note: we deliberately do NOT call permissions.Register
	_, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "permissions")
}

func TestRun_ViewerRoleHasOnlyReadPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	var operatorBeforeSeed int64
	require.NoError(t, gdb.Model(&models.Role{}).Where("code = ?", "k8s_operator").Count(&operatorBeforeSeed).Error)
	require.Zero(t, operatorBeforeSeed, "seed must not require a k8s_operator role")
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	var perms []models.Permission
	gdb.Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", viewer.ID).
		Find(&perms)
	require.NotEmpty(t, perms)
	for _, p := range perms {
		require.Contains(t, p.Code, ":read")
		require.NotRegexp(t, `^(system|credentials):`, p.Code)
	}
}

func TestRun_EditorRoleHasAllNonSystemPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"})
	require.NoError(t, err)

	var editor models.Role
	require.NoError(t, gdb.Where("code = ?", "editor").First(&editor).Error)
	var got []string
	require.NoError(t, gdb.Table("permissions").
		Select("permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", editor.ID).
		Order("permissions.code").
		Scan(&got).Error)

	var want []string
	require.NoError(t, gdb.Model(&models.Permission{}).
		Where("code NOT LIKE ?", "system:%").Order("code").Pluck("code", &want).Error)
	require.Equal(t, want, got)
	for _, code := range got {
		require.NotRegexp(t, `^system:`, code)
	}
}

func TestRun_PromotesExistingEditorToBuiltinRole(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.NoError(t, gdb.Create(&models.Role{
		Code: "editor", Name: "Legacy editor", Description: "custom", IsBuiltin: false,
	}).Error)

	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"})
	require.NoError(t, err)
	var editor models.Role
	require.NoError(t, gdb.Where("code = ?", "editor").First(&editor).Error)
	require.True(t, editor.IsBuiltin)
	require.Equal(t, "role.editor", editor.Name)
	require.Equal(t, "Full access except system administration", editor.Description)
}

func TestRun_ResynchronizesBuiltinRolePermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	opts := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"}
	_, err = seed.Run(context.Background(), gdb, opts)
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	var systemRead models.Permission
	require.NoError(t, gdb.Where("code = ?", "system:user:read").First(&systemRead).Error)
	require.NoError(t, gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: systemRead.ID}).Error)

	_, err = seed.Run(context.Background(), gdb, opts)
	require.NoError(t, err)
	var stale int64
	require.NoError(t, gdb.Model(&models.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", viewer.ID, systemRead.ID).Count(&stale).Error)
	require.Zero(t, stale)
}

func TestRun_SeedsInitialMenuTree(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	wantCodes := []string{
		"dashboard",
		"assets", "assets.inventory",
		"system", "system.users", "system.roles", "system.permissions", "system.menus", "system.audit_logs",
		"credentials", "credentials.ssh_keys", "credentials.kubeconfigs", "credentials.cloud_keys",
		"containers", "containers.kubernetes", "containers.docker",
		"apps", "apps.applications", "apps.chart_repos",
		"observability", "observability.overview",
	}
	var menus []models.Menu
	require.NoError(t, gdb.Find(&menus).Error)
	var menuCodes []string
	for _, menu := range menus {
		menuCodes = append(menuCodes, menu.Code)
	}
	require.ElementsMatch(t, wantCodes, menuCodes)
	var assetParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets").First(&assetParent).Error)
	var assetChildren []models.Menu
	require.NoError(t, gdb.Where("parent_id = ?", assetParent.ID).Order("sort_order, id").Find(&assetChildren).Error)
	var assetCodes []string
	for _, menu := range assetChildren {
		assetCodes = append(assetCodes, menu.Code)
	}
	require.Equal(t, []string{"assets.inventory"}, assetCodes)
	var topLevel []models.Menu
	require.NoError(t, gdb.Where("parent_id IS NULL").Order("sort_order, id").Find(&topLevel).Error)
	var topLevelCodes []string
	for _, menu := range topLevel {
		topLevelCodes = append(topLevelCodes, menu.Code)
	}
	require.Equal(t,
		[]string{"dashboard", "assets", "containers", "apps", "observability", "credentials", "system"},
		topLevelCodes,
	)

	// Parent linkage: credentials.* children must have parent_id = credentials.id.
	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "credentials").First(&parent).Error)
	var childrenCount int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", parent.ID).Count(&childrenCount)
	require.Equal(t, int64(3), childrenCount)

	var observability models.Menu
	require.NoError(t, gdb.Where("code = ?", "observability").First(&observability).Error)
	require.Nil(t, observability.PermissionCode)
	var children []models.Menu
	require.NoError(t, gdb.Where("parent_id = ?", observability.ID).Find(&children).Error)
	require.Len(t, children, 1)
	require.Equal(t, "observability.overview", children[0].Code)
	require.Equal(t, "/observability/overview", children[0].Path)
	require.Equal(t, "observability/overview/Index", children[0].Component)
	require.Nil(t, children[0].PermissionCode)

	// Parent linkage: the container group contains Kubernetes and Docker.
	var k8sParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "containers").First(&k8sParent).Error)
	var k8sChildren int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", k8sParent.ID).Count(&k8sChildren)
	require.Equal(t, int64(2), k8sChildren)

	// Parent linkage: apps.* children must have parent_id = apps.id (P3).
	var appsParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "apps").First(&appsParent).Error)
	var appsChildren int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", appsParent.ID).Count(&appsChildren)
	require.Equal(t, int64(2), appsChildren)
}

// TestRun_AdminRoleIncludesAppsPermissions covers the implicit P3 grant:
// bindAdminPermissions binds every permission row, so the 10 apps:* codes
// flow to admin automatically without explicit per-role wiring in seed.
func TestRun_AdminRoleIncludesAppsPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var adminRole models.Role
	require.NoError(t, gdb.Where("code = ?", "admin").First(&adminRole).Error)

	wantCodes := []string{
		"apps:repo:read", "apps:repo:write", "apps:repo:delete",
		"apps:application:read", "apps:application:write", "apps:application:delete",
		"apps:release:install", "apps:release:upgrade",
		"apps:release:rollback", "apps:release:uninstall",
	}
	for _, code := range wantCodes {
		var n int64
		gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ? AND permissions.code = ?", adminRole.ID, code).
			Count(&n)
		require.Equal(t, int64(1), n, "admin role missing %q", code)
	}
}

// TestRun_ViewerRoleIncludesAppsReadPermissions covers the implicit P3 viewer
// grant: bindViewerPermissions binds every "%:read" code, so the apps read
// permissions flow to viewer automatically.
func TestRun_ViewerRoleIncludesAppsReadPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)

	wantCodes := []string{"apps:repo:read", "apps:application:read"}
	for _, code := range wantCodes {
		var n int64
		gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ? AND permissions.code = ?", viewer.ID, code).
			Count(&n)
		require.Equal(t, int64(1), n, "viewer role missing %q", code)
	}
}

func TestRun_SeedsAssetsMenuTree(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	resourceRead := "assets:resource:read"
	want := []struct {
		code, name, path, component string
		permission                  *string
		sortOrder                   int
	}{
		{"assets", "menu.assets_group", "/assets", "", &resourceRead, 1},
		{"assets.inventory", "menu.assets.inventory", "/assets/inventory", "assets/Inventory", nil, 1},
	}

	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets").First(&parent).Error)
	for i, expected := range want {
		var menu models.Menu
		require.NoError(t, gdb.Where("code = ?", expected.code).First(&menu).Error, "missing menu %q", expected.code)
		require.Equal(t, expected.name, menu.Name)
		require.Equal(t, expected.path, menu.Path)
		require.Equal(t, expected.component, menu.Component)
		require.Equal(t, expected.permission, menu.PermissionCode)
		require.Equal(t, expected.sortOrder, menu.SortOrder)
		if i == 0 {
			require.Nil(t, menu.ParentID)
		} else {
			require.Equal(t, &parent.ID, menu.ParentID)
		}
	}

	var assetsRows int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ? OR code LIKE ?", "assets", "assets.%").Count(&assetsRows).Error)
	require.Equal(t, int64(2), assetsRows)
}

func TestRun_AssetsMenusRemainIdempotent(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	options := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"}
	_, err = seed.Run(context.Background(), gdb, options)
	require.NoError(t, err)
	var inventory models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets.inventory").First(&inventory).Error)
	require.NoError(t, gdb.Model(&inventory).Update("path", "/assets/cloud-accounts").Error)
	_, err = seed.Run(context.Background(), gdb, options)
	require.NoError(t, err)

	var current models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets.inventory").First(&current).Error)
	require.Equal(t, inventory.ID, current.ID)
	require.Equal(t, "/assets/inventory", current.Path)
	require.Nil(t, current.PermissionCode)
	var assetsRows int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ? OR code LIKE ?", "assets", "assets.%").Count(&assetsRows).Error)
	require.Equal(t, int64(2), assetsRows)
}

func TestRun_LegacyFirewallChildrenMoveToInventory(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	legacy := models.Menu{Code: "assets.security_groups", Name: "menu.assets.security_groups", Path: "/assets/security-groups", Component: "assets/security-groups/List"}
	require.NoError(t, gdb.Create(&legacy).Error)
	child := models.Menu{Code: "operator.firewall-child", Name: "custom", ParentID: &legacy.ID}
	require.NoError(t, gdb.Create(&child).Error)
	options := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com", BcryptCost: 4}
	for range 2 {
		_, err = seed.Run(context.Background(), gdb, options)
		require.NoError(t, err)
		var inventory models.Menu
		require.NoError(t, gdb.Where("code = ?", "assets.inventory").First(&inventory).Error)
		var count int64
		require.NoError(t, gdb.Model(&models.Menu{}).Where("code IN ?", []string{"assets.security_groups", "assets.firewalls"}).Count(&count).Error)
		require.Zero(t, count)
		var preserved models.Menu
		require.NoError(t, gdb.First(&preserved, child.ID).Error)
		require.Equal(t, &inventory.ID, preserved.ParentID)
	}
}

func TestRun_AssetsInventoryUpgradePreservesAccountIDAndReparentsChildren(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	opts := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com", BcryptCost: 4}
	_, err = seed.Run(ctx, gdb, opts)
	require.NoError(t, err)
	var parent, product models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets").First(&parent).Error)
	require.NoError(t, gdb.Where("code = ?", "assets.inventory").First(&product).Error)
	require.NoError(t, gdb.Model(&product).Updates(map[string]any{
		"code": "assets.cloud_accounts", "name": "menu.assets.cloud_accounts",
		"path": "/assets/cloud-accounts", "component": "assets/cloud-accounts/List",
		"permission_code": "assets:account:read",
	}).Error)
	var custom []models.Menu
	for _, code := range []string{"assets.vpcs", "assets.firewalls", "assets.instances", "assets.databases", "assets.sync_runs"} {
		old := models.Menu{Code: code, Name: code, ParentID: &parent.ID}
		require.NoError(t, gdb.Create(&old).Error)
		child := models.Menu{Code: "custom." + code, Name: "custom", ParentID: &old.ID}
		require.NoError(t, gdb.Create(&child).Error)
		custom = append(custom, child)
	}
	var grantsBefore []models.RolePermission
	customRole := models.Role{Code: "custom_asset_reader", Name: "Custom asset reader"}
	require.NoError(t, gdb.Create(&customRole).Error)
	var resourcePermission models.Permission
	require.NoError(t, gdb.Where("code = ?", "assets:resource:read").First(&resourcePermission).Error)
	customGrant := models.RolePermission{
		RoleID: customRole.ID, PermissionID: resourcePermission.ID,
		CreatedAt: time.Date(2026, time.November, 18, 0, 0, 0, 123456789, time.UTC),
	}
	require.NoError(t, gdb.Create(&customGrant).Error)
	var persistedGrant models.RolePermission
	require.NoError(t, gdb.Where("role_id = ? AND permission_id = ?", customRole.ID, resourcePermission.ID).First(&persistedGrant).Error)
	require.NoError(t, gdb.Select("role_id, permission_id").Order("role_id, permission_id").Find(&grantsBefore).Error)
	for range 2 {
		_, err = seed.Run(ctx, gdb, opts)
		require.NoError(t, err)
	}
	var current models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets.inventory").First(&current).Error)
	require.Equal(t, product.ID, current.ID)
	require.Equal(t, "/assets/inventory", current.Path)
	require.Equal(t, "assets/Inventory", current.Component)
	require.Nil(t, current.PermissionCode)
	for _, child := range custom {
		var got models.Menu
		require.NoError(t, gdb.First(&got, child.ID).Error)
		require.Equal(t, &product.ID, got.ParentID)
	}
	var oldCount int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code IN ?", []string{
		"assets.cloud_accounts", "assets.vpcs", "assets.firewalls", "assets.instances", "assets.databases", "assets.sync_runs",
	}).Count(&oldCount).Error)
	require.Zero(t, oldCount)
	var grantsAfter []models.RolePermission
	require.NoError(t, gdb.Select("role_id, permission_id").Order("role_id, permission_id").Find(&grantsAfter).Error)
	require.Equal(t, grantsBefore, grantsAfter)
	var retainedGrant models.RolePermission
	require.NoError(t, gdb.Where("role_id = ? AND permission_id = ?", customRole.ID, resourcePermission.ID).First(&retainedGrant).Error)
	require.True(t, persistedGrant.CreatedAt.Equal(retainedGrant.CreatedAt), "custom grants must not be rewritten")
}

func TestRun_AssetsInventoryMenuCollisionRollsBack(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	accounts := models.Menu{Code: "assets.cloud_accounts", Name: "old"}
	require.NoError(t, gdb.Create(&accounts).Error)
	collision := models.Menu{Code: "assets.inventory", Name: "operator-owned"}
	require.NoError(t, gdb.Create(&collision).Error)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", BcryptCost: 4})
	require.ErrorContains(t, err, "assets inventory menu collision")
	var after models.Menu
	require.NoError(t, gdb.First(&after, accounts.ID).Error)
	require.Equal(t, "assets.cloud_accounts", after.Code)
	require.Equal(t, "old", after.Name)
	var owned models.Menu
	require.NoError(t, gdb.First(&owned, collision.ID).Error)
	require.Equal(t, "operator-owned", owned.Name)
	var roles int64
	require.NoError(t, gdb.Model(&models.Role{}).Count(&roles).Error)
	require.Zero(t, roles, "permission and role initialization is atomic with menus")
}

func TestRun_FirewallMenuCollisionRollsBackWithoutDeletingRows(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	role := models.Role{Code: "operator", Name: "operator"}
	require.NoError(t, gdb.Create(&role).Error)
	var permission models.Permission
	require.NoError(t, gdb.Where("code = ?", "assets:resource:read").First(&permission).Error)
	grant := models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}
	require.NoError(t, gdb.Create(&grant).Error)
	for _, code := range []string{"assets.security_groups", "assets.firewalls"} {
		require.NoError(t, gdb.Create(&models.Menu{Code: code, Name: "operator metadata"}).Error)
	}
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", BcryptCost: 4})
	require.ErrorContains(t, err, "assets menu collision")
	var menus []models.Menu
	require.NoError(t, gdb.Where("code IN ?", []string{"assets.security_groups", "assets.firewalls"}).Find(&menus).Error)
	require.Len(t, menus, 2)
	for _, menu := range menus {
		require.Equal(t, "operator metadata", menu.Name)
	}
	var grants []models.RolePermission
	require.NoError(t, gdb.Order("role_id, permission_id").Find(&grants).Error)
	require.Len(t, grants, 1)
	require.Equal(t, role.ID, grants[0].RoleID)
	require.Equal(t, permission.ID, grants[0].PermissionID)
}

func TestRun_AdminAndViewerCoverAssetsPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	roleHasPermission := func(roleCode, permissionCode string) bool {
		var count int64
		require.NoError(t, gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Joins("JOIN roles ON roles.id = role_permissions.role_id").
			Where("roles.code = ? AND permissions.code = ?", roleCode, permissionCode).
			Count(&count).Error)
		return count == 1
	}

	for _, code := range []string{
		"assets:account:read", "assets:account:write", "assets:account:delete",
		"assets:resource:read", "assets:sync:read",
	} {
		require.True(t, roleHasPermission("admin", code), "admin role missing %q", code)
	}
	for _, code := range []string{
		"assets:account:read", "assets:resource:read", "assets:sync:read",
	} {
		require.True(t, roleHasPermission("viewer", code), "viewer role missing %q", code)
	}
	require.False(t, roleHasPermission("viewer", "assets:account:write"))
	require.False(t, roleHasPermission("viewer", "assets:account:delete"))
}
