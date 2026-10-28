package securitygroup

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
)

type stubRepo struct {
	query ListQuery
	err   error
	calls int
}

func (r *stubRepo) List(_ context.Context, q ListQuery) ([]Summary, int64, error) {
	r.query = q
	r.calls++
	return nil, 0, r.err
}
func (r *stubRepo) Get(context.Context, uint64) (*Detail, error) {
	r.calls++
	return &Detail{Summary: Summary{ID: 9}, InboundRules: []Rule{}, OutboundRules: []Rule{}, Tags: map[string]string{}}, r.err
}

func TestSecurityGroupHTTPContractAndValidation(t *testing.T) {
	repo := &stubRepo{}
	router := gin.New()
	NewHandler(NewService(repo)).Mount(router.Group("/assets"), func(code string) gin.HandlerFunc {
		require.Equal(t, "assets:resource:read", code)
		return func(c *gin.Context) { c.Next() }
	})
	for _, path := range []string{"/assets/security-groups?page=0", "/assets/security-groups?size=201", "/assets/security-groups?include_deleted=bad", "/assets/security-groups?account_id=9223372036854775808", "/assets/security-groups/0", "/assets/security-groups/no", "/assets/security-groups/9223372036854775808"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, rec.Code, path)
	}
	require.Zero(t, repo.calls)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/security-groups?account_id=7&region=%20us-east-1%20&vpc_id=%20vpc-1%20&q=%20web%20&include_deleted=true", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ListQuery{AccountID: 7, Region: "us-east-1", VPCID: "vpc-1", Q: "web", IncludeDeleted: true, Page: 1, Size: 20}, repo.query)
	var envelope struct {
		Code int          `json:"code"`
		Data ListResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
	require.NotNil(t, envelope.Data.Items)
	for _, tc := range []struct {
		err    error
		status int
	}{{gorm.ErrRecordNotFound, 404}, {errors.New("secret postgres password"), 500}, {nil, 200}} {
		repo.err = tc.err
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/security-groups/9", nil))
		require.Equal(t, tc.status, rec.Code)
		require.NotContains(t, rec.Body.String(), "secret postgres")
	}
}

func TestSecurityGroupRoutesDenyWithoutResourcePermission(t *testing.T) {
	repo := &stubRepo{}
	router := gin.New()
	NewHandler(NewService(repo)).Mount(router.Group("/assets"), func(string) gin.HandlerFunc {
		return func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) }
	})
	for _, path := range []string{"/assets/security-groups", "/assets/security-groups/9"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusForbidden, rec.Code)
	}
	require.Zero(t, repo.calls)
}
