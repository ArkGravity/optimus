//go:build dbtest

package k8s_test

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

	"github.com/logic3579/optimus/internal/infra/db"
	"github.com/logic3579/optimus/internal/infra/middleware"
	"github.com/logic3579/optimus/internal/infra/permissions"
	"github.com/logic3579/optimus/internal/models"
	"github.com/logic3579/optimus/internal/modules/audit"
	"github.com/logic3579/optimus/internal/modules/credentials"
	"github.com/logic3579/optimus/internal/modules/credentials/kubeconfig"
	"github.com/logic3579/optimus/internal/modules/credentials/vault"
	"github.com/logic3579/optimus/internal/modules/k8s"
	"github.com/logic3579/optimus/internal/modules/rbac"
	"github.com/logic3579/optimus/internal/seed"
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
	consumer := credentials.NewConsumer(nil, kcService, nil, nil)
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
