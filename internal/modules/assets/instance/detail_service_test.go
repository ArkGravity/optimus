package instance

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
	stubListRepo
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
	for _, key := range []string{"gcp:wrong:42", "0", "azure:instance:42"} {
		_, err := service.FindByKey(context.Background(), key, false)
		var biz *apperr.BizError
		if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation {
			t.Fatalf("key=%s err=%#v", key, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid key reached repository")
	}
	for _, key := range []string{"gcp:instance:42", "42"} {
		row, err := service.FindByKey(context.Background(), key, true)
		if err != nil || row != repo.row || repo.key != key || !repo.deleted {
			t.Fatalf("row=%#v err=%v repo=%#v", row, err, repo)
		}
	}
	repo.findError = gorm.ErrRecordNotFound
	_, err := service.FindByKey(context.Background(), "gcp:instance:42", false)
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeNotFound {
		t.Fatalf("not found error=%#v", err)
	}
	repo.findError = errors.New("database password=secret")
	_, err = service.FindByKey(context.Background(), "gcp:instance:42", false)
	if !errors.As(err, &biz) || biz.Code != apperr.CodeDBError || biz.Message != "database operation failed" {
		t.Fatalf("unsafe database error=%#v", err)
	}
}
