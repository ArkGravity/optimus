//go:build dbtest

package cloudkey_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
	"github.com/ArkGravity/optimus/internal/seed"
)

func newHandlerRouter(t *testing.T) *gin.Engine {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	h := cloudkey.NewHandler(cloudkey.NewService(cloudkey.NewRepo(gdb), passthroughCipher{}, audit.NewRecorder(gdb)))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/api/v1/credentials/cloud-keys")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_CreateAndNoSecretLeak(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", cloudkey.CreateRequest{
		Name: "h-aws", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "verysecret123",
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("verysecret123")),
		"response leaks plaintext secret_access_key")
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("AKIAEXAMPLE")),
		"response leaks plaintext access_key_id")

	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	// List shouldn't leak either.
	w = doJSON(t, r, "GET", "/api/v1/credentials/cloud-keys?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("verysecret123")))
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("AKIAEXAMPLE")))

	// Get
	w = doJSON(t, r, "GET", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"provider":"aws"`)

	// Delete
	w = doJSON(t, r, "DELETE", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_RejectsBadProvider(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", map[string]any{
		"name":              "ibm-key",
		"provider":          "ibm",
		"access_key_id":     "key",
		"secret_access_key": "sec",
	})
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), "common.validation")
}

func TestHandler_Update_RotateSecret(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", cloudkey.CreateRequest{
		Name: "h-u", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AK1", SecretAccessKey: "S1",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	newSecret := "S2"
	w = doJSON(t, r, "PUT", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10),
		cloudkey.UpdateRequest{SecretAccessKey: &newSecret})
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("S2")),
		"PUT response leaks the new secret")
}

func TestHandler_CloudKeyProviderContract(t *testing.T) {
	router := newHandlerRouter(t)
	var aliyunID uint64
	for _, provider := range []string{"aliyun", "aws", "gcp", "azure", "ibm"} {
		t.Run(provider, func(t *testing.T) {
			request := newReq()
			request.Name = provider + "-key"
			request.Provider = provider
			response := doJSON(t, router, http.MethodPost, "/api/v1/credentials/cloud-keys", request)
			if provider == "azure" || provider == "ibm" {
				require.Equal(t, http.StatusBadRequest, response.Code)
				require.Contains(t, response.Body.String(), "common.validation")
				return
			}
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var result struct {
				Data cloudkey.Detail `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.Equal(t, provider, result.Data.Provider)
			if provider == "aliyun" {
				aliyunID = result.Data.ID
			}
		})
	}
	response := doJSON(t, router, http.MethodGet, "/api/v1/credentials/cloud-keys?provider=aliyun", nil)
	require.Equal(t, http.StatusOK, response.Code)
	var list struct {
		Data cloudkey.ListResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list))
	require.Equal(t, int64(1), list.Data.Total)
	require.Len(t, list.Data.Items, 1)
	path := "/api/v1/credentials/cloud-keys/" + strconv.FormatUint(aliyunID, 10)
	response = doJSON(t, router, http.MethodPut, path, map[string]any{"provider": "gcp"})
	require.Equal(t, http.StatusOK, response.Code)
	response = doJSON(t, router, http.MethodPut, path, map[string]any{"provider": "aliyun"})
	require.Equal(t, http.StatusOK, response.Code)
	response = doJSON(t, router, http.MethodPut, path, map[string]any{"provider": "azure"})
	require.Equal(t, http.StatusBadRequest, response.Code)
	response = doJSON(t, router, http.MethodGet, path, nil)
	require.Contains(t, response.Body.String(), `"provider":"aliyun"`)
}

func TestHandler_PreservesLegacyAzureCredentialMetadata(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	legacy := &models.CredentialCloudKey{
		Name: "legacy-azure", Provider: "azure", AccessKeyIDEnc: []byte("SEALold-ak"), SecretAccessKeyEnc: []byte("SEALold-sk"),
	}
	require.NoError(t, db.Create(legacy).Error)
	router := gin.New()
	handler := cloudkey.NewHandler(svc)
	router.PUT("/cloud-keys/:id", handler.HandleUpdate())
	response := doJSON(t, router, http.MethodPut, "/cloud-keys/"+strconv.FormatUint(legacy.ID, 10), map[string]any{"description": "retained"})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"provider":"azure"`)
	require.NoError(t, db.First(legacy, legacy.ID).Error)
	require.Equal(t, []byte("SEALold-ak"), legacy.AccessKeyIDEnc)
	require.Equal(t, []byte("SEALold-sk"), legacy.SecretAccessKeyEnc)
}
