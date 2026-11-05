package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/infra/webui"
)

func swaggerTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	ui, err := webui.New("", fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>Optimus SPA</title>")},
	})
	require.NoError(t, err)
	r := gin.New()
	r.NoRoute(ui.Handle)
	mountSwaggerRoutes(r)
	return r
}

func swaggerRequest(r http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestSwaggerUnknownRoutesReturnJSONEnvelope(t *testing.T) {
	r := swaggerTestRouter(t)
	for _, target := range []string{
		"/swagger/not-a-route", "/swagger/", "/swagger/api/v1/instances",
		"/swagger/doc.json/typo", "/swagger/index.html.bak", "/swagger/nested/index.html",
		"/swagger/missing.css", "/swagger/not-a-route?file=index.html",
		"/swagger/swagger-ui-es-bundle.js", "/swagger/swagger-initializer.js",
		"/swaggerish", "/api/v1/not-a-route",
	} {
		t.Run(target, func(t *testing.T) {
			w := swaggerRequest(r, http.MethodGet, target)
			require.Equal(t, http.StatusNotFound, w.Code)
			require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			var body response.Envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, 40401, body.Code)
			require.Nil(t, body.Data)
			require.Equal(t, "common.not_found", body.MessageKey)
			require.Equal(t, "route not found", body.Message)
		})
	}
}

func TestSwaggerKnownResourcesRemainAvailable(t *testing.T) {
	r := swaggerTestRouter(t)
	for _, tc := range []struct {
		name        string
		contentType string
	}{
		{"index.html", "text/html"},
		{"doc.json", "application/json"},
		{"swagger-ui.css", "text/css"},
		{"swagger-ui.css.map", "text/css"},
		{"swagger-ui.js", "application/javascript"},
		{"swagger-ui.js.map", "application/javascript"},
		{"swagger-ui-bundle.js", "application/javascript"},
		{"swagger-ui-bundle.js.map", "application/javascript"},
		{"swagger-ui-standalone-preset.js", "application/javascript"},
		{"swagger-ui-standalone-preset.js.map", "application/javascript"},
		{"favicon-16x16.png", "image/png"},
		{"favicon-32x32.png", "image/png"},
		{"oauth2-redirect.html", "text/html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, query := range []string{"", "?cache=123", "?next=index.html&other=doc.json"} {
				w := swaggerRequest(r, http.MethodGet, "/swagger/"+tc.name+query)
				require.Equal(t, http.StatusOK, w.Code, query)
				require.Contains(t, w.Header().Get("Content-Type"), tc.contentType, query)
				require.NotEmpty(t, w.Body.Bytes(), query)
				if tc.name == "doc.json" {
					var spec map[string]any
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &spec))
					require.Equal(t, "2.0", spec["swagger"])
					require.Equal(t, "/api/v1", spec["basePath"])
					require.NotEmpty(t, spec["paths"])
				}
				if tc.name == "index.html" {
					require.Contains(t, w.Body.String(), "Swagger UI")
					require.NotContains(t, w.Body.String(), "Optimus SPA")
				}
			}
		})
	}
}

func TestSwaggerCompositionPreservesRedirectMethodsAndSPA(t *testing.T) {
	r := swaggerTestRouter(t)
	w := swaggerRequest(r, http.MethodGet, "/swagger?cache=123")
	require.Equal(t, http.StatusMovedPermanently, w.Code)
	require.Equal(t, "/swagger/?cache=123", w.Header().Get("Location"))
	for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodOptions} {
		w := swaggerRequest(r, method, "/swagger/index.html")
		require.Equal(t, http.StatusNotFound, w.Code, method)
		require.Contains(t, w.Header().Get("Content-Type"), "application/json", method)
	}
	w = swaggerRequest(r, http.MethodGet, "/dashboard")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "Optimus SPA")
}

func TestSwaggerQueryCannotPoisonLaterResourcesOrMiddleware(t *testing.T) {
	r := gin.New()
	var requestURI, rawQuery string
	r.Use(func(c *gin.Context) {
		c.Next()
		requestURI = c.Request.RequestURI
		rawQuery = c.Request.URL.RawQuery
	})
	mountSwaggerRoutes(r)

	const query = "next=index.html&other=doc.json"
	w := swaggerRequest(r, http.MethodGet, "/swagger/swagger-ui.css?"+query)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/css")
	assert.Equal(t, "/swagger/swagger-ui.css?"+query, requestURI)
	assert.Equal(t, query, rawQuery)

	w = swaggerRequest(r, http.MethodGet, "/swagger/swagger-ui-bundle.js")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
	assert.NotEmpty(t, w.Body.Bytes())
}
