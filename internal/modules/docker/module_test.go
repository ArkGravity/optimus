//go:build dbtest

package docker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/credentials/vault"
	"github.com/ArkGravity/optimus/internal/modules/docker"
	"github.com/ArkGravity/optimus/internal/modules/docker/container"
	"github.com/ArkGravity/optimus/internal/modules/docker/image"
	"github.com/ArkGravity/optimus/internal/modules/docker/internal/enginetest"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
	"github.com/ArkGravity/optimus/internal/seed"
)

const imageID = "sha256:4f67c83422ec747235357c04556616234e66fc3fa39cb4f40b2d4441ddd8f100"

func setup(t *testing.T) (*gin.Engine, *audit.Recorder, uint64, uint64, func(string) []models.AuditLog) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.test", BcryptCost: 4})
	require.NoError(t, err)

	var admin models.User
	require.NoError(t, gdb.Where("username = ?", "admin").First(&admin).Error)
	var viewerRole models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewerRole).Error)
	viewer := &models.User{Username: "viewer", Email: "viewer@example.test", PasswordHash: "x"}
	require.NoError(t, gdb.Create(viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: viewer.ID, RoleID: viewerRole.ID}).Error)

	cipher, err := vault.NewCipher(make([]byte, vault.KeyLen))
	require.NoError(t, err)
	rec := audit.NewRecorder(gdb)
	mod := docker.New(gdb, cipher, credentials.New(gdb, cipher, rec).Consumer, rec)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		uid, _ := strconv.ParseUint(c.GetHeader("X-Test-User"), 10, 64)
		c.Set(middleware.CtxKeyUserID, uid)
		c.Next()
	})
	mod.MountRoutes(r.Group("/api/v1"), rbac.NewPermissionCache(gdb, time.Minute))

	logs := func(action string) []models.AuditLog {
		var out []models.AuditLog
		require.NoError(t, gdb.Where("action = ?", action).Order("id").Find(&out).Error)
		return out
	}
	return r, rec, admin.ID, viewer.ID, logs
}

func TestMountRoutes_LiveRoutesAreRegisteredAndGated(t *testing.T) {
	r, _, adminID, viewerID, _ := setup(t)

	routes := []struct {
		method, path string
		write        bool
	}{
		{http.MethodGet, "/api/v1/docker/hosts/999/images", false},
		{http.MethodDelete, "/api/v1/docker/hosts/999/images/" + imageID, true},
		{http.MethodPost, "/api/v1/docker/hosts/999/images/prune", true},
		{http.MethodGet, "/api/v1/docker/hosts/999/containers", false},
		{http.MethodGet, "/api/v1/docker/hosts/999/containers/web", false},
		{http.MethodPost, "/api/v1/docker/hosts/999/containers/web/start", true},
		{http.MethodPost, "/api/v1/docker/hosts/999/containers/web/stop", true},
		{http.MethodPost, "/api/v1/docker/hosts/999/containers/web/restart", true},
		{http.MethodDelete, "/api/v1/docker/hosts/999/containers/web", true},
	}
	call := func(uid uint64, method, path string) (int, int) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Test-User", strconv.FormatUint(uid, 10))
		r.ServeHTTP(w, req)
		var env struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
		return w.Code, env.Code
	}
	for _, rt := range routes {
		// Passing the gate reaches the host lookup, which misses for id 999.
		status, code := call(adminID, rt.method, rt.path)
		require.Equal(t, http.StatusNotFound, status, "admin %s %s", rt.method, rt.path)
		require.Equal(t, 40401, code)

		status, code = call(viewerID, rt.method, rt.path)
		if rt.write {
			require.Equal(t, http.StatusForbidden, status, "viewer %s %s", rt.method, rt.path)
			require.Equal(t, 40302, code)
		} else {
			require.Equal(t, http.StatusNotFound, status, "viewer %s %s", rt.method, rt.path)
			require.Equal(t, 40401, code)
		}
	}
}

func TestLiveMutationsAreAudited(t *testing.T) {
	_, rec, adminID, _, logs := setup(t)
	ctx := context.Background()

	e := enginetest.New(t)
	e.Reply(http.MethodDelete, "/images/"+imageID, http.StatusOK, []map[string]string{{"Deleted": imageID}})
	e.Reply(http.MethodPost, "/images/prune", http.StatusOK, map[string]any{"SpaceReclaimed": 512})
	e.Reply(http.MethodPost, "/containers/web/stop", http.StatusNoContent, nil)
	e.Reply(http.MethodDelete, "/containers/web", http.StatusConflict, "container is running")

	images := image.NewService(e.Connector(), rec)
	containers := container.NewService(e.Connector(), rec)
	_, err := images.Remove(ctx, adminID, "10.0.0.1", "test-agent", 7, imageID, false)
	require.NoError(t, err)
	_, err = images.Prune(ctx, adminID, "10.0.0.1", "test-agent", 7, true)
	require.NoError(t, err)
	require.NoError(t, containers.Do(ctx, adminID, "10.0.0.1", "test-agent", 7, "web", container.ActionStop))
	require.Error(t, containers.Remove(ctx, adminID, "10.0.0.1", "test-agent", 7, "web", false))

	check := func(action string, want map[string]any) {
		rows := logs(action)
		require.Len(t, rows, 1, action)
		row := rows[0]
		require.Equal(t, "docker.host", row.TargetType)
		require.Equal(t, "7", row.TargetID)
		require.NotNil(t, row.UserID)
		require.Equal(t, adminID, *row.UserID)
		require.Equal(t, "10.0.0.1", row.IP)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(row.Payload, &payload))
		for k, v := range want {
			require.Equal(t, v, payload[k], "%s payload %s", action, k)
		}
	}
	check("docker.image.delete", map[string]any{"image_id": imageID, "host_name": "edge-1", "force": false, "deleted": float64(1)})
	check("docker.image.prune", map[string]any{"all": true, "space_reclaimed": float64(512)})
	check("docker.container.stop", map[string]any{"container": "web", "host_name": "edge-1"})
	require.Empty(t, logs("docker.container.delete"), "failed mutations are not audited")
}
