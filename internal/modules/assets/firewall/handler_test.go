package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type stubRepo struct {
	rows    []Summary
	detail  *Detail
	filter  query.Filter
	key     string
	deleted bool
	err     error
	calls   int
}

func (r *stubRepo) List(_ context.Context, f query.Filter) ([]Summary, int64, error) {
	r.calls++
	r.filter = f
	return r.rows, int64(len(r.rows)), r.err
}
func (r *stubRepo) FindByKey(_ context.Context, key string, deleted bool) (*Detail, error) {
	r.calls++
	r.key = key
	r.deleted = deleted
	return r.detail, r.err
}

func TestFirewallRoutesProtectDetailsAndKeepListBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	zero := int32(0)
	detail := query.Firewall{Base: query.Base{AssetKey: "aws:firewall:42", Provider: "aws", NativeID: "sg-42"}, InboundRuleCount: 1, InboundRules: []resource.Rule{{Protocol: "icmp", ICMPType: &zero, ICMPCode: &zero}}, NativeDetails: query.FirewallNativeDetails{AWSInboundRuleFacts: []query.AWSRuleFacts{{RuleIndex: 0, OwnerID: "peer", VPCID: "vpc-peer", PeeringID: "pcx-peer"}}}}
	repo := &stubRepo{detail: &detail, rows: []Summary{summarize(detail)}}
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
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", auth)
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/assets/firewalls", "/assets/firewalls/aws:firewall:42"} {
		require.Equal(t, 401, request(path, "").Code)
		require.Equal(t, 403, request(path, "denied").Code)
	}
	require.Zero(t, repo.calls)
	for _, key := range []string{"aws:firewall:42", "gcp:firewall:42", "42"} {
		rec := request("/assets/firewalls/"+key+"?include_deleted=true", "allowed")
		require.Equal(t, 200, rec.Code, rec.Body.String())
		require.Equal(t, key, repo.key)
		require.True(t, repo.deleted)
		require.Contains(t, rec.Body.String(), `"icmp_type":0`)
		require.Contains(t, rec.Body.String(), `"icmp_code":0`)
		require.Contains(t, rec.Body.String(), `"owner_id":"peer"`)
	}
	rec := request("/assets/firewalls?provider=gcp&account_id=5&project_id=project&network_ref=projects/host/global/networks/main&include_deleted=true&page=2&size=3", "allowed")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, "gcp", repo.filter.Provider)
	require.EqualValues(t, 5, repo.filter.AccountID)
	require.Equal(t, "project", repo.filter.ProjectID)
	require.Equal(t, "projects/host/global/networks/main", repo.filter.NetworkRef)
	require.True(t, repo.filter.IncludeDeleted)
	require.Equal(t, 2, repo.filter.Page)
	require.Equal(t, 3, repo.filter.Size)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.EqualValues(t, 1, body.Data.Items[0]["inbound_rule_count"])
	require.NotContains(t, body.Data.Items[0], "inbound_rules")
	require.NotContains(t, rec.Body.String(), "aws_inbound_rule_facts")
	calls := repo.calls
	for _, path := range []string{"/assets/firewalls/gcp:database:42", "/assets/firewalls/0", "/assets/firewalls/azure:firewall:42", "/assets/firewalls/gcp:firewall:42?include_deleted=no", "/assets/firewalls?provider=azure", "/assets/firewalls?account_id=9223372036854775808", "/assets/firewalls?page=0", "/assets/firewalls?size=201"} {
		require.Equal(t, 400, request(path, "allowed").Code, path)
	}
	require.Equal(t, calls, repo.calls)
	repo.err = errors.New("password=never-leak")
	rec = request("/assets/firewalls/aws:firewall:42", "allowed")
	require.Equal(t, 500, rec.Code)
	require.NotContains(t, rec.Body.String(), "never-leak")
	repo.err = gorm.ErrRecordNotFound
	require.Equal(t, 404, request("/assets/firewalls/aws:firewall:42", "allowed").Code)
	repo.err = nil
	repo.rows = nil
	rec = request("/assets/firewalls", "allowed")
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), `"items":[]`)
}
