//go:build dbtest

package k8s_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	"github.com/ArkGravity/optimus/internal/modules/credentials/kubeconfig"
	"github.com/ArkGravity/optimus/internal/modules/credentials/vault"
	"github.com/ArkGravity/optimus/internal/modules/k8s"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
	"github.com/ArkGravity/optimus/internal/seed"
)

const actorBridgeKubeconfig = `apiVersion: v1
kind: Config
clusters:
  - name: smoke
    cluster:
      server: http://127.0.0.1:1
users:
  - name: smoke
    user:
      token: smoke
contexts:
  - name: smoke
    context:
      cluster: smoke
      user: smoke
current-context: smoke
`

func TestMountRoutes_PingPropagatesActorToRealCredentialConsumer(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)

	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{
		AdminUsername: "admin",
		AdminEmail:    "admin@example.test",
		BcryptCost:    4,
	})
	require.NoError(t, err)
	var admin models.User
	require.NoError(t, gdb.Where("username = ?", "admin").First(&admin).Error)

	cipher, err := vault.NewCipher(make([]byte, vault.KeyLen))
	require.NoError(t, err)
	sealed, err := cipher.Seal([]byte(actorBridgeKubeconfig))
	require.NoError(t, err)
	kc := &models.CredentialKubeconfig{Name: "actor-bridge", KubeconfigEnc: sealed}
	require.NoError(t, gdb.Create(kc).Error)
	clusterRow := &models.Cluster{Name: "actor-bridge", KubeconfigID: kc.ID, Context: "smoke"}
	require.NoError(t, gdb.Create(clusterRow).Error)

	recorder := audit.NewRecorder(gdb)
	kcService := kubeconfig.NewService(kubeconfig.NewRepo(gdb), cipher, recorder)
	consumer := credentials.NewConsumer(nil, kcService, nil)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	module := k8s.New(gdb, consumer, recorder, cache)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, admin.ID)
		c.Next()
	})
	module.MountRoutes(router.Group("/api/v1"), cache)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/k8s/clusters/"+strconv.FormatUint(clusterRow.ID, 10)+"/ping", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
	require.False(t, envelope.Data.OK)
	require.NotContains(t, envelope.Data.Message, "system caller purpose")

	var consumeAudit models.AuditLog
	require.NoError(t, gdb.Where("action = ?", "credentials.consume").Order("id DESC").First(&consumeAudit).Error)
	require.Equal(t, &admin.ID, consumeAudit.UserID)
}

func TestMountRoutes_StoragePermissionAndCredentialActor(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.test", BcryptCost: 4})
	require.NoError(t, err)
	var admin models.User
	var role models.Role
	var permission models.Permission
	require.NoError(t, gdb.Where("username = ?", "admin").First(&admin).Error)
	require.NoError(t, gdb.Where("code = ?", "admin").First(&role).Error)
	require.NoError(t, gdb.Where("code = ?", "k8s:storage:read").First(&permission).Error)
	require.NoError(t, gdb.Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).Delete(&models.RolePermission{}).Error)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/namespaces/prod/persistentvolumeclaims", r.URL.Path)
		require.Equal(t, "500", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaimList","items":[{"metadata":{"name":"data","namespace":"prod"},"status":{"phase":"Pending"}}]}`))
	}))
	defer upstream.Close()
	cipher, err := vault.NewCipher(make([]byte, vault.KeyLen))
	require.NoError(t, err)
	sealed, err := cipher.Seal([]byte(strings.ReplaceAll(actorBridgeKubeconfig, "http://127.0.0.1:1", upstream.URL)))
	require.NoError(t, err)
	kc := models.CredentialKubeconfig{Name: "storage-actor", KubeconfigEnc: sealed}
	require.NoError(t, gdb.Create(&kc).Error)
	clusterRow := models.Cluster{Name: "storage-actor", KubeconfigID: kc.ID, Context: "smoke"}
	require.NoError(t, gdb.Create(&clusterRow).Error)
	recorder := audit.NewRecorder(gdb)
	consumer := credentials.NewConsumer(nil, kubeconfig.NewService(kubeconfig.NewRepo(gdb), cipher, recorder), nil)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	module := k8s.New(gdb, consumer, recorder, cache)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, admin.ID); c.Next() })
	module.MountRoutes(router.Group("/api/v1"), cache)
	path := "/api/v1/k8s/clusters/" + strconv.FormatUint(clusterRow.ID, 10) + "/storage/persistentvolumeclaims?namespace=prod"
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	require.Zero(t, upstreamCalls.Load(), "permission must be checked before reading the cluster")
	var audits int64
	require.NoError(t, gdb.Model(&models.AuditLog{}).Where("action = ?", "credentials.consume").Count(&audits).Error)
	require.Zero(t, audits)
	require.NoError(t, gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error)
	cache.InvalidateUser(admin.ID)
	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	require.Contains(t, allowed.Body.String(), `"name":"data"`)
	require.Equal(t, int32(1), upstreamCalls.Load())
	var consumeAudit models.AuditLog
	require.NoError(t, gdb.Where("action = ?", "credentials.consume").First(&consumeAudit).Error)
	require.Equal(t, &admin.ID, consumeAudit.UserID)
}
