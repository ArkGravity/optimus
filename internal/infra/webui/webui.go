// Package webui serves the single-page web UI from the embedded build or from
// an on-disk directory, falling back to index.html for client-side routes.
package webui

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

const (
	indexFile   = "index.html"
	assetsDir   = "assets/"
	gzipMinSize = 1024

	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheNoStore    = "no-cache, no-store, must-revalidate"
	cacheRevalidate = "no-cache"

	notBuiltMessage = "web UI is not built: run `make web` or set server.web_dir"
)

type asset struct {
	body        []byte
	gzip        []byte
	contentType string
	etag        string
}

// Handler is a gin NoRoute handler. Exactly one of assets or dir is set.
type Handler struct {
	assets map[string]*asset
	dir    fs.FS
}

// New serves webDir from disk when set; otherwise it preloads the embedded
// build into memory with precomputed gzip variants and ETags.
func New(webDir string, embedded fs.FS) (*Handler, error) {
	if webDir != "" {
		if _, err := os.Stat(filepath.Join(webDir, indexFile)); err != nil {
			return nil, fmt.Errorf("server.web_dir %q has no %s: %w", webDir, indexFile, err)
		}
		return &Handler{dir: os.DirFS(webDir)}, nil
	}
	assets, err := preload(embedded)
	if err != nil {
		return nil, err
	}
	return &Handler{assets: assets}, nil
}

// Built reports whether an index.html is available to serve.
func (h *Handler) Built() bool {
	if h.dir != nil {
		return true
	}
	_, ok := h.assets[indexFile]
	return ok
}

func (h *Handler) Handle(c *gin.Context) {
	p := c.Request.URL.Path
	if p == "/api" || strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/swagger") {
		response.Error(c, apperr.New(apperr.CodeNotFound, "common.not_found", "route not found"))
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Header("Allow", "GET, HEAD")
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean(p), "/")
	if name == "" {
		name = indexFile
	}
	if !hidden(name) && h.serve(c, name) {
		return
	}
	// Assets pages share this prefix with Vite bundles. Only known page shapes
	// fall back; stale chunks and unknown resource paths must remain 404s.
	if (strings.HasPrefix(p, "/"+assetsDir) || strings.HasPrefix(name, assetsDir)) && !knownAssetsPage(p) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if !h.serve(c, indexFile) {
		c.String(http.StatusNotFound, notBuiltMessage)
	}
}

// knownAssetsPage mirrors the Assets menu and static detail route shapes without
// depending on business modules. Check the original decoded path so cleaning
// cannot turn an arbitrary nested path into a valid page.
func knownAssetsPage(p string) bool {
	if p == "/assets" || p == "/assets/" {
		return true
	}
	parts := strings.Split(strings.TrimSuffix(p, "/"), "/")
	if len(parts) < 3 || parts[0] != "" || parts[1] != "assets" {
		return false
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "cloud-accounts", "vpcs", "firewalls", "instances", "databases", "sync-runs", "security-groups":
			return true
		}
		return false
	}
	if len(parts) != 4 {
		return false
	}
	var family string
	switch parts[2] {
	case "instances":
		family = "instance"
	case "vpcs":
		family = "vpc"
	case "databases":
		family = "database"
	case "firewalls":
		family = "firewall"
	default:
		return false
	}
	key := strings.Split(parts[3], ":")
	if len(key) == 3 {
		switch key[0] {
		case "aws", "aliyun", "gcp":
		default:
			return false
		}
		if key[1] != family {
			return false
		}
	} else if len(key) != 1 {
		return false
	}
	id := key[len(key)-1]
	n, err := strconv.ParseUint(id, 10, 63)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == id
}

func (h *Handler) serve(c *gin.Context, name string) bool {
	if h.dir != nil {
		return serveFromDir(c, h.dir, name)
	}
	a, ok := h.assets[name]
	if !ok {
		return false
	}
	hdr := c.Writer.Header()
	hdr.Set("Content-Type", a.contentType)
	hdr.Set("Cache-Control", cacheControl(name))
	hdr.Set("ETag", a.etag)
	body := a.body
	if a.gzip != nil {
		hdr.Add("Vary", "Accept-Encoding")
		if acceptsGzip(c.GetHeader("Accept-Encoding")) {
			hdr.Set("Content-Encoding", "gzip")
			body = a.gzip
		}
	}
	if etagMatches(c.GetHeader("If-None-Match"), a.etag) {
		c.Status(http.StatusNotModified)
		return true
	}
	hdr.Set("Content-Length", strconv.Itoa(len(body)))
	c.Status(http.StatusOK)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.Write(body)
	}
	return true
}

func serveFromDir(c *gin.Context, dir fs.FS, name string) bool {
	f, err := dir.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	c.Header("Cache-Control", cacheControl(name))
	http.ServeContent(c.Writer, c.Request, name, st.ModTime(), rs)
	return true
}

func preload(fsys fs.FS) (map[string]*asset, error) {
	assets := map[string]*asset{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name != "." && hidden(name) {
				return fs.SkipDir
			}
			return nil
		}
		if hidden(name) {
			return nil
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		a := &asset{
			body:        body,
			contentType: contentType(name, body),
			etag:        `W/"` + hex.EncodeToString(sum[:12]) + `"`,
		}
		if len(body) >= gzipMinSize && compressible(a.contentType) {
			gz, err := gzipBytes(body)
			if err != nil {
				return err
			}
			if len(gz) < len(body) {
				a.gzip = gz
			}
		}
		assets[name] = a
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load embedded web UI: %w", err)
	}
	return assets, nil
}

func cacheControl(name string) string {
	switch {
	case strings.HasPrefix(name, assetsDir):
		return cacheImmutable
	case name == indexFile:
		return cacheNoStore
	default:
		return cacheRevalidate
	}
}

// hidden rejects dot-segments such as the tracked dist/.gitkeep placeholder.
func hidden(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func contentType(name string, body []byte) string {
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return ct
	}
	return http.DetectContentType(body)
}

func compressible(ct string) bool {
	return strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "xml")
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}

func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "*" || strings.TrimPrefix(tag, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}
