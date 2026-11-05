package aliyun

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func fixtureUnit(kind string) assetsync.Unit {
	return assetsync.Unit{Key: kind + "/cn-hangzhou", ResourceType: kind, LocationType: "region", Location: "cn-hangzhou", Region: "cn-hangzhou", Scope: account.Scope{AccountID: 1, CloudKeyID: 1, Provider: "aliyun", ExternalAccountID: "1234567890123456", Enabled: true, Revision: 1}}
}

func TestFetchCompletesOnlySupportedSuccessfulUnits(t *testing.T) {
	for _, kind := range []string{"instance", "network", "database", "security_group"} {
		t.Run(kind, func(t *testing.T) {
			transport := instanceFixture(t, nil)
			if kind == "database" {
				transport = databaseFixture(t, nil)
			}
			if kind == "security_group" {
				transport = firewallFixture(t, nil)
			}
			if kind == "network" {
				transport = networkFixture(t, nil)
			}
			snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), fixtureUnit(kind), fixtureKey())
			if err != nil || !snapshot.Complete || snapshot.Publish == nil {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
		})
	}
	for _, kind := range []string{"unknown"} {
		t.Run(kind, func(t *testing.T) {
			transport := fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-acs-action") != "GetCallerIdentity" {
					t.Error("unsupported resource called SDK")
				}
				_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
			})
			snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), fixtureUnit(kind), fixtureKey())
			if err == nil || snapshot.Complete || snapshot.Publish != nil {
				t.Fatalf("unsupported complete %+v %v", snapshot, err)
			}
		})
	}
}

func TestFreshUnitIdentityRejectsRotatedAccountBeforeResourceCalls(t *testing.T) {
	calls := 0
	p := New(Options{Transport: fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-acs-action") != "GetCallerIdentity" {
			t.Fatal("resource called before fresh identity verification")
		}
		id := "1234567890123456"
		if strings.Contains(r.Header.Get("Authorization"), "rotated-id") {
			id = "9999999999999999"
		}
		_, _ = w.Write([]byte(`{"AccountId":"` + id + `"}`))
	})})
	plan, err := p.Plan(context.Background(), models.CloudAccount{EnabledRegions: []string{"cn-hangzhou"}}, fixtureKey())
	if err != nil {
		t.Fatal(err)
	}
	rotated := fixtureKey()
	rotated.AccessKeyID = "rotated-id"
	unit := fixtureUnit("instance")
	unit.Scope.ExternalAccountID = plan.ExternalAccountID
	snapshot, err := p.Fetch(context.Background(), unit, rotated)
	biz, ok := apperr.AsBiz(err)
	if !ok || biz.Code != errs.CodeAssetsIdentityMismatch || snapshot.Complete || snapshot.Publish != nil || calls != 2 {
		t.Fatalf("snapshot=%+v calls=%d err=%v", snapshot, calls, err)
	}
}

func TestFetchFailedIdentityStopsAndFailedResourcesNeverPublish(t *testing.T) {
	for _, action := range []string{"GetCallerIdentity", "DescribeInstances", "DescribeNetworkInterfaces", "DescribeVpcs", "DescribeVSwitches"} {
		t.Run(action, func(t *testing.T) {
			override := func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != action {
					return false
				}
				w.WriteHeader(403)
				_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret raw body"}`))
				return true
			}
			kind := "instance"
			transport := instanceFixture(t, override)
			if action == "DescribeVpcs" || action == "DescribeVSwitches" {
				kind = "network"
				transport = networkFixture(t, override)
			}
			snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), fixtureUnit(kind), fixtureKey())
			if err == nil || snapshot.Complete || snapshot.Publish != nil || strings.Contains(err.Error(), "synthetic") {
				t.Fatalf("unsafe snapshot %+v %v", snapshot, err)
			}
			if (action == "GetCallerIdentity") != errors.Is(err, assetsync.ErrIdentityVerification) {
				t.Fatalf("identity sentinel %v", err)
			}
		})
	}
}

func TestResourceCancellationAndSecondPageTimeoutNeverPublish(t *testing.T) {
	for _, action := range []string{"DescribeInstances", "DescribeInstances/page-two", "DescribeNetworkInterfaces", "DescribeVpcs", "DescribeVSwitches"} {
		for _, deadline := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "/cancel", true: "/timeout"}[deadline], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				override := func(_ http.ResponseWriter, r *http.Request) bool {
					_ = r.ParseForm()
					target := strings.Split(action, "/")[0]
					if r.Header.Get("x-acs-action") != target || (strings.HasSuffix(action, "/page-two") && r.FormValue("PageNumber") != "2") {
						return false
					}
					if !deadline {
						cancel()
					}
					<-r.Context().Done()
					return true
				}
				kind := "instance"
				transport := instanceFixture(t, override)
				if action == "DescribeVpcs" || action == "DescribeVSwitches" {
					kind = "network"
					transport = networkFixture(t, override)
				}
				snapshot, err := New(Options{Transport: transport, RequestTimeout: 100 * time.Millisecond}).Fetch(ctx, fixtureUnit(kind), fixtureKey())
				want := context.Canceled
				if deadline {
					want = context.DeadlineExceeded
				}
				if !errors.Is(err, want) || snapshot.Complete || snapshot.Publish != nil {
					t.Fatalf("unsafe canceled snapshot %+v %v", snapshot, err)
				}
			})
		}
	}
}

func TestDatabaseAndFirewallFreshIdentityGuardPrecedesProductCalls(t *testing.T) {
	for _, kind := range []string{"database", "security_group"} {
		t.Run(kind, func(t *testing.T) {
			p := New(Options{Transport: fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GetCallerIdentity", r.Header.Get("x-acs-action"))
				_, _ = w.Write([]byte(`{"AccountId":"9999999999999999"}`))
			})})
			snapshot, err := p.Fetch(context.Background(), fixtureUnit(kind), fixtureKey())
			biz, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, errs.CodeAssetsIdentityMismatch, biz.Code)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
		})
	}
}
