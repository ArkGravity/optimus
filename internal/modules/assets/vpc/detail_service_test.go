package vpc

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

type keyReader interface {
	FindByKey(context.Context, string, bool) (*Summary, error)
}
type keyRepo struct {
	stubRepo
	key       string
	deleted   bool
	calls     int
	row       *Summary
	findError error
}

func (r *keyRepo) FindByKey(_ context.Context, key string, deleted bool) (*Summary, error) {
	r.calls++
	r.key = key
	r.deleted = deleted
	return r.row, r.findError
}
func TestServiceKeyReadsValidateAndPreserveErrors(t *testing.T) {
	repo := &keyRepo{row: &Summary{ID: 42, Provider: "gcp", NativeID: "123"}}
	service, ok := any(NewService(repo)).(keyReader)
	if !ok {
		t.Fatal("provider-key detail service is missing")
	}
	for _, key := range []string{"gcp:wrong:42", "0", "azure:vpc:42"} {
		_, err := service.FindByKey(context.Background(), key, false)
		var biz *apperr.BizError
		if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation {
			t.Fatalf("key=%s err=%#v", key, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid key reached repository")
	}
	for _, key := range []string{"gcp:vpc:42", "42"} {
		row, err := service.FindByKey(context.Background(), key, true)
		if err != nil || row != repo.row || repo.key != key || !repo.deleted {
			t.Fatalf("row=%#v err=%v repo=%#v", row, err, repo)
		}
	}
	repo.findError = gorm.ErrRecordNotFound
	_, err := service.FindByKey(context.Background(), "gcp:vpc:42", false)
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeAssetsVPCNotFound {
		t.Fatalf("not found error=%#v", err)
	}
	repo.findError = errors.New("database password=secret")
	_, err = service.FindByKey(context.Background(), "gcp:vpc:42", false)
	if !errors.As(err, &biz) || biz.Code != apperr.CodeDBError || biz.Message != "database operation failed" {
		t.Fatalf("unsafe database error=%#v", err)
	}
}

type subnetKeyReader interface {
	ListSubnetsByVPCKey(context.Context, string, ListQuery) (*SubnetListResponse, error)
}
type subnetKeyRepo struct {
	stubRepo
	key    string
	filter ListFilter
	calls  int
	rows   []SubnetSummary
	err    error
}

func (r *subnetKeyRepo) ListSubnetsByVPC(_ context.Context, key string, f ListFilter) ([]SubnetSummary, int64, error) {
	r.calls++
	r.key = key
	r.filter = f
	return r.rows, int64(len(r.rows)), r.err
}
func TestServiceKeySubnetReadsValidateAndBindParent(t *testing.T) {
	repo := &subnetKeyRepo{rows: []SubnetSummary{{SubnetID: "subnet"}}}
	service, ok := any(NewService(repo)).(subnetKeyReader)
	if !ok {
		t.Fatal("provider-key subnet service is missing")
	}
	for _, input := range []struct {
		key   string
		query ListQuery
	}{{"gcp:instance:42", ListQuery{}}, {"gcp:vpc:42", ListQuery{Provider: "azure"}}, {"gcp:vpc:42", ListQuery{Page: -1}}} {
		_, err := service.ListSubnetsByVPCKey(context.Background(), input.key, input.query)
		assertBizCode(t, err, apperr.CodeValidation)
	}
	if repo.calls != 0 {
		t.Fatal("invalid subnet input reached repository")
	}
	result, err := service.ListSubnetsByVPCKey(context.Background(), "gcp:vpc:42", ListQuery{ProjectID: " project ", Region: " us-central1 ", IncludeDeleted: true})
	if err != nil || len(result.Items) != 1 || repo.key != "gcp:vpc:42" || repo.filter.ProjectID != "project" || repo.filter.Region != "us-central1" || !repo.filter.IncludeDeleted {
		t.Fatalf("result=%#v error=%v repository=%#v", result, err, repo)
	}
	repo.rows = nil
	result, err = service.ListSubnetsByVPCKey(context.Background(), "42", ListQuery{})
	if err != nil || result.Items == nil {
		t.Fatalf("empty result=%#v err=%v", result, err)
	}
	repo.err = gorm.ErrRecordNotFound
	_, err = service.ListSubnetsByVPCKey(context.Background(), "gcp:vpc:42", ListQuery{})
	assertBizCode(t, err, apperr.CodeAssetsVPCNotFound)
	repo.err = errors.New("database password=secret")
	_, err = service.ListSubnetsByVPCKey(context.Background(), "gcp:vpc:42", ListQuery{})
	assertSafeDBError(t, err)
}
