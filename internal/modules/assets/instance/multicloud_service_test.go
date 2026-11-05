package instance

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

func TestServiceRejectsUnsupportedProviderBeforeRepository(t *testing.T) {
	input := ListQuery{Provider: "azure"}
	repo := &stubListRepo{}
	_, err := NewService(repo).List(context.Background(), input)
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation || biz.MessageKey != "common.validation" {
		t.Fatalf("expected common.validation, got %#v", err)
	}
	if repo.filter.Page != 0 {
		t.Fatal("repository called for invalid provider")
	}
}

func TestServiceTrimsCommonFilters(t *testing.T) {
	repo := &stubListRepo{}
	_, err := NewService(repo).List(context.Background(), ListQuery{Provider: " gcp ", ProjectID: " project ", Region: " region ", Zone: " zone ", Q: " needle "})
	if err != nil {
		t.Fatal(err)
	}
	f := repo.filter
	if f.Provider != "gcp" || f.ProjectID != "project" || f.Region != "region" || f.Zone != "zone" || f.Q != "needle" {
		t.Fatalf("filter=%#v", f)
	}
}

func TestServiceRejectsInvalidExactFiltersBeforeRepository(t *testing.T) {
	for _, input := range []ListQuery{{IP: "bad"}, {IP: "fe80::1%eth0"}, {TagValue: "prod"}, {TagKey: " ", TagValue: "prod"}} {
		repo := &stubListRepo{}
		_, err := NewService(repo).List(context.Background(), input)
		var biz *apperr.BizError
		if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation || biz.MessageKey != "common.validation" {
			t.Fatalf("input=%#v error=%#v", input, err)
		}
		if repo.filter.Page != 0 {
			t.Fatal("repository called for invalid filter")
		}
	}
}
func TestServicePassesExactFilters(t *testing.T) {
	repo := &stubListRepo{}
	_, err := NewService(repo).List(context.Background(), ListQuery{IP: " 2001:db8::1 ", TagKey: " env ", TagValue: " prod "})
	if err != nil {
		t.Fatal(err)
	}
	if repo.filter.IP != "2001:db8::1" || repo.filter.TagKey != "env" || repo.filter.TagValue != "prod" {
		t.Fatalf("filter=%#v", repo.filter)
	}
}
