//go:build dbtest

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
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/stretchr/testify/require"
)

type engineConsumer struct {
	raw    []byte
	keys   []*credentials.CloudKey
	copies [][]byte
}

func (c *engineConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	raw := append([]byte(nil), c.raw...)
	// Rotating the credential's informational home project never redirects calls.
	if len(c.keys) > 0 {
		raw = []byte(strings.ReplaceAll(string(raw), `"project_id":"home-project"`, `"project_id":"rotated-home"`))
	}
	key := &credentials.CloudKey{Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSON: raw}
	c.keys = append(c.keys, key)
	c.copies = append(c.copies, raw)
	return key, nil
}
func TestEngineSDKZoneSnapshotsPreserveFailedScopeAndWipeKeys(t *testing.T) {
	gdb, row := setupPersistence(t)
	key, private := fixtureKey(t)
	consumer := &engineConsumer{raw: key.ServiceAccountJSON}
	old := time.Now().Add(-time.Hour)
	for _, zone := range []string{"us-central1-a", "us-central1-b", "europe-west1-b"} {
		require.NoError(t, gdb.Create(&models.GCPInstance{CloudAccountID: row.ID, Region: strings.TrimSuffix(zone, zone[len(zone)-2:]), Zone: zone, InstanceID: "old-" + zone, LastSeenAt: old}).Error)
	}
	require.NoError(t, gdb.Create(&models.GCPNetwork{CloudAccountID: row.ID, NetworkID: "old-network", LastSeenAt: old}).Error)
	require.NoError(t, gdb.Create(&models.GCPSubnet{CloudAccountID: row.ID, Region: "us-central1", SubnetID: "old-subnet", LastSeenAt: old}).Error)
	calls := map[string]int{}
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls[r.URL.Path]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/zones"):
			return 200, `{"items":[{"name":"us-central1-a","region":"projects/target-project/regions/us-central1"},{"name":"us-central1-b","region":"projects/target-project/regions/us-central1","status":"DOWN"},{"name":"europe-west1-b","region":"projects/target-project/regions/europe-west1"}]}`
		case strings.HasSuffix(r.URL.Path, "/global/networks"):
			return 200, `{"items":[{"id":"18446744073709551615","name":"network"}]}`
		case strings.HasSuffix(r.URL.Path, "/global/firewalls"), strings.HasPrefix(r.URL.Path, "/v1/projects/"):
			return 200, `{}`
		case strings.HasSuffix(r.URL.Path, "/subnetworks"):
			return 200, `{"warning":{"code":"NO_RESULTS_ON_PAGE"}}`
		case strings.HasSuffix(r.URL.Path, "/zones/us-central1-a/instances"):
			return 200, `{"warning":{"code":"NO_RESULTS_ON_PAGE"}}`
		case strings.HasSuffix(r.URL.Path, "/zones/us-central1-b/instances"):
			if r.URL.Query().Get("pageToken") == "" {
				return 200, `{"items":[{"id":"123","name":"partial","zone":"zones/us-central1-b"}],"nextPageToken":"p2"}`
			}
			return 403, `{"error":{"code":403,"message":"private-key access-token JWT body"}}`
		case strings.HasSuffix(r.URL.Path, "/zones/europe-west1-b/instances"):
			return 200, `{"items":[{"id":"456","name":"new-vm","zone":"zones/europe-west1-b","status":"TERMINATED"}]}`
		default:
			t.Fatalf("unexpected API %s", r.URL)
			return 500, `{}`
		}
	})
	registry, err := assetsync.NewRegistry(New(Options{Transport: transport}))
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, consumer, time.Second, time.Minute)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "manual", nil))
	var instances []models.GCPInstance
	require.NoError(t, gdb.Order("zone").Find(&instances).Error)
	require.Len(t, instances, 2)
	require.Equal(t, "456", instances[0].InstanceID)
	require.Equal(t, "old-us-central1-b", instances[1].InstanceID)
	var networks []models.GCPNetwork
	require.NoError(t, gdb.Find(&networks).Error)
	require.Len(t, networks, 1)
	require.Equal(t, "18446744073709551615", networks[0].NetworkID)
	var subnets int64
	require.NoError(t, gdb.Model(&models.GCPSubnet{}).Count(&subnets).Error)
	require.Zero(t, subnets)
	require.Equal(t, 1, calls["/compute/v1/projects/target-project/global/networks"])
	var recorded []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&recorded).Error)
	require.Len(t, recorded, 10)
	for _, run := range recorded {
		require.NotContains(t, run.Error, "private-key")
		require.NotContains(t, run.Error, "JWT")
		if run.Location == "us-central1-b" {
			require.Equal(t, "failed", run.Status)
			require.EqualValues(t, 43202, run.ErrorCode)
		}
		if run.Location == "us-central1-a" {
			require.Equal(t, "success", run.Status)
			require.EqualValues(t, 1, run.ItemsSoftDeleted)
		}
	}
	require.NoError(t, gdb.First(&row, row.ID).Error)
	require.Empty(t, row.ExternalAccountID)
	require.Len(t, consumer.keys, 10)
	for i, key := range consumer.keys {
		require.Nil(t, key.ServiceAccountJSON)
		require.Empty(t, key.Provider)
		for _, b := range consumer.copies[i] {
			require.Zero(t, b)
		}
	}
}
func TestSDKDeferredSnapshotRejectsChangedProjectAndRevivesStableID(t *testing.T) {
	gdb, row := setupPersistence(t)
	key, private := fixtureKey(t)
	unit := assetsync.Unit{ResourceType: "instance", LocationType: "zone", Location: "us-central1-a", Region: "us-central1", Zone: "us-central1-a", Scope: account.CaptureScope(row)}
	empty := false
	transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
		if empty {
			return 200, `{}`
		}
		return 200, `{"items":[{"id":"123","name":"vm","zone":"zones/us-central1-a"}]}`
	})
	p := New(Options{Transport: transport})
	publish := func() {
		snapshot, err := p.Fetch(context.Background(), unit, key)
		require.NoError(t, err)
		_, err = assetsync.PublishSnapshot(context.Background(), gdb, unit, snapshot)
		require.NoError(t, err)
	}
	publish()
	var original models.GCPInstance
	require.NoError(t, gdb.First(&original).Error)
	empty = true
	publish()
	empty = false
	publish()
	var revived models.GCPInstance
	require.NoError(t, gdb.First(&revived).Error)
	require.Equal(t, original.ID, revived.ID)
	snapshot, err := p.Fetch(context.Background(), unit, key)
	require.NoError(t, err)
	// Simulate a changed binding between collection and the deferred transaction.
	// The service forbids project edits; the publisher independently rechecks it.
	require.NoError(t, gdb.Model(&row).Update("project_id", "different-project").Error)
	_, err = assetsync.PublishSnapshot(context.Background(), gdb, unit, snapshot)
	require.ErrorIs(t, err, account.ErrScopeIneligible)
}
func TestEngineZoneCatalogFailureNeverPrunesExistingRows(t *testing.T) {
	gdb, row := setupPersistence(t)
	key, private := fixtureKey(t)
	old := models.GCPInstance{CloudAccountID: row.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "old", LastSeenAt: time.Now().Add(-time.Hour)}
	require.NoError(t, gdb.Create(&old).Error)
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		require.True(t, strings.HasSuffix(r.URL.Path, "/zones"))
		return 403, `{"error":{"code":403,"errors":[{"reason":"accessNotConfigured"}],"message":"secret-api-body"}}`
	})
	registry, err := assetsync.NewRegistry(New(Options{Transport: transport}))
	require.NoError(t, err)
	engine := assetsync.NewProviderEngine(gdb, runs.NewRepo(gdb), registry, &engineConsumer{raw: key.ServiceAccountJSON}, time.Second, time.Minute)
	require.Error(t, engine.RunAccount(context.Background(), row.ID, "manual", nil))
	var after models.GCPInstance
	require.NoError(t, gdb.First(&after, old.ID).Error)
	require.Equal(t, old.InstanceID, after.InstanceID)
	var recorded []models.AssetsSyncRun
	require.NoError(t, gdb.Find(&recorded).Error)
	require.Len(t, recorded, 1)
	require.EqualValues(t, 43207, recorded[0].ErrorCode)
}
