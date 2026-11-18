package image_test

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
	"github.com/ArkGravity/optimus/internal/modules/docker/image"
	"github.com/ArkGravity/optimus/internal/modules/docker/internal/enginetest"
)

const (
	nginxID = "sha256:4f67c83422ec747235357c04556616234e66fc3fa39cb4f40b2d4441ddd8f100"
	oldID   = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
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
	h := image.NewHandler(image.NewService(connect, nil))
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(42)); c.Next() })
	r.GET("/hosts/:id/images", h.HandleList())
	r.DELETE("/hosts/:id/images/:imageId", h.HandleDelete())
	r.POST("/hosts/:id/images/prune", h.HandlePrune())
	return r
}

func call(t *testing.T, r *gin.Engine, method, path, body string) (int, envelope) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	require.NotContains(t, w.Body.String(), leak)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	return w.Code, env
}

func TestList_MapsSortsAndCleansUp(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodGet, "/images/json", http.StatusOK, []map[string]any{
		{"Id": oldID, "RepoTags": []string{"<none>:<none>"}, "Size": 10, "Created": 1_700_000_000},
		{"Id": nginxID, "RepoTags": []string{"nginx:1.27", "nginx:latest"}, "Size": 196_000_000, "Created": 1_760_000_000},
	})
	status, env := call(t, router(e.Connector()), http.MethodGet, "/hosts/7/images", "")
	require.Equal(t, http.StatusOK, status)

	var out image.ListResponse
	require.NoError(t, json.Unmarshal(env.Data, &out))
	require.Equal(t, 2, out.Total)
	require.Equal(t, nginxID, out.Items[0].ID)
	require.Equal(t, "4f67c83422ec", out.Items[0].ShortID)
	require.Equal(t, []string{"nginx:1.27", "nginx:latest"}, out.Items[0].RepoTags)
	require.False(t, out.Items[0].Dangling)
	require.Equal(t, int64(196_000_000), out.Items[0].Size)
	require.Equal(t, int64(1_760_000_000), out.Items[0].Created.Unix())
	require.True(t, out.Items[1].Dangling)
	require.Empty(t, out.Items[1].RepoTags)
	require.NotNil(t, out.Items[1].RepoTags, "empty tags serialize as [] not null")

	require.Equal(t, []string{"docker.image.list"}, e.Purposes())
	require.True(t, e.Balanced())
}

func TestDelete_ForwardsForceAndSplitsResult(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodDelete, "/images/"+nginxID, http.StatusOK, []map[string]string{
		{"Untagged": "nginx:latest"}, {"Untagged": "nginx:1.27"}, {"Deleted": nginxID},
	})
	r := router(e.Connector())

	status, env := call(t, r, http.MethodDelete, "/hosts/7/images/"+nginxID+"?force=true", "")
	require.Equal(t, http.StatusOK, status)
	var out image.DeleteResponse
	require.NoError(t, json.Unmarshal(env.Data, &out))
	require.Equal(t, []string{"nginx:1.27", "nginx:latest"}, out.Untagged)
	require.Equal(t, []string{nginxID}, out.Deleted)

	reqs := e.Requests()
	require.Len(t, reqs, 1)
	require.Equal(t, []string{"1"}, reqs[0].Query["force"])
	require.NotContains(t, reqs[0].Query, "noprune", "children are pruned like docker rmi")
	require.Equal(t, []string{"docker.image.delete"}, e.Purposes())
	require.True(t, e.Balanced())
}

func TestDelete_RejectsNonIDReferencesBeforeConnecting(t *testing.T) {
	e := enginetest.New(t)
	r := router(e.Connector())
	for _, ref := range []string{"nginx", "nginx:latest", "sha256:XYZ", "abc"} {
		status, env := call(t, r, http.MethodDelete, "/hosts/7/images/"+ref, "")
		require.Equal(t, http.StatusBadRequest, status, ref)
		require.Equal(t, 40002, env.Code, ref)
		require.Equal(t, "docker.image.id_invalid", env.MessageKey, ref)
	}
	status, env := call(t, r, http.MethodDelete, "/hosts/7/images/"+nginxID+"?force=maybe", "")
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, 40002, env.Code)
	require.Empty(t, e.Purposes())
}

func TestDelete_MapsEngineErrorsToSafeEnvelopes(t *testing.T) {
	cases := []struct {
		engineStatus, status, code int
		key                        string
	}{
		{http.StatusConflict, http.StatusConflict, 44105, "docker.image.conflict"},
		{http.StatusNotFound, http.StatusNotFound, 40401, "docker.image.not_found"},
		{http.StatusInternalServerError, http.StatusInternalServerError, 44103, "docker.engine.error"},
	}
	for _, tc := range cases {
		e := enginetest.New(t)
		e.Reply(http.MethodDelete, "/images/"+nginxID, tc.engineStatus, "unable to delete "+leak)
		status, env := call(t, router(e.Connector()), http.MethodDelete, "/hosts/7/images/"+nginxID, "")
		require.Equal(t, tc.status, status, tc.key)
		require.Equal(t, tc.code, env.Code, tc.key)
		require.Equal(t, tc.key, env.MessageKey)
		require.True(t, e.Balanced(), "cleanup after failure")
	}
}

func TestPrune_DefaultsToDanglingAndAllWidens(t *testing.T) {
	e := enginetest.New(t)
	e.Reply(http.MethodPost, "/images/prune", http.StatusOK, map[string]any{
		"ImagesDeleted":  []map[string]string{{"Deleted": oldID}, {"Untagged": "old:1"}},
		"SpaceReclaimed": 4096,
	})
	r := router(e.Connector())

	status, env := call(t, r, http.MethodPost, "/hosts/7/images/prune", "")
	require.Equal(t, http.StatusOK, status)
	var out image.PruneResponse
	require.NoError(t, json.Unmarshal(env.Data, &out))
	require.Equal(t, []string{oldID}, out.Deleted)
	require.Equal(t, []string{"old:1"}, out.Untagged)
	require.Equal(t, uint64(4096), out.SpaceReclaimed)

	status, _ = call(t, r, http.MethodPost, "/hosts/7/images/prune", `{"all":true}`)
	require.Equal(t, http.StatusOK, status)

	reqs := e.Requests()
	require.Len(t, reqs, 2)
	require.Contains(t, reqs[0].Query["filters"][0], `"dangling":{"true":true}`)
	require.Contains(t, reqs[1].Query["filters"][0], `"dangling":{"false":true}`)
	require.True(t, e.Balanced())

	status, env = call(t, r, http.MethodPost, "/hosts/7/images/prune", `{"all":`)
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, 40002, env.Code)
}

func TestConnectFailuresAreSafe(t *testing.T) {
	connect := enginetest.Failing(dockerssh.AppError(&dockerssh.Error{Kind: dockerssh.KindSocket}))
	r := router(connect)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/hosts/7/images"},
		{http.MethodDelete, "/hosts/7/images/" + nginxID},
		{http.MethodPost, "/hosts/7/images/prune"},
	} {
		status, env := call(t, r, tc.method, tc.path, "")
		require.Equal(t, http.StatusInternalServerError, status, tc.path)
		require.Equal(t, 44101, env.Code, tc.path)
		require.Equal(t, "docker.host.unreachable", env.MessageKey)
		require.Equal(t, dockerssh.SafeMessage(dockerssh.KindSocket), env.Message)
	}

	status, env := call(t, r, http.MethodGet, "/hosts/abc/images", "")
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, 40001, env.Code)
}
