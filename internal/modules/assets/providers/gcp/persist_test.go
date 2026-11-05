//go:build dbtest

package gcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type publication struct {
	name, table, nativeColumn, kind string
	publish                         func(context.Context, *gorm.DB, account.Scope, resource.Location, time.Time, []string) (int64, int64, error)
}

func publications() []publication {
	return []publication{
		{"Instances", "gcp_instances", "instance_id", "zone", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.GCPInstance, len(ids))
			for i, id := range ids {
				items[i] = models.GCPInstance{ID: 999999, CloudAccountID: 999999, InstanceID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishInstances(ctx, tx, scope, loc, at, items)
		}},
		{"Networks", "gcp_networks", "network_id", "global", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.GCPNetwork, len(ids))
			for i, id := range ids {
				items[i] = models.GCPNetwork{ID: 999999, CloudAccountID: 999999, NetworkID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishNetworks(ctx, tx, scope, loc, at, items)
		}},
		{"Subnets", "gcp_subnets", "subnet_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.GCPSubnet, len(ids))
			for i, id := range ids {
				items[i] = models.GCPSubnet{ID: 999999, CloudAccountID: 999999, SubnetID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishSubnets(ctx, tx, scope, loc, at, items)
		}},
		{"Databases", "gcp_databases", "database_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.GCPDatabase, len(ids))
			for i, id := range ids {
				items[i] = models.GCPDatabase{ID: 999999, CloudAccountID: 999999, DatabaseID: id, Region: "spoofed", Zone: "native-zone", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishDatabases(ctx, tx, scope, loc, at, items)
		}},
		{"Firewalls", "gcp_firewalls", "firewall_id", "global", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.GCPFirewall, len(ids))
			for i, id := range ids {
				items[i] = models.GCPFirewall{ID: 999999, CloudAccountID: 999999, FirewallID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishFirewalls(ctx, tx, scope, loc, at, items)
		}},
	}
}

func setupPersistence(t *testing.T) (*gorm.DB, models.CloudAccount) {
	t.Helper()
	gdb, cleanup := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(cleanup)
	key := &models.CredentialCloudKey{Name: "key", Provider: "gcp", AccessKeyIDEnc: []byte("sealed-ak"), SecretAccessKeyEnc: []byte("sealed-sk")}
	key.AuthType = "service_account_json"
	key.AccessKeyIDEnc = nil
	key.SecretAccessKeyEnc = nil
	key.ServiceAccountJSONEnc = []byte("sealed-json")
	key.ServiceAccountEmail = "reader@example.test"
	key.CredentialProjectID = "home-project"
	require.NoError(t, gdb.Create(key).Error)
	row := models.CloudAccount{Name: "account", Provider: "gcp", CloudKeyID: key.ID, Enabled: true, EnabledRegions: models.StringArray{"us-central1", "europe-west1"}, ProjectID: "target-project"}
	require.NoError(t, gdb.Create(&row).Error)
	return gdb, row
}
func location(kind string, other bool) resource.Location {
	region := "us-central1"
	if other {
		region = "europe-west1"
	}
	switch kind {
	case "global":
		return resource.Location{Type: "global", Name: "global"}
	case "zone":
		return resource.Location{Type: "zone", Name: region + "-a", Zone: region + "-a", Region: region}
	default:
		return resource.Location{Type: "region", Name: region, Region: region}
	}
}
func TestPublishLifecycle(t *testing.T) {
	for _, unit := range publications() {
		t.Run(unit.name, func(t *testing.T) {
			db, row := setupPersistence(t)
			ctx := context.Background()
			scope := account.CaptureScope(row)
			loc := location(unit.kind, false)
			at := time.Now().UTC().Truncate(time.Microsecond)
			_, _, err := unit.publish(ctx, db, scope, loc, at, []string{"standalone"})
			require.ErrorIs(t, err, gorm.ErrInvalidTransaction)
			invalid := loc
			invalid.Name = "invalid"
			require.Error(t, db.Transaction(func(tx *gorm.DB) error { _, _, err := unit.publish(ctx, tx, scope, invalid, at, nil); return err }))

			publish := func(s account.Scope, l resource.Location, when time.Time, ids ...string) (int64, int64, error) {
				var seen, deleted int64
				err := db.Transaction(func(tx *gorm.DB) error {
					var err error
					seen, deleted, err = unit.publish(ctx, tx, s, l, when, ids)
					return err
				})
				return seen, deleted, err
			}
			seen, deleted, err := publish(scope, loc, at, "one", "two")
			require.NoError(t, err)
			require.EqualValues(t, 2, seen)
			require.Zero(t, deleted)
			type stored struct {
				ID                               uint64
				CloudAccountID                   uint64
				Region, Zone                     string
				LastSeenAt, CreatedAt, UpdatedAt time.Time
				DeletedAt                        *time.Time
			}
			var first stored
			require.NoError(t, db.Table(unit.table).Where(unit.nativeColumn+" = ?", "one").Take(&first).Error)
			require.NotEqualValues(t, 999999, first.ID)
			require.Equal(t, row.ID, first.CloudAccountID)
			require.Equal(t, loc.Region, first.Region)
			require.WithinDuration(t, at, first.LastSeenAt, time.Microsecond)
			require.WithinDuration(t, at, first.CreatedAt, time.Microsecond)
			require.WithinDuration(t, at, first.UpdatedAt, time.Microsecond)
			require.Nil(t, first.DeletedAt)
			var jsonShapes struct{ Tags, NativeDetails string }
			require.NoError(t, db.Table(unit.table).Select("jsonb_typeof(tags) AS tags, jsonb_typeof(native_details) AS native_details").Where("id = ?", first.ID).Take(&jsonShapes).Error)
			require.Equal(t, "object", jsonShapes.Tags)
			require.Equal(t, "object", jsonShapes.NativeDetails)
			arrays := map[string][]string{"Instances": {"network_interfaces"}, "Network": {"cidr_blocks"}, "Networks": {"cidr_blocks"}, "Subnets": {"cidr_blocks"}, "Databases": {"endpoints"}, "SecurityGroups": {"inbound_rules", "outbound_rules"}, "Firewalls": {"inbound_rules", "outbound_rules"}}
			for _, column := range arrays[unit.name] {
				var shape string
				require.NoError(t, db.Table(unit.table).Select("jsonb_typeof("+column+")").Where("id = ?", first.ID).Scan(&shape).Error)
				require.Equal(t, "array", shape)
			}

			if unit.kind == "zone" {
				require.Equal(t, loc.Zone, first.Zone)
			}
			if unit.name == "Databases" {
				require.Equal(t, "native-zone", first.Zone)
			}
			var original stored
			require.NoError(t, db.Table(unit.table).Where(unit.nativeColumn+" = ?", "two").Take(&original).Error)
			other := row
			other.ID = 0
			other.Name = "other-account"
			require.NoError(t, db.Create(&other).Error)
			_, _, err = publish(account.CaptureScope(other), loc, at, "unrelated")
			require.NoError(t, err)
			if unit.kind != "global" {
				_, _, err = publish(scope, location(unit.kind, true), at, "elsewhere")
				require.NoError(t, err)
			}
			seen, deleted, err = publish(scope, loc, at.Add(time.Minute), "one")
			require.NoError(t, err)
			require.EqualValues(t, 1, seen)
			require.EqualValues(t, 1, deleted)
			seen, deleted, err = publish(scope, loc, at.Add(2*time.Minute), "one")
			require.NoError(t, err)
			require.EqualValues(t, 1, seen)
			require.Zero(t, deleted)
			_, _, err = publish(scope, loc, at.Add(3*time.Minute), "one", "two")
			require.NoError(t, err)
			var revived stored
			require.NoError(t, db.Table(unit.table).Where("id = ?", original.ID).Take(&revived).Error)
			require.Equal(t, original.ID, revived.ID)
			require.Equal(t, original.CreatedAt, revived.CreatedAt)
			require.Nil(t, revived.DeletedAt)
			seen, deleted, err = publish(scope, loc, at.Add(4*time.Minute))
			require.NoError(t, err)
			require.Zero(t, seen)
			require.EqualValues(t, 2, deleted)
			var live int64
			require.NoError(t, db.Table(unit.table).Where("deleted_at IS NULL").Count(&live).Error)
			expected := int64(1)
			if unit.kind != "global" {
				expected++
			}
			require.Equal(t, expected, live)
			// A snapshot timestamp cannot prune rows from a newer successful sweep.
			_, _, err = publish(scope, loc, at.Add(6*time.Minute), "newer")
			require.NoError(t, err)
			_, deleted, err = publish(scope, loc, at.Add(5*time.Minute))
			require.NoError(t, err)
			require.Zero(t, deleted)
			// More than 500 rows exercises batch boundaries and the conflict path.
			ids := make([]string, 501)
			for i := range ids {
				ids[i] = fmt.Sprintf("batch-%d", i)
			}
			seen, _, err = publish(scope, loc, at.Add(7*time.Minute), ids...)
			require.NoError(t, err)
			require.EqualValues(t, 501, seen)
			_, deleted, err = publish(scope, loc, at.Add(8*time.Minute), ids...)
			require.NoError(t, err)
			require.Zero(t, deleted)
			// Every family rejects invalid captures before creating or pruning anything.
			for _, change := range []func(*account.Scope){func(s *account.Scope) { s.Revision++ }, func(s *account.Scope) { s.Provider = "aws" }, func(s *account.Scope) { s.CloudKeyID++ }, func(s *account.Scope) { s.ProjectID = "wrong" }, func(s *account.Scope) { s.ExternalAccountID = "wrong" }, func(s *account.Scope) { s.Enabled = false }} {
				stale := scope
				change(&stale)
				_, _, err = publish(stale, loc, at.Add(9*time.Minute), "forbidden")
				require.Error(t, err)
			}
			require.NoError(t, db.Table(unit.table).Where("cloud_account_id = ? AND deleted_at IS NULL", row.ID).Count(&live).Error)
			expectedLive := int64(501)
			if unit.kind != "global" {
				expectedLive++
			}
			require.Equal(t, expectedLive, live)
			require.NoError(t, db.Table(unit.table).Where(unit.nativeColumn+" = ?", "forbidden").Count(&live).Error)
			require.Zero(t, live)
			// Outer rollback restores both upserts and pruning.
			marker := errors.New("rollback")
			require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
				_, _, err := unit.publish(ctx, tx, scope, loc, at.Add(10*time.Minute), []string{"rolled-back"})
				require.NoError(t, err)
				return marker
			}), marker)
			require.NoError(t, db.Table(unit.table).Where(unit.nativeColumn+" = ?", "rolled-back").Count(&live).Error)
			require.Zero(t, live)
			require.NoError(t, db.Model(&row).Update("enabled", false).Error)
			_, _, err = publish(scope, loc, at.Add(11*time.Minute))
			require.Error(t, err)
			require.NoError(t, db.Model(&row).Update("enabled", true).Error)
			require.NoError(t, db.Delete(&row).Error)
			_, _, err = publish(scope, loc, at.Add(12*time.Minute))
			require.Error(t, err)
		})
	}
}

func TestPublishInvalidJSONRollsBack(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	loc := location("region", false)
	at := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at, []models.GCPDatabase{{DatabaseID: "before"}})
		return err
	}))
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at.Add(time.Minute), []models.GCPDatabase{{DatabaseID: "bad", NativeDetails: datatypes.JSON(`[]`)}})
		return err
	})
	require.Error(t, err)
	var live int64
	require.NoError(t, db.Model(&models.GCPDatabase{}).Count(&live).Error)
	require.EqualValues(t, 1, live)
}

func TestPublishGlobalAndRegionalIndependence(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	at := time.Now().UTC().Truncate(time.Microsecond)
	global := location("global", false)
	regional := location("region", false)
	ref := "projects/host-project/global/networks/shared-vpc"
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishNetworks(ctx, tx, scope, global, at, []models.GCPNetwork{{NetworkID: "12345678901234567890", NativeRef: ref, Region: "spoofed"}})
		if err != nil {
			return err
		}
		_, _, err = PublishFirewalls(ctx, tx, scope, global, at, []models.GCPFirewall{{FirewallID: "456", NetworkRef: ref, Region: "spoofed"}})
		if err != nil {
			return err
		}
		_, _, err = PublishSubnets(ctx, tx, scope, regional, at, []models.GCPSubnet{{SubnetID: "789", NetworkRef: ref, Zone: "spoofed"}})
		return err
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, deleted, err := PublishSubnets(ctx, tx, scope, regional, at.Add(time.Minute), nil)
		require.EqualValues(t, 1, deleted)
		return err
	}))
	var network models.GCPNetwork
	require.NoError(t, db.First(&network).Error)
	require.Equal(t, ref, network.NativeRef)
	require.Empty(t, network.Region)
	var firewall models.GCPFirewall
	require.NoError(t, db.First(&firewall).Error)
	require.Equal(t, ref, firewall.NetworkRef)
	require.Empty(t, firewall.Region)
	// A region change retains global families, but invalidates captures by revision.
	require.NoError(t, db.Model(&row).Updates(map[string]any{"enabled_regions": models.StringArray{"europe-west1"}, "scope_revision": gorm.Expr("scope_revision + 1")}).Error)
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishNetworks(ctx, tx, scope, global, at.Add(2*time.Minute), nil)
		return err
	}), account.ErrScopeIneligible)
	require.NoError(t, db.First(&row, row.ID).Error)
	scope = account.CaptureScope(row)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, deleted, err := PublishNetworks(ctx, tx, scope, global, at.Add(3*time.Minute), nil)
		require.EqualValues(t, 1, deleted)
		return err
	}))
	require.NoError(t, db.First(&firewall).Error)
	// The same native VM ID in two zones is independent within one region.
	for _, zone := range []string{"europe-west1-a", "europe-west1-b"} {
		loc := resource.Location{Type: "zone", Name: zone, Region: "europe-west1", Zone: zone}
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			_, _, err := PublishInstances(ctx, tx, scope, loc, at, []models.GCPInstance{{InstanceID: "same"}})
			return err
		}))
	}
	loc := resource.Location{Type: "zone", Name: "europe-west1-a", Region: "europe-west1", Zone: "europe-west1-a"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, deleted, err := PublishInstances(ctx, tx, scope, loc, at.Add(time.Minute), nil)
		require.EqualValues(t, 1, deleted)
		return err
	}))
	var remaining models.GCPInstance
	require.NoError(t, db.First(&remaining).Error)
	require.Equal(t, "europe-west1-b", remaining.Zone)
}

func TestPublishIgnoresInheritedUnscoped(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	loc := location("region", false)
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at, []models.GCPDatabase{{DatabaseID: "historical"}})
		return err
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx.Session(&gorm.Session{PropagateUnscoped: true}).Unscoped(), scope, loc, at.Add(time.Minute), nil)
		return err
	}))
	var historical models.GCPDatabase
	require.NoError(t, db.Unscoped().First(&historical).Error)
	require.True(t, historical.DeletedAt.Valid)
}

func TestPublishPreservesStructuredFacts(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	at := time.Now().UTC().Truncate(time.Microsecond)
	regional := location("region", false)
	instanceLocation := location("zone", false)
	endpoints := datatypes.JSON(`[ {"address":"db.internal","port":null,"type":"private"} ]`)
	interfaces := datatypes.JSON(`[ {"id":"nic-1","network_ref":"projects/host-project/global/networks/shared","subnet_ref":"subnet-1","private_ipv4":["10.0.0.1","10.0.0.2"],"public_ipv4":[],"private_ipv6":["fd00::1"],"public_ipv6":[]} ]`)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, regional, at, []models.GCPDatabase{{DatabaseID: "nullable", Endpoints: endpoints, Tags: datatypes.JSON("null"), NativeDetails: datatypes.JSON(`{"maintenance":"weekly"}`)}})
		if err != nil {
			return err
		}
		_, _, err = PublishInstances(ctx, tx, scope, instanceLocation, at, []models.GCPInstance{{InstanceID: "multi-nic", NetworkInterfaces: interfaces}})
		return err
	}))
	var database models.GCPDatabase
	require.NoError(t, db.First(&database).Error)
	require.JSONEq(t, string(endpoints), string(database.Endpoints))
	require.JSONEq(t, "{}", string(database.Tags))
	require.JSONEq(t, `{"maintenance":"weekly"}`, string(database.NativeDetails))
	require.Nil(t, database.StorageGB)
	require.Nil(t, database.HAMode)
	require.Nil(t, database.PubliclyAccessible)
	var instance models.GCPInstance
	require.NoError(t, db.First(&instance).Error)
	require.JSONEq(t, string(interfaces), string(instance.NetworkInterfaces))
}

func TestPublishInstancesRejectsMismatchedZoneRegion(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, db.Model(&row).Update("enabled_regions", models.StringArray{"us-central1"}).Error)
	scope := account.CaptureScope(row)
	for _, item := range []models.GCPInstance{
		{CloudAccountID: row.ID, InstanceID: "central-existing", Region: "us-central1", Zone: "us-central1-a", LastSeenAt: at},
		{CloudAccountID: row.ID, InstanceID: "foreign-existing", Region: "europe-west1", Zone: "europe-west1-a", LastSeenAt: at},
	} {
		require.NoError(t, db.Create(&item).Error)
	}
	var before []models.GCPInstance
	require.NoError(t, db.Unscoped().Order("id").Find(&before).Error)
	for _, zone := range []string{"europe-west1-a", "us-central1-"} {
		t.Run(zone, func(t *testing.T) {
			loc := resource.Location{Type: "zone", Name: zone, Region: "us-central1", Zone: zone}
			err := db.Transaction(func(tx *gorm.DB) error {
				_, _, err := PublishInstances(ctx, tx, scope, loc, at.Add(time.Minute), []models.GCPInstance{{InstanceID: "must-not-write"}})
				return err
			})
			require.ErrorIs(t, err, account.ErrScopeIneligible)
			var after []models.GCPInstance
			require.NoError(t, db.Unscoped().Order("id").Find(&after).Error)
			require.Equal(t, before, after, "inconsistent location must neither write nor prune snapshots")
		})
	}
}
