//go:build dbtest

package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type catalogFunc func(context.Context, *credentials.CloudKey, string) ([]RegionOption, error)

func (f catalogFunc) Regions(ctx context.Context, key *credentials.CloudKey, project string) ([]RegionOption, error) {
	return f(ctx, key, project)
}

type catalogCipher struct{}

func (catalogCipher) Seal(p []byte) ([]byte, error) { return append([]byte(nil), p...), nil }
func (catalogCipher) Open(p []byte) ([]byte, error) { return append([]byte(nil), p...), nil }

func TestRegionOptionsGCPUsesExplicitTargetAndAuditsActor(t *testing.T) {
	svc, repo, _, key := setupSvc(t)
	require.NoError(t, repo.DB().Model(key).Updates(map[string]any{"provider": "gcp", "auth_type": "service_account_json", "access_key_id_enc": nil, "secret_access_key_enc": nil, "service_account_json_enc": []byte(`{"project_id":"credential-home"}`), "service_account_email": "collector@example.test", "credential_project_id": "credential-home"}).Error)
	actor := dbtest.SeedUser(t, repo.DB(), "catalog-actor")
	svc.consumer = credentials.NewConsumer(nil, nil, cloudkey.NewService(cloudkey.NewRepo(repo.DB()), catalogCipher{}, audit.NewRecorder(repo.DB())))
	var retained []byte
	svc.SetRegionCatalogs(map[string]RegionCatalog{"gcp": catalogFunc(func(_ context.Context, consumed *credentials.CloudKey, project string) ([]RegionOption, error) {
		require.Equal(t, "target-project", project)
		require.Equal(t, "gcp", consumed.Provider)
		retained = consumed.ServiceAccountJSON
		return []RegionOption{{ID: "us-central1", Name: "Iowa"}}, nil
	})})
	got, err := svc.RegionOptions(context.Background(), actor.ID, RegionOptionsQuery{Provider: "gcp", CloudKeyID: key.ID, ProjectID: "target-project"})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.NotEmpty(t, retained)
	for _, b := range retained {
		require.Zero(t, b)
	}
	var event models.AuditLog
	require.NoError(t, repo.DB().Where("action = ?", "credentials.consume").First(&event).Error)
	require.NotNil(t, event.UserID)
	require.Equal(t, actor.ID, *event.UserID)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Payload, &payload))
	require.Equal(t, "assets.region_options", payload["purpose"])
}

type keyConsumerFunc func(context.Context, uint64, string) (*credentials.CloudKey, error)

func (f keyConsumerFunc) GetCloudKey(ctx context.Context, id uint64, purpose string) (*credentials.CloudKey, error) {
	return f(ctx, id, purpose)
}

func TestRegionOptionsConsumesPurposeWipesAndBoundsContext(t *testing.T) {
	svc, _, _, key := setupSvc(t)
	material := &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "secret-id", SecretAccessKey: "secret-value"}
	svc.consumer = keyConsumerFunc(func(_ context.Context, id uint64, purpose string) (*credentials.CloudKey, error) {
		require.Equal(t, key.ID, id)
		require.Equal(t, "assets.region_options", purpose)
		return material, nil
	})
	svc.SetRegionCatalogs(map[string]RegionCatalog{"aws": catalogFunc(func(ctx context.Context, consumed *credentials.CloudKey, project string) ([]RegionOption, error) {
		require.Same(t, material, consumed)
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, time.Second)
		require.Empty(t, project)
		return []RegionOption{{ID: "us-east-1", Name: "US East"}}, nil
	})})
	got, err := svc.RegionOptions(context.Background(), 7, RegionOptionsQuery{Provider: "aws", CloudKeyID: key.ID})
	require.NoError(t, err)
	require.Equal(t, "us-east-1", got.Items[0].ID)
	require.Empty(t, material.AccessKeyID)
	require.Empty(t, material.SecretAccessKey)
}

func TestRegionOptionsSanitizesErrorsAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "cancelled"}[cancelled], func(t *testing.T) {
			svc, _, _, key := setupSvc(t)
			material := &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "secret-id", SecretAccessKey: "secret-value"}
			svc.consumer = keyConsumerFunc(func(context.Context, uint64, string) (*credentials.CloudKey, error) { return material, nil })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			svc.SetRegionCatalogs(map[string]RegionCatalog{"aws": catalogFunc(func(ctx context.Context, _ *credentials.CloudKey, _ string) ([]RegionOption, error) {
				if cancelled {
					cancel()
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return nil, errors.New("secret-value upstream URL")
			})})
			_, err := svc.RegionOptions(ctx, 7, RegionOptionsQuery{Provider: "aws", CloudKeyID: key.ID})
			require.Error(t, err)
			be, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.NotContains(t, be.Message, "secret-value")
			if cancelled {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Empty(t, material.SecretAccessKey)
		})
	}
}

func TestRegionOptionsUnconfiguredProviderFailsSafely(t *testing.T) {
	svc, _, _, key := setupSvc(t)
	_, err := svc.RegionOptions(context.Background(), 7, RegionOptionsQuery{Provider: "aws", CloudKeyID: key.ID})
	requireBizCode(t, err, errs.CodeAssetsConfig)
}

func TestHandler_RegionOptionsPermission(t *testing.T) {
	svc, _, _, key := setupSvc(t)
	calls := 0
	svc.consumer = keyConsumerFunc(func(context.Context, uint64, string) (*credentials.CloudKey, error) {
		calls++
		return &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "id", SecretAccessKey: "secret"}, nil
	})
	svc.SetRegionCatalogs(map[string]RegionCatalog{"aws": catalogFunc(func(context.Context, *credentials.CloudKey, string) ([]RegionOption, error) {
		return []RegionOption{{ID: "us-east-1", Name: "US East"}}, nil
	})})
	router := gin.New()
	group := router.Group("/api/v1/assets", func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "common.unauthorized", "authentication required"))
			c.Abort()
			return
		}
		c.Set(middleware.CtxKeyUserID, uint64(7))
		c.Next()
	})
	NewHandler(svc).Mount(group, func(permission string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if permission != "assets:account:write" || c.GetHeader("X-Allow") != "write" {
				response.Error(c, apperr.New(apperr.CodeForbidden, "common.forbidden", "permission denied"))
				c.Abort()
				return
			}
			c.Next()
		}
	})
	path := "/api/v1/assets/cloud-accounts/region-options?provider=aws&cloudkey_id=" + strconv.FormatUint(key.ID, 10)
	for _, tc := range []struct {
		auth, allow string
		status      int
	}{{"", "", 401}, {"Bearer actor", "read", 403}, {"Bearer actor", "write", 200}} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", tc.auth)
		req.Header.Set("X-Allow", tc.allow)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, tc.status, rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	require.Equal(t, 1, calls)
}
