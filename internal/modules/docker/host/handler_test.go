package host_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/modules/docker/host"
)

func TestHandler_RejectsBadInputBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := host.NewHandler(nil)
	r := gin.New()
	r.GET("/hosts/:id", h.HandleGet())
	r.POST("/hosts/:id/test", h.HandleTest())
	r.POST("/hosts/probe", h.HandleProbe())
	r.POST("/hosts", h.HandleCreate())

	cases := []struct {
		method, path, body string
		status, code       int
	}{
		{http.MethodGet, "/hosts/abc", "", http.StatusBadRequest, 40001},
		{http.MethodPost, "/hosts/0/test", "", http.StatusBadRequest, 40001},
		{http.MethodPost, "/hosts/probe", `{"host":"h","auth_type":"agent"}`, http.StatusBadRequest, 40002},
		{http.MethodPost, "/hosts", `{"name":"a","host":"h","auth_type":"agent"}`, http.StatusBadRequest, 40002},
		{http.MethodPost, "/hosts", `{"name":"a","host":"h","auth_type":"password","port":70000}`, http.StatusBadRequest, 40002},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		require.Equal(t, tc.status, w.Code, tc.path)
		var env struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		require.Equal(t, tc.code, env.Code, tc.path)
	}
}
