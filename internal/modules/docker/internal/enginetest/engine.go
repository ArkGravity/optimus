// Package enginetest serves a scripted fake Docker Engine API so live-service
// tests drive the real Docker client, including its error decoding.
package enginetest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	dockerapi "github.com/docker/docker/client"

	"github.com/ArkGravity/optimus/internal/models"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
)

// APIVersion is advertised on /_ping so the client pins a stable prefix.
const APIVersion = "1.47"

var versionPrefix = regexp.MustCompile(`^/v[0-9.]+`)

// Request is one Engine call with the API version prefix stripped.
type Request struct {
	Method, Path string
	Query        map[string][]string
}

type reply struct {
	status int
	body   any
}

// Engine is a fake Engine plus a Connector that reaches it.
type Engine struct {
	srv *httptest.Server

	mu       sync.Mutex
	routes   map[string]reply
	requests []Request
	purposes []string
	opens    int
	cleanups int
}

func New(t *testing.T) *Engine {
	t.Helper()
	e := &Engine{routes: map[string]reply{}}
	e.srv = httptest.NewServer(http.HandlerFunc(e.serve))
	t.Cleanup(e.srv.Close)
	return e
}

// Reply scripts the response for method + path (without version prefix).
// Statuses >= 400 send the Engine error shape {"message": body}.
func (e *Engine) Reply(method, path string, status int, body any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.routes[method+" "+path] = reply{status: status, body: body}
}

func (e *Engine) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Api-Version", APIVersion)
	if r.URL.Path == "/_ping" {
		_, _ = w.Write([]byte("OK"))
		return
	}
	path := versionPrefix.ReplaceAllString(r.URL.Path, "")
	e.mu.Lock()
	e.requests = append(e.requests, Request{Method: r.Method, Path: path, Query: r.URL.Query()})
	rep, ok := e.routes[r.Method+" "+path]
	e.mu.Unlock()
	if !ok {
		rep = reply{status: http.StatusNotImplemented, body: "unscripted " + r.Method + " " + path}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rep.status)
	if rep.status == http.StatusNoContent || rep.status == http.StatusNotModified {
		return
	}
	body := rep.body
	if rep.status >= 400 {
		body = map[string]any{"message": rep.body}
	}
	_ = json.NewEncoder(w).Encode(body)
}

// Host is the stored host every Connector call resolves to.
var Host = &models.DockerHost{ID: 7, Name: "edge-1"}

// Connector opens a real Docker client against the fake Engine.
func (e *Engine) Connector() dockerssh.Connector {
	return func(_ context.Context, _, hostID uint64, purpose string) (*models.DockerHost, *dockerapi.Client, func(), error) {
		e.mu.Lock()
		e.purposes = append(e.purposes, purpose)
		e.mu.Unlock()
		cli, err := dockerapi.NewClientWithOpts(dockerapi.WithHost(e.srv.URL), dockerapi.WithAPIVersionNegotiation())
		if err != nil {
			return nil, nil, nil, err
		}
		e.mu.Lock()
		e.opens++
		e.mu.Unlock()
		h := *Host
		h.ID = hostID
		return &h, cli, func() {
			_ = cli.Close()
			e.mu.Lock()
			e.cleanups++
			e.mu.Unlock()
		}, nil
	}
}

// Failing returns a Connector that always fails with err (already safe).
func Failing(err error) dockerssh.Connector {
	return func(context.Context, uint64, uint64, string) (*models.DockerHost, *dockerapi.Client, func(), error) {
		return nil, nil, nil, err
	}
}

func (e *Engine) Requests() []Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Request(nil), e.requests...)
}

func (e *Engine) Purposes() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.purposes...)
}

// Balanced reports whether every opened session was cleaned up.
func (e *Engine) Balanced() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.opens == e.cleanups
}
