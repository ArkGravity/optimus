package webui_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
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

	"github.com/ArkGravity/optimus/internal/infra/webui"
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

func assetPagePaths() []string {
	paths := []string{
		"/assets",
		"/assets/cloud-accounts", "/assets/vpcs", "/assets/firewalls",
		"/assets/instances", "/assets/databases", "/assets/sync-runs",
		"/assets/security-groups",
		"/assets/vpcs/gcp%3Avpc%3A42", "/assets/instances/aws%3Ainstance%3A9223372036854775807",
	}
	for _, family := range []string{"instance", "vpc", "database", "firewall"} {
		for _, id := range []string{"1", "9223372036854775807"} {
			paths = append(paths, fmt.Sprintf("/assets/%ss/%s", family, id))
			for _, provider := range []string{"aws", "aliyun", "gcp"} {
				paths = append(paths, fmt.Sprintf("/assets/%ss/%s:%s:%s", family, provider, family, id))
			}
		}
	}
	return paths
}

func TestAssetsPageReusesIndexGzipAndETag(t *testing.T) {
	dist := distFS()
	largeIndex := strings.Repeat(indexHTML, 100)
	dist["index.html"] = &fstest.MapFile{Data: []byte(largeIndex)}
	h, err := webui.New("", dist)
	require.NoError(t, err)
	r := newRouter(t, h)
	headers := map[string]string{"Accept-Encoding": "gzip"}
	index := do(r, http.MethodGet, "/", headers)
	page := do(r, http.MethodGet, "/assets/vpcs/gcp%3Avpc%3A42", headers)
	require.Equal(t, http.StatusOK, page.Code)
	require.Equal(t, "gzip", page.Header().Get("Content-Encoding"))
	require.Equal(t, "no-cache, no-store, must-revalidate", page.Header().Get("Cache-Control"))
	require.Equal(t, index.Header(), page.Header())
	require.Equal(t, index.Body.Bytes(), page.Body.Bytes())
	zr, err := gzip.NewReader(bytes.NewReader(page.Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.NoError(t, zr.Close())
	require.Equal(t, largeIndex, string(decoded))
	head := do(r, http.MethodHead, "/assets/vpcs/gcp%3Avpc%3A42", headers)
	require.Equal(t, http.StatusOK, head.Code)
	require.Empty(t, head.Body.String())
	require.Equal(t, page.Header(), head.Header())
	cached := do(r, http.MethodGet, "/assets/vpcs/gcp%3Avpc%3A42", map[string]string{"If-None-Match": page.Header().Get("ETag")})
	require.Equal(t, http.StatusNotModified, cached.Code)
	require.Empty(t, cached.Body.String())
	require.Equal(t, page.Header().Get("Cache-Control"), cached.Header().Get("Cache-Control"))
}

func TestAssetsPagesServeSPAShell(t *testing.T) {
	for _, mode := range []string{"embedded", "disk"} {
		t.Run(mode, func(t *testing.T) {
			r := embedded(t)
			if mode == "disk" {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o600))
				h, err := webui.New(dir, nil)
				require.NoError(t, err)
				r = newRouter(t, h)
			}
			for _, target := range assetPagePaths() {
				for _, suffix := range []string{"", "/", "?provider=gcp&page=2"} {
					t.Run(target+suffix, func(t *testing.T) {
						get := do(r, http.MethodGet, target+suffix, nil)
						require.Equal(t, http.StatusOK, get.Code)
						require.Equal(t, indexHTML, get.Body.String())
						require.Contains(t, get.Header().Get("Content-Type"), "text/html")
						require.Equal(t, "no-cache, no-store, must-revalidate", get.Header().Get("Cache-Control"))
						head := do(r, http.MethodHead, target+suffix, nil)
						require.Equal(t, http.StatusOK, head.Code)
						require.Empty(t, head.Body.String())
						require.Equal(t, get.Header().Get("Content-Length"), head.Header().Get("Content-Length"))
						require.Equal(t, get.Header().Get("Cache-Control"), head.Header().Get("Cache-Control"))
					})
				}
			}
		})
	}
}

func TestUnknownAssetsPathsRemainNotFound(t *testing.T) {
	r := embedded(t)
	paths := []string{
		"/assets//", "/assets/unknown", "/assets/stale-123.js", "/assets/stale-123.js.map",
		"/assets/chunks/stale-123.js", "/assets/instances/index-abc.js",
		"/assets/instances/42.js", "/assets/instances/42.js.map",
		"/assets/instances/42/extra", "/assets/instances/gcp:instance:42/index.js",
		"/assets/cloud-accounts/new", "/assets/cloud-accounts/42", "/assets/sync-runs/42",
		"/assets/security-groups/42", "/assets/subnets/42",
		"/assets/instances/aws:vpc:42", "/assets/vpcs/gcp:instance:42",
		"/assets/databases/azure:database:42", "/assets/firewalls/GCP:firewall:42",
		"/assets/instances/0", "/assets/instances/01", "/assets/instances/+1",
		"/assets/instances/-1", "/assets/instances/9223372036854775808",
		"/assets/instances/gcp:instance:0", "/assets/instances/gcp:instance:01",
		"/assets/instances/gcp:instance:9223372036854775808",
		"/assets/instances/gcp:instance:42:1", "/assets/instances/42//",
		"/assets/instances//42", "/assets/fake/../instances", "/assets/./instances",
		"/assets/instances/.hidden", "/assets/instances/%2e%2e/firewalls",
	}
	for _, target := range paths {
		t.Run(target, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				rec := do(r, method, target, nil)
				require.Equal(t, http.StatusNotFound, rec.Code)
				require.Empty(t, rec.Body.String())
			}
		})
	}
}

func TestMissingAssetsTraversalPathsRemainNotFound(t *testing.T) {
	for _, mode := range []string{"embedded", "disk"} {
		t.Run(mode, func(t *testing.T) {
			r := embedded(t)
			if mode == "disk" {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o600))
				h, err := webui.New(dir, nil)
				require.NoError(t, err)
				r = newRouter(t, h)
			}
			for _, target := range []string{
				"/assets/../missing", "/assets/../../missing",
				"/assets/%2e%2e/missing", "/assets/instances/../../missing",
			} {
				t.Run(target, func(t *testing.T) {
					for _, method := range []string{http.MethodGet, http.MethodHead} {
						rec := do(r, method, target, nil)
						require.Equal(t, http.StatusNotFound, rec.Code)
						require.Empty(t, rec.Body.String())
					}
				})
			}
		})
	}
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
	r := embedded(t)
	for _, target := range []string{"/system/users", "/assets/instances", "/assets/instances/gcp:instance:42"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			rec := do(r, method, target, nil)
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code, target)
			require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"), target)
		}
	}
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
