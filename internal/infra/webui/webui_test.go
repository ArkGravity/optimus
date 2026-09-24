package webui_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/logic3579/optimus/internal/infra/webui"
)

const indexHTML = `<!doctype html><div id="app"></div>`

var bundle = strings.Repeat("console.log('optimus');\n", 200)

func distFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(indexHTML)},
		"optimus-logo.png":     {Data: []byte("\x89PNG\r\n\x1a\n")},
		"assets/index-abc.js":  {Data: []byte(bundle)},
		"assets/index-abc.css": {Data: []byte("body{}")},
		".gitkeep":             {Data: nil},
	}
}

func newRouter(t *testing.T, h *webui.Handler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.NoRoute(h.Handle)
	return r
}

func do(r http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func embedded(t *testing.T) *gin.Engine {
	t.Helper()
	h, err := webui.New("", distFS())
	require.NoError(t, err)
	require.True(t, h.Built())
	return newRouter(t, h)
}

func TestEmbedded_ServesIndexAndSPAFallback(t *testing.T) {
	r := embedded(t)
	for _, target := range []string{"/", "/index.html", "/system/users", "/k8s/pods/detail", "/.gitkeep"} {
		rec := do(r, http.MethodGet, target, nil)
		require.Equal(t, http.StatusOK, rec.Code, target)
		require.Equal(t, indexHTML, rec.Body.String(), target)
		require.Equal(t, "no-cache, no-store, must-revalidate", rec.Header().Get("Cache-Control"), target)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html", target)
	}
}

func TestEmbedded_HashedAssetsAreImmutableAndGzipped(t *testing.T) {
	r := embedded(t)

	plain := do(r, http.MethodGet, "/assets/index-abc.js", nil)
	require.Equal(t, http.StatusOK, plain.Code)
	require.Equal(t, bundle, plain.Body.String())
	require.Equal(t, "public, max-age=31536000, immutable", plain.Header().Get("Cache-Control"))
	require.Equal(t, "Accept-Encoding", plain.Header().Get("Vary"))
	require.Empty(t, plain.Header().Get("Content-Encoding"))

	gz := do(r, http.MethodGet, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "br, gzip"})
	require.Equal(t, "gzip", gz.Header().Get("Content-Encoding"))
	zr, err := gzip.NewReader(bytes.NewReader(gz.Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.Equal(t, bundle, string(decoded))

	refused := do(r, http.MethodGet, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "gzip;q=0"})
	require.Empty(t, refused.Header().Get("Content-Encoding"))

	small := do(r, http.MethodGet, "/assets/index-abc.css", map[string]string{"Accept-Encoding": "gzip"})
	require.Empty(t, small.Header().Get("Content-Encoding"), "assets below the size threshold stay uncompressed")
}

func TestEmbedded_MissingHashedAssetDoesNotFallBack(t *testing.T) {
	rec := do(embedded(t), http.MethodGet, "/assets/stale-123.js", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestEmbedded_ConditionalAndHeadRequests(t *testing.T) {
	r := embedded(t)
	first := do(r, http.MethodGet, "/optimus-logo.png", nil)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "no-cache", first.Header().Get("Cache-Control"))
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	cached := do(r, http.MethodGet, "/optimus-logo.png", map[string]string{"If-None-Match": etag})
	require.Equal(t, http.StatusNotModified, cached.Code)
	require.Empty(t, cached.Body.String())

	head := do(r, http.MethodHead, "/optimus-logo.png", nil)
	require.Equal(t, http.StatusOK, head.Code)
	require.Empty(t, head.Body.String())
	require.Equal(t, first.Header().Get("Content-Length"), head.Header().Get("Content-Length"))
}

func TestBackendPathsReturnJSONNotFound(t *testing.T) {
	r := embedded(t)
	for _, target := range []string{"/api", "/api/v1/unknown", "/swagger"} {
		rec := do(r, http.MethodGet, target, nil)
		require.Equal(t, http.StatusNotFound, rec.Code, target)
		var body struct {
			Code       int    `json:"code"`
			MessageKey string `json:"message_key"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), target)
		require.Equal(t, 40401, body.Code, target)
		require.Equal(t, "common.not_found", body.MessageKey, target)
	}
	require.Equal(t, http.StatusOK, do(r, http.MethodGet, "/api/v1/health", nil).Code)
}

func TestNonReadMethodsAreRejectedOutsideAPI(t *testing.T) {
	rec := do(embedded(t), http.MethodPost, "/system/users", nil)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
}

func TestEmbedded_NotBuilt(t *testing.T) {
	h, err := webui.New("", fstest.MapFS{".gitkeep": {Data: nil}})
	require.NoError(t, err)
	require.False(t, h.Built())
	rec := do(newRouter(t, h), http.MethodGet, "/", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "web UI is not built")
}

func TestWebDir_ServesFromDiskWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	_, err := webui.New(dir, nil)
	require.Error(t, err, "a web_dir without index.html is rejected at startup")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
	h, err := webui.New(dir, nil)
	require.NoError(t, err)
	r := newRouter(t, h)

	require.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/assets/app.js", nil).Code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte(bundle), 0o600))
	rec := do(r, http.MethodGet, "/assets/app.js", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, bundle, rec.Body.String())
	require.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))

	spa := do(r, http.MethodGet, "/system/users", nil)
	require.Equal(t, http.StatusOK, spa.Code)
	require.Equal(t, indexHTML, spa.Body.String())
}
