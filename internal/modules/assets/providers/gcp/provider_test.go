package gcp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestPlanDiscoversAuthoritativeZonesBeforeAnyCollection(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.True(t, strings.HasSuffix(r.URL.Path, "/zones"))
		if calls == 1 {
			return 200, `{"items":[{"name":"us-central1-a","region":"https://www.googleapis.com/compute/v1/projects/target-project/regions/us-central1","status":"UP"},{"name":"us-central1-b","region":"projects/target-project/regions/us-central1","status":"DOWN"}],"nextPageToken":"next"}`
		}
		return 200, `{"items":[{"name":"europe-west1-b","region":"projects/target-project/regions/europe-west1"},{"name":"asia-east1-a","region":"projects/target-project/regions/asia-east1"}]}`
	})
	plan, err := New(Options{Transport: transport, RequestTimeout: time.Second}).Plan(context.Background(), models.CloudAccount{Provider: "gcp", ProjectID: "target-project", EnabledRegions: models.StringArray{"us-central1", "europe-west1"}}, key)
	require.NoError(t, err)
	require.Empty(t, plan.ExternalAccountID)
	require.Len(t, plan.Units, 9)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, *tokens)
	counts := map[string]int{}
	zones := []string{}
	for _, unit := range plan.Units {
		counts[unit.ResourceType]++
		if unit.ResourceType == "instance" {
			zones = append(zones, unit.Zone)
			require.Equal(t, "zone", unit.LocationType)
		}
	}
	require.Equal(t, map[string]int{"network": 1, "firewall": 1, "subnet": 2, "database": 2, "instance": 3}, counts)
	require.ElementsMatch(t, []string{"us-central1-a", "us-central1-b", "europe-west1-b"}, zones)
}
func TestPlanFailsClosedForBrokenZoneCatalog(t *testing.T) {
	for _, body := range []string{`{"items":[{"name":"us-central1-a","region":"projects/foreign/regions/us-central1"}]}`, `{"items":[{"name":"us-central1-a","region":"projects/target-project/regions/europe-west1"}]}`, `{"items":[null]}`, `{"nextPageToken":"again"}`} {
		t.Run(body, func(t *testing.T) {
			key, private := fixtureKey(t)
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, body })
			plan, err := New(Options{Transport: transport}).Plan(context.Background(), models.CloudAccount{Provider: "gcp", ProjectID: "target-project", EnabledRegions: models.StringArray{"us-central1"}}, key)
			require.Error(t, err)
			require.Empty(t, plan.Units)
		})
	}
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
		calls++
		if calls == 1 {
			return 200, `{"items":[{"name":"us-central1-a","region":"projects/target-project/regions/us-central1"}],"nextPageToken":"p2"}`
		}
		return 403, `{"error":{"code":403,"message":"secret","errors":[{"reason":"accessNotConfigured"}]}}`
	})
	plan, err := New(Options{Transport: transport}).Plan(context.Background(), models.CloudAccount{Provider: "gcp", ProjectID: "target-project", EnabledRegions: models.StringArray{"us-central1"}}, key)
	require.Error(t, err)
	require.Empty(t, plan.Units)
	require.NotContains(t, err.Error(), "secret")
}

func TestFetchDefersPublishAndFailsClosedForUnsupportedFamilies(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.True(t, strings.HasSuffix(r.URL.Path, "/zones/us-central1-a/instances"))
		return 200, `{}`
	})
	p := New(Options{Transport: transport})
	unit := assetsync.Unit{ResourceType: "instance", LocationType: "zone", Location: "us-central1-a", Region: "us-central1", Zone: "us-central1-a", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}
	snapshot, err := p.Fetch(context.Background(), unit, key)
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NotNil(t, snapshot.Publish)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, *tokens)
	for _, kind := range []string{"database", "firewall"} {
		unit.ResourceType = kind
		snapshot, err := p.Fetch(context.Background(), unit, key)
		require.Error(t, err)
		require.False(t, snapshot.Complete)
		require.Nil(t, snapshot.Publish)
	}
	require.Equal(t, 1, calls)
	for _, mutate := range []func(*assetsync.Unit){func(u *assetsync.Unit) { u.Scope.Provider = "aws" }, func(u *assetsync.Unit) { u.Scope.ProjectID = "../bad" }, func(u *assetsync.Unit) { u.Zone = "europe-west1-a" }, func(u *assetsync.Unit) { u.LocationType = "region" }} {
		bad := unit
		bad.ResourceType = "instance"
		mutate(&bad)
		snapshot, err := p.Fetch(context.Background(), bad, key)
		require.Error(t, err)
		require.False(t, snapshot.Complete)
	}
}
