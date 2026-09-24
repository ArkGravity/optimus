package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/middleware"
)

func TestSecurityHeaders_AppliedToRoutesAndNoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.SecurityHeaders())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, target := range []string{"/ok", "/missing"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), target)
		require.Equal(t, "SAMEORIGIN", rec.Header().Get("X-Frame-Options"), target)
		require.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"), target)
	}
}
