package database

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDetailRoutesProtectValidateAndSeparateProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &keyRepo{}
	router := gin.New()
	protected := router.Group("/assets", func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthorized", "unauthorized"))
			c.Abort()
			return
		}
		c.Next()
	})
	NewHandler(NewService(repo)).Mount(protected, func(code string) gin.HandlerFunc {
		require.Equal(t, "assets:resource:read", code)
		return func(c *gin.Context) {
			if c.GetHeader("Authorization") != "allowed" {
				response.Error(c, apperr.New(apperr.CodeForbidden, "common.forbidden", "forbidden"))
				c.Abort()
				return
			}
			c.Next()
		}
	})
	request := func(path, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		router.ServeHTTP(r, req)
		return r
	}
	for _, auth := range []string{"", "denied"} {
		rec := request("/assets/databases/gcp:database:42", auth)
		want := http.StatusUnauthorized
		if auth != "" {
			want = http.StatusForbidden
		}
		require.Equal(t, want, rec.Code, rec.Body.String())
		require.Zero(t, repo.calls)
	}
	for _, key := range []string{"gcp:database:42", "aws:database:42", "42"} {
		repo.row = &Summary{AssetKey: key, Provider: "gcp", NativeID: "native", ProjectID: "project"}
		rec := request("/assets/databases/"+key+"?include_deleted=true", "allowed")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, key, repo.key)
		require.True(t, repo.deleted)
		require.Contains(t, rec.Body.String(), `"native_id":"native"`)
		require.Contains(t, rec.Body.String(), `"code":0`)
	}
	calls := repo.calls
	for _, key := range []string{"gcp:wrong:42", "azure:database:42", "0", "-1", "gcp:database:9223372036854775808", "gcp:database:42?include_deleted=no"} {
		rec := request("/assets/databases/"+key, "allowed")
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), `"message_key":"common.validation"`)
	}
	require.Equal(t, calls, repo.calls)
	repo.findError = errors.New("password=never-leak")
	rec := request("/assets/databases/gcp:database:42", "allowed")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "never-leak")
}
