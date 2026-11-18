package container_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/middleware"
	dockerssh "github.com/ArkGravity/optimus/internal/modules/docker/client"
	"github.com/ArkGravity/optimus/internal/modules/docker/container"
	"github.com/ArkGravity/optimus/internal/modules/docker/internal/enginetest"
)

const (
	webID   = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	imageID = "sha256:4f67c83422ec747235357c04556616234e66fc3fa39cb4f40b2d4441ddd8f100"
	leak    = "raw-engine-detail-should-not-leak"
)

type envelope struct {
	Code       int             `json:"code"`
	Data       json.RawMessage `json:"data"`
	Message    string          `json:"message"`
	MessageKey string          `json:"message_key"`
}

func router(connect dockerssh.Connector) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := container.NewHandler(container.NewService(connect, nil))
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(42)); c.Next() })
	r.GET("/hosts/:id/containers", h.HandleList())
	r.GET("/hosts/:id/containers/:containerId", h.HandleGet())
	r.POST("/hosts/:id/containers/:containerId/start", h.HandleStart())
	r.POST("/hosts/:id/containers/:containerId/stop", h.HandleStop())
	r.POST("/hosts/:id/containers/:containerId/restart", h.HandleRestart())
	r.DELETE("/hosts/:id/containers/:containerId", h.HandleDelete())
	return r
}

func call(t *testing.T, r *gin.Engine, method, path string) (int, envelope) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	require.NotContains(t, w.Body.String(), leak)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	return w.Code, env
}

func TestList_IncludesStoppedAndSummarizesPorts(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodGet, "/containers/json", http.StatusOK, []map[string]any{
		{
			"Id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Names": []string{"/old"},
			"Image": "busybox", "ImageID": imageID, "Command": "sh", "Created": 1_700_000_000,
			"State": "exited", "Status": "Exited (0) 2 days ago",
		},
		{
			"Id": webID, "Names": []string{"/web"}, "Image": "nginx:1.27", "ImageID": imageID,
			"Command": "/docker-entrypoint.sh nginx", "Created": 1_760_000_000, "State": "running", "Status": "Up 3 hours",
			"Ports": []map[string]any{
				{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
				{"IP": "::", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
				{"PrivatePort": 443, "Type": "tcp"},
				{"PrivatePort": 443, "Type": "tcp"},
			},
		},
	})
	status, env := call(t, router(e.Connector()), http.MethodGet, "/hosts/7/containers")
	require.Equal(t, http.StatusOK, status)

	var out container.ListResponse
	require.NoError(t, json.Unmarshal(env.Data, &out))
	require.Equal(t, 2, out.Total)
	web := out.Items[0]
	require.Equal(t, webID, web.ID)
	require.Equal(t, "9f86d081884c", web.ShortID)
	require.Equal(t, "web", web.Name)
	require.Equal(t, "4f67c83422ec", web.ImageID)
	require.Equal(t, "running", web.State)
	require.Equal(t, "Up 3 hours", web.Status)
	require.Equal(t, []string{"0.0.0.0:8080->80/tcp", "443/tcp", "[::]:8080->80/tcp"}, web.Ports)
	require.Equal(t, "exited", out.Items[1].State)
	require.NotNil(t, out.Items[1].Ports)

	reqs := e.Requests()
	require.Len(t, reqs, 1)
	require.Equal(t, []string{"1"}, reqs[0].Query["all"], "stopped containers are listed")
	require.Equal(t, []string{"docker.container.list"}, e.Purposes())
	require.True(t, e.Balanced())
}

func TestGet_MapsInspectSummary(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodGet, "/containers/web/json", http.StatusOK, map[string]any{
		"Id": webID, "Name": "/web", "Image": imageID, "Created": "2026-11-18T10:00:00.123456789Z",
		"Path": "/docker-entrypoint.sh", "Args": []string{"nginx", "-g", "daemon off;"}, "RestartCount": 2,
		"State": map[string]any{
			"Status": "exited", "ExitCode": 137, "OOMKilled": true,
			"StartedAt": "2026-11-18T10:00:01Z", "FinishedAt": "0001-01-01T00:00:00Z",
			"Health": map[string]any{"Status": "unhealthy"},
		},
		"HostConfig": map[string]any{"RestartPolicy": map[string]any{"Name": "on-failure", "MaximumRetryCount": 3}},
		"Config":     map[string]any{"Image": "nginx:1.27", "Env": []string{"PATH=/usr/bin", "TOKEN=x"}},
		"NetworkSettings": map[string]any{
			"Ports": map[string]any{
				"80/tcp":  []map[string]string{{"HostIp": "0.0.0.0", "HostPort": "8080"}},
				"443/tcp": nil,
			},
			"Networks": map[string]any{
				"frontend": map[string]any{"IPAddress": "10.0.1.5", "Gateway": "10.0.1.1"},
				"bridge":   map[string]any{"IPAddress": "172.17.0.2", "Gateway": "172.17.0.1", "MacAddress": "02:42:ac:11:00:02"},
			},
		},
		"Mounts": []map[string]any{{
			"Type": "volume", "Name": "data", "Source": "/var/lib/docker/volumes/data/_data",
			"Destination": "/data", "Mode": "z", "RW": true,
		}},
	})
	status, env := call(t, router(e.Connector()), http.MethodGet, "/hosts/7/containers/web")
	require.Equal(t, http.StatusOK, status)

	var d container.Detail
	require.NoError(t, json.Unmarshal(env.Data, &d))
	require.Equal(t, webID, d.ID)
	require.Equal(t, "9f86d081884c", d.ShortID)
	require.Equal(t, "web", d.Name)
	require.Equal(t, "nginx:1.27", d.Image)
	require.Equal(t, "4f67c83422ec", d.ImageID)
	require.NotNil(t, d.Created)
	require.Equal(t, []string{"/docker-entrypoint.sh", "nginx", "-g", "daemon off;"}, d.Command)
	require.Equal(t, container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3}, d.RestartPolicy)
	require.Equal(t, 2, d.RestartCount)
	require.Equal(t, "exited", d.State.Status)
	require.Equal(t, 137, d.State.ExitCode)
	require.True(t, d.State.OOMKilled)
	require.Equal(t, "unhealthy", d.State.Health)
	require.NotNil(t, d.State.StartedAt)
	require.Nil(t, d.State.FinishedAt, "Docker's zero time means never")
	require.Equal(t, []string{"0.0.0.0:8080->80/tcp", "443/tcp"}, d.Ports)
	require.Equal(t, []container.Network{
		{Name: "bridge", IPAddress: "172.17.0.2", Gateway: "172.17.0.1", MacAddress: "02:42:ac:11:00:02"},
		{Name: "frontend", IPAddress: "10.0.1.5", Gateway: "10.0.1.1"},
	}, d.Networks)
	require.Equal(t, []container.Mount{{
		Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Mode: "z", RW: true,
	}}, d.Mounts)
	require.Equal(t, []string{"PATH=/usr/bin", "TOKEN=x"}, d.Env)
	require.Equal(t, []string{"docker.container.get"}, e.Purposes())
	require.True(t, e.Balanced())
}

func TestLifecycleActions(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodPost, "/containers/web/start", http.StatusNoContent, nil)
	e.Reply(http.MethodPost, "/containers/web/stop", http.StatusNotModified, nil)
	e.Reply(http.MethodPost, "/containers/web/restart", http.StatusNoContent, nil)
	e.Reply(http.MethodDelete, "/containers/web", http.StatusNoContent, nil)
	r := router(e.Connector())

	for _, path := range []string{
		"/hosts/7/containers/web/start",
		"/hosts/7/containers/web/stop",
		"/hosts/7/containers/web/restart",
	} {
		status, env := call(t, r, http.MethodPost, path)
		require.Equal(t, http.StatusOK, status, path)
		require.JSONEq(t, `{"ok":true}`, string(env.Data), path)
	}
	status, _ := call(t, r, http.MethodDelete, "/hosts/7/containers/web?force=true")
	require.Equal(t, http.StatusOK, status)
	status, _ = call(t, r, http.MethodDelete, "/hosts/7/containers/web")
	require.Equal(t, http.StatusOK, status)

	reqs := e.Requests()
	require.Len(t, reqs, 5)
	require.Equal(t, []string{"1"}, reqs[3].Query["force"])
	require.NotContains(t, reqs[4].Query, "force")
	require.NotContains(t, reqs[4].Query, "v", "volumes are kept")
	require.Equal(t, []string{
		"docker.container.start", "docker.container.stop", "docker.container.restart",
		"docker.container.delete", "docker.container.delete",
	}, e.Purposes())
	require.True(t, e.Balanced())
}

func TestEngineErrorsAreSafe(t *testing.T) {
	cases := []struct {
		method, enginePath, path   string
		engineStatus, status, code int
		key                        string
	}{
		{http.MethodDelete, "/containers/web", "/hosts/7/containers/web", http.StatusConflict, http.StatusConflict, 44105, "docker.container.conflict"},
		{http.MethodGet, "/containers/ghost/json", "/hosts/7/containers/ghost", http.StatusNotFound, http.StatusNotFound, 40401, "docker.container.not_found"},
		{http.MethodPost, "/containers/web/start", "/hosts/7/containers/web/start", http.StatusInternalServerError, http.StatusInternalServerError, 44103, "docker.engine.error"},
	}
	for _, tc := range cases {
		e := enginetest.New(t)
		e.Reply(tc.method, tc.enginePath, tc.engineStatus, "cannot proceed: "+leak)
		status, env := call(t, router(e.Connector()), tc.method, tc.path)
		require.Equal(t, tc.status, status, tc.key)
		require.Equal(t, tc.code, env.Code, tc.key)
		require.Equal(t, tc.key, env.MessageKey)
		require.True(t, e.Balanced())
	}
}

func TestRejectsBadInputBeforeConnecting(t *testing.T) {
	e := enginetest.New(t)
	r := router(e.Connector())
	cases := []struct {
		method, path string
		status, code int
	}{
		{http.MethodGet, "/hosts/0/containers", http.StatusBadRequest, 40001},
		{http.MethodGet, "/hosts/7/containers/-web", http.StatusBadRequest, 40002},
		{http.MethodPost, "/hosts/7/containers/.web/stop", http.StatusBadRequest, 40002},
		{http.MethodDelete, "/hosts/7/containers/" + strings.Repeat("a", 300), http.StatusBadRequest, 40002},
		{http.MethodDelete, "/hosts/7/containers/web?force=maybe", http.StatusBadRequest, 40002},
	}
	for _, tc := range cases {
		status, env := call(t, r, tc.method, tc.path)
		require.Equal(t, tc.status, status, tc.path)
		require.Equal(t, tc.code, env.Code, tc.path)
	}
	require.Empty(t, e.Purposes())
}

func TestConnectFailuresAreSafe(t *testing.T) {
	r := router(enginetest.Failing(dockerssh.AppError(&dockerssh.Error{Kind: dockerssh.KindAuth})))
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/hosts/7/containers"},
		{http.MethodGet, "/hosts/7/containers/web"},
		{http.MethodPost, "/hosts/7/containers/web/restart"},
		{http.MethodDelete, "/hosts/7/containers/web"},
	} {
		status, env := call(t, r, tc.method, tc.path)
		require.Equal(t, http.StatusInternalServerError, status, tc.path)
		require.Equal(t, 44102, env.Code, tc.path)
		require.Equal(t, dockerssh.SafeMessage(dockerssh.KindAuth), env.Message)
	}
}
