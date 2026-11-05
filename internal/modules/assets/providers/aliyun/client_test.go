package aliyun

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureTransport(t *testing.T, handler http.HandlerFunc) http.RoundTripper {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	return transportFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return server.Client().Transport.RoundTrip(clone)
	})
}

func fixtureKey() *credentials.CloudKey {
	return &credentials.CloudKey{Provider: "aliyun", AuthType: "access_key", AccessKeyID: "synthetic-access-id", SecretAccessKey: "synthetic-secret"}
}

func TestPlanUsesSignedExplicitIdentityAndFourUnits(t *testing.T) {
	calls := 0
	transport := fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-acs-action") != "GetCallerIdentity" || !strings.Contains(r.Header.Get("Authorization"), "synthetic-access-id") {
			t.Errorf("missing SDK action or explicit signature")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
	})
	p := New(Options{Transport: transport})
	plan, err := p.Plan(context.Background(), models.CloudAccount{EnabledRegions: []string{"cn-hangzhou", "cn-shanghai"}}, fixtureKey())
	if err != nil || calls != 1 || plan.ExternalAccountID != "1234567890123456" || len(plan.Units) != 8 {
		t.Fatalf("plan=%+v calls=%d err=%v", plan, calls, err)
	}
	for _, unit := range plan.Units {
		if unit.LocationType != "region" || unit.Region != unit.Location || unit.Budget <= 0 {
			t.Fatalf("invalid unit %+v", unit)
		}
	}
}

func TestInvalidAuthMakesNoSDKCalls(t *testing.T) {
	p := New(Options{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected request"); return nil, nil })})
	for _, key := range []*credentials.CloudKey{nil, {}, {Provider: "aws", AuthType: "access_key", AccessKeyID: "a", SecretAccessKey: "s"}, {Provider: "aliyun", AuthType: "service_account_json"}} {
		if _, err := p.Plan(context.Background(), models.CloudAccount{EnabledRegions: []string{"cn-hangzhou"}}, key); err == nil {
			t.Fatal("accepted invalid auth")
		}
	}
}

func TestIdentityCancellationSurvivesRepeatedSanitization(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deadline {
				var c context.CancelFunc
				ctx, c = context.WithTimeout(ctx, 20*time.Millisecond)
				defer c()
			}
			p := New(Options{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if !deadline {
					cancel()
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})})
			_, err := p.Fetch(ctx, assetsync.Unit{Region: "cn-hangzhou", Scope: account.Scope{ExternalAccountID: "1234567890123456"}}, fixtureKey())
			want := context.Canceled
			if deadline {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || !errors.Is(err, assetsync.ErrIdentityVerification) || !errors.Is(safeError(safeError(err)), want) {
				t.Fatalf("lost sentinel: %v", err)
			}
			if strings.Contains(err.Error(), "synthetic") {
				t.Fatalf("secret leaked: %v", err)
			}
		})
	}
}

func TestRegionCatalogAuthenticatedDeduplicatedAndValidated(t *testing.T) {
	catalog := RegionCatalog{Transport: fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-acs-action") != "DescribeRegions" || r.Header.Get("Authorization") == "" {
			t.Error("unauthenticated catalog")
		}
		_, _ = w.Write([]byte(`{"Regions":{"Region":[{"RegionId":"cn-shanghai","LocalName":"Shanghai"},{"RegionId":"cn-hangzhou"},{"RegionId":"cn-shanghai"}]}}`))
	})}
	items, err := catalog.Regions(context.Background(), fixtureKey(), "")
	if err != nil || len(items) != 2 || items[0].ID != "cn-hangzhou" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestResourceDeadlineSurvivesSDKWrapping(t *testing.T) {
	for _, kind := range []string{"instance", "network", "database", "security_group"} {
		t.Run(kind, func(t *testing.T) {
			transport := fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-acs-action") == "GetCallerIdentity" {
					_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
					return
				}
				<-r.Context().Done()
			})
			p := New(Options{Transport: transport, RequestTimeout: 30 * time.Millisecond})
			snapshot, err := p.Fetch(context.Background(), fixtureUnit(kind), fixtureKey())
			if snapshot.Complete || snapshot.Publish != nil || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("resource deadline lost: %+v %v", snapshot, err)
			}
		})
	}
}

func TestRegionCatalogRejectsMalformedAndPermissionFailures(t *testing.T) {
	for _, payload := range []string{`{}`, `{"Regions":{"Region":[{}]}}`, `{"Regions":{"Region":[{"RegionId":"http://arbitrary.invalid"}]}}`, `{`} {
		t.Run(payload, func(t *testing.T) {
			catalog := RegionCatalog{Transport: fixtureTransport(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) })}
			items, err := catalog.Regions(context.Background(), fixtureKey(), "")
			if err == nil || len(items) != 0 {
				t.Fatalf("accepted malformed catalog %+v %v", items, err)
			}
		})
	}
	for _, status := range []int{401, 403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			catalog := RegionCatalog{Transport: fixtureTransport(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
			})}
			items, err := catalog.Regions(context.Background(), fixtureKey(), "")
			if err == nil || len(items) != 0 || calls > 3 || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("unsafe catalog items=%v calls=%d err=%v", items, calls, err)
			}
		})
	}
}
