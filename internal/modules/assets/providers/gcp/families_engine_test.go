//go:build dbtest

package gcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/stretchr/testify/require"
)

func TestEngineAllGCPFamiliesSDKToNativeTables(t *testing.T) {
	gdb, row := setupPersistence(t)
	key, private := fixtureKey(t)
	mode := "complete"
	calls := map[string]int{}
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls[r.URL.Path]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/zones"):
			return 200, `{"items":[{"name":"us-central1-a","region":"regions/us-central1"},{"name":"europe-west1-b","region":"regions/europe-west1"}]}`
		case strings.HasSuffix(r.URL.Path, "/global/networks"):
			return 200, `{"items":[{"id":"1","name":"main"}]}`
		case strings.HasSuffix(r.URL.Path, "/subnetworks"):
			parts := strings.Split(r.URL.Path, "/")
			region := parts[len(parts)-2]
			return 200, fmt.Sprintf(`{"items":[{"id":"101","name":"subnet","region":"regions/%s","network":"global/networks/main","ipCidrRange":"10.0.0.0/24"}]}`, region)
		case strings.Contains(r.URL.Path, "/zones/"):
			parts := strings.Split(r.URL.Path, "/")
			zone := parts[len(parts)-2]
			return 200, fmt.Sprintf(`{"items":[{"id":"201","name":"vm","zone":"zones/%s","networkInterfaces":[{"network":"global/networks/main","networkIP":"10.0.0.2"}]}]}`, zone)
		case strings.HasSuffix(r.URL.Path, "/global/firewalls"):
			switch mode {
			case "loop":
				return 200, `{"nextPageToken":"again"}`
			case "page-failure":
				return 403, `{"error":{"code":403,"message":"secret-firewall","errors":[{"reason":"accessNotConfigured"}]}}`
			case "warning":
				return 200, `{"warning":{"code":"UNREACHABLE","message":"secret-warning"}}`
			case "empty":
				return 200, `{"warning":{"code":"NO_RESULTS_ON_PAGE"}}`
			}
			return 200, `{"items":[{"id":"999","name":"allow-web","network":"global/networks/main","priority":0,"allowed":[{"IPProtocol":"tcp","ports":["80","443"]}]}]}`
		case strings.HasPrefix(r.URL.Path, "/v1/projects/"):
			if mode == "loop" {
				return 200, `{"nextPageToken":"again"}`
			}
			if mode == "warning" {
				return 200, `{"warnings":[{"code":"REGION_UNREACHABLE","region":"europe-west1","message":"secret-warning"}]}`
			}
			if r.URL.Query().Get("pageToken") == "" {
				return 200, `{"items":[{"name":"external-db","project":"target-project","instanceType":"ON_PREMISES_INSTANCE"},{"name":"east-db","project":"target-project","region":"europe-west1","databaseVersion":"MYSQL_8_0"}],"nextPageToken":"p2"}`
			}
			if mode == "page-failure" {
				return 403, `{"error":{"code":403,"message":"secret-page-two"}}`
			}
			if mode == "empty" {
				return 200, `{}`
			}
			return 200, `{"items":[{"name":"central-db","project":"target-project","region":"us-central1","databaseVersion":"POSTGRES_16","ipAddresses":[{"type":"PRIVATE","ipAddress":"10.0.0.2"},{"type":"OUTGOING","ipAddress":"203.0.113.7"}],"connectionName":"target-project:us-central1:central-db","settings":{"dataDiskSizeGb":"1024","availabilityType":"REGIONAL"}}]}`
		default:
			t.Fatalf("unexpected API %s", r.URL)
			return 500, `{}`
		}
	})
	tokenFailure := false
	wrapped := transportFunc(func(r *http.Request) (*http.Response, error) {
		if tokenFailure && r.URL.String() == tokenURL {
			return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: &observedBody{Reader: strings.NewReader(`{"error":"invalid_grant","error_description":"secret-token"}`)}}, nil
		}
		return transport.RoundTrip(r)
	})
	p := New(Options{Transport: wrapped})
	registry, err := assetsync.NewRegistry(p)
	require.NoError(t, err)
	consumer := &engineConsumer{raw: key.ServiceAccountJSON}
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, time.Minute)
	run := func() { require.NoError(t, engine.RunAccount(context.Background(), row.ID, "manual", nil)) }
	run()
	var original []models.GCPDatabase
	require.NoError(t, gdb.Order("database_id").Find(&original).Error)
	require.Len(t, original, 2)
	var firewall models.GCPFirewall
	require.NoError(t, gdb.First(&firewall).Error)
	require.Empty(t, firewall.Region)
	for _, family := range []struct {
		model any
		want  int64
	}{{&models.GCPNetwork{}, 1}, {&models.GCPSubnet{}, 2}, {&models.GCPInstance{}, 2}} {
		var count int64
		require.NoError(t, gdb.Model(family.model).Count(&count).Error)
		require.Equal(t, family.want, count)
	}
	require.Equal(t, 1, calls["/compute/v1/projects/target-project/global/firewalls"])
	require.Equal(t, 4, calls["/v1/projects/target-project/instances"], "complete project pagination repeats once per selected region")
	var recorded []models.AssetsSyncRun
	require.NoError(t, gdb.Find(&recorded).Error)
	require.Len(t, recorded, 9)
	for _, run := range recorded {
		require.Equal(t, "success", run.Status)
	}
	items, total, err := query.NewRepo(gdb).ListDatabases(context.Background(), query.Filter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	for _, item := range items {
		if item.NativeID == "central-db" {
			require.Len(t, item.Endpoints, 1)
			require.Nil(t, item.Endpoints[0].Port)
			require.Len(t, item.NativeDetails.GCPIPAddresses, 2)
		}
	}
	for _, failure := range []string{"page-failure", "warning", "loop", "token"} {
		mode = failure
		tokenFailure = failure == "token"
		require.Error(t, engine.RunAccount(context.Background(), row.ID, "manual", nil))
		var after []models.GCPDatabase
		require.NoError(t, gdb.Order("database_id").Find(&after).Error)
		require.Equal(t, original, after)
		var afterFirewall models.GCPFirewall
		require.NoError(t, gdb.First(&afterFirewall).Error)
		require.Equal(t, firewall, afterFirewall)
	}
	tokenFailure = false
	mode = "empty"
	run()
	var remaining []models.GCPDatabase
	require.NoError(t, gdb.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "europe-west1", remaining[0].Region)
	var count int64
	require.NoError(t, gdb.Model(&models.GCPFirewall{}).Count(&count).Error)
	require.Zero(t, count)
	mode = "complete"
	run()
	var revived []models.GCPDatabase
	require.NoError(t, gdb.Order("database_id").Find(&revived).Error)
	require.Len(t, revived, 2)
	require.Equal(t, original[0].ID, revived[0].ID)
	require.Equal(t, original[1].ID, revived[1].ID)
	var revivedFirewall models.GCPFirewall
	require.NoError(t, gdb.First(&revivedFirewall).Error)
	require.Equal(t, firewall.ID, revivedFirewall.ID)
	// Deferred closures recheck captured scope independently of successful I/O.
	unit := assetsync.Unit{ResourceType: "database", LocationType: "region", Location: "us-central1", Region: "us-central1", Scope: account.CaptureScope(row)}
	snapshot, err := p.Fetch(context.Background(), unit, key)
	require.NoError(t, err)
	require.NoError(t, gdb.Model(&row).Updates(map[string]any{"enabled_regions": models.StringArray{"europe-west1"}, "scope_revision": row.ScopeRevision + 1}).Error)
	_, err = assetsync.PublishSnapshot(context.Background(), gdb, unit, snapshot)
	require.ErrorIs(t, err, account.ErrScopeIneligible)
	require.NoError(t, gdb.First(&revivedFirewall).Error)
	require.NoError(t, gdb.Find(&recorded).Error)
	for _, run := range recorded {
		require.NotContains(t, run.Error, "secret")
		require.NotContains(t, run.Error, "JWT")
	}
	for _, key := range consumer.keys {
		require.Nil(t, key.ServiceAccountJSON)
	}
}
