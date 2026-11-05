package vpc

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

func TestServiceRejectsUnsupportedProviderBeforeRepository(t *testing.T) {
	input := ListQuery{Provider: "azure"}
	repo := &stubRepo{}
	_, err := NewService(repo).List(context.Background(), input)
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation || biz.MessageKey != "common.validation" {
		t.Fatalf("expected common.validation, got %#v", err)
	}
	if repo.listFilter.Page != 0 {
		t.Fatal("repository called for invalid provider")
	}
}

func TestServiceTrimsCommonFilters(t *testing.T) {
	repo := &stubRepo{}
	_, err := NewService(repo).List(context.Background(), ListQuery{Provider: " gcp ", ProjectID: " project ", Region: " region ", Zone: " zone ", Q: " needle "})
	if err != nil {
		t.Fatal(err)
	}
	f := repo.listFilter
	if f.Provider != "gcp" || f.ProjectID != "project" || f.Region != "region" || f.Zone != "zone" || f.Q != "needle" {
		t.Fatalf("filter=%#v", f)
	}
}
