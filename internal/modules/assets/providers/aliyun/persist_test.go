//go:build dbtest

package aliyun

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
		{"Instances", "aliyun_instances", "instance_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.AliyunInstance, len(ids))
			for i, id := range ids {
				items[i] = models.AliyunInstance{ID: 999999, CloudAccountID: 999999, InstanceID: id, Region: "spoofed", Zone: "native-zone", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishInstances(ctx, tx, scope, loc, at, items)
		}},
		{"Network", "aliyun_vpcs", "network_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.AliyunVPC, len(ids))
			for i, id := range ids {
				items[i] = models.AliyunVPC{ID: 999999, CloudAccountID: 999999, NetworkID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishNetwork(ctx, tx, scope, loc, at, items, nil)
		}},
		{"Databases", "aliyun_databases", "database_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.AliyunDatabase, len(ids))
			for i, id := range ids {
				items[i] = models.AliyunDatabase{ID: 999999, CloudAccountID: 999999, DatabaseID: id, Region: "spoofed", Zone: "native-zone", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishDatabases(ctx, tx, scope, loc, at, items)
		}},
		{"SecurityGroups", "aliyun_security_groups", "firewall_id", "region", func(ctx context.Context, tx *gorm.DB, scope account.Scope, loc resource.Location, at time.Time, ids []string) (int64, int64, error) {
			items := make([]models.AliyunSecurityGroup, len(ids))
			for i, id := range ids {
				items[i] = models.AliyunSecurityGroup{ID: 999999, CloudAccountID: 999999, FirewallID: id, Region: "spoofed", LastSeenAt: at.Add(time.Hour), CreatedAt: at.Add(-time.Hour), UpdatedAt: at.Add(-time.Hour), DeletedAt: gorm.DeletedAt{Time: at, Valid: true}}
			}
			return PublishSecurityGroups(ctx, tx, scope, loc, at, items)
		}},
	}
}

func setupPersistence(t *testing.T) (*gorm.DB, models.CloudAccount) {
	t.Helper()
	gdb, cleanup := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(cleanup)
	key := &models.CredentialCloudKey{Name: "key", Provider: "aliyun", AccessKeyIDEnc: []byte("sealed-ak"), SecretAccessKeyEnc: []byte("sealed-sk")}

	require.NoError(t, gdb.Create(key).Error)
	row := models.CloudAccount{Name: "account", Provider: "aliyun", CloudKeyID: key.ID, Enabled: true, EnabledRegions: models.StringArray{"cn-hangzhou", "cn-shanghai"}}
	require.NoError(t, gdb.Create(&row).Error)
	return gdb, row
}
func location(kind string, other bool) resource.Location {
	region := "cn-hangzhou"
	if other {
		region = "cn-shanghai"
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
			if unit.name == "Databases" || unit.name == "Instances" {
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
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at, []models.AliyunDatabase{{DatabaseID: "before"}})
		return err
	}))
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at.Add(time.Minute), []models.AliyunDatabase{{DatabaseID: "bad", NativeDetails: datatypes.JSON(`[]`)}})
		return err
	})
	require.Error(t, err)
	var live int64
	require.NoError(t, db.Model(&models.AliyunDatabase{}).Count(&live).Error)
	require.EqualValues(t, 1, live)
}

func TestPublishNetworkAtomicChildren(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	loc := location("region", false)
	at := time.Now().UTC().Truncate(time.Microsecond)
	publish := func(when time.Time, vpcs []models.AliyunVPC, switches []models.AliyunVSwitch) (int64, int64, error) {
		var seen, deleted int64
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			seen, deleted, err = PublishNetwork(ctx, tx, scope, loc, when, vpcs, switches)
			return err
		})
		return seen, deleted, err
	}
	vpcs := []models.AliyunVPC{{NetworkID: "vpc-one", NativeRef: "vpc-one"}, {NetworkID: "vpc-two", NativeRef: "vpc-two"}}
	switches := []models.AliyunVSwitch{{ID: 9999, CloudAccountID: 9999, Region: "spoofed", SubnetID: "subnet-one", NetworkRef: "vpc-one", Zone: "cn-hangzhou-h"}, {SubnetID: "subnet-two", NetworkRef: "vpc-two", Zone: "cn-hangzhou-i"}}
	seen, deleted, err := publish(at, vpcs, switches)
	require.NoError(t, err)
	require.EqualValues(t, 4, seen)
	require.Zero(t, deleted)
	// Inputs remain reusable by the adapter after publication.
	require.EqualValues(t, 9999, switches[0].ID)
	require.Equal(t, "spoofed", switches[0].Region)
	var original models.AliyunVSwitch
	require.NoError(t, db.Where("subnet_id = ?", "subnet-two").Take(&original).Error)
	require.Equal(t, loc.Region, original.Region)
	require.Equal(t, "cn-hangzhou-i", original.Zone)
	_, deleted, err = publish(at.Add(time.Minute), vpcs[:1], switches[:1])
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	_, _, err = publish(at.Add(2*time.Minute), vpcs, switches)
	require.NoError(t, err)
	var revived models.AliyunVSwitch
	require.NoError(t, db.Where("subnet_id = ?", "subnet-two").Take(&revived).Error)
	require.Equal(t, original.ID, revived.ID)
	require.Equal(t, original.CreatedAt, revived.CreatedAt)
	// Malformed child details cannot change either the VPC or vSwitch snapshot.
	_, _, err = publish(at.Add(3*time.Minute), []models.AliyunVPC{{NetworkID: "invalid-parent"}}, []models.AliyunVSwitch{{SubnetID: "invalid-child", NativeDetails: datatypes.JSON(`{"bad"`)}})
	require.Error(t, err)
	// A real error in the second table rolls back the earlier VPC upsert.
	require.NoError(t, db.Exec(`CREATE FUNCTION reject_vswitch_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test child rejected'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_vswitch_insert BEFORE INSERT ON aliyun_vswitches FOR EACH ROW EXECUTE FUNCTION reject_vswitch_insert()`).Error)
	_, _, err = publish(at.Add(3*time.Minute), []models.AliyunVPC{{NetworkID: "failed-parent"}}, []models.AliyunVSwitch{{SubnetID: "failed-child"}})
	require.Error(t, err)
	for _, model := range []any{&models.AliyunVPC{}, &models.AliyunVSwitch{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.EqualValues(t, 2, count)
	}
	require.NoError(t, db.Exec("DROP TRIGGER reject_vswitch_insert ON aliyun_vswitches").Error)
	_, deleted, err = publish(at.Add(4*time.Minute), nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 4, deleted)
	// Exercise batching on both sides of the atomic network unit.
	vpcs = make([]models.AliyunVPC, 501)
	switches = make([]models.AliyunVSwitch, 501)
	for i := range vpcs {
		vpcs[i] = models.AliyunVPC{NetworkID: fmt.Sprintf("vpc-%d", i)}
		switches[i] = models.AliyunVSwitch{SubnetID: fmt.Sprintf("subnet-%d", i), NetworkRef: vpcs[i].NetworkID}
	}
	seen, _, err = publish(at.Add(5*time.Minute), vpcs, switches)
	require.NoError(t, err)
	require.EqualValues(t, 1002, seen)
	_, deleted, err = publish(at.Add(6*time.Minute), nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1002, deleted)
}

func TestPublishIgnoresInheritedUnscoped(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	loc := location("region", false)
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, loc, at, []models.AliyunDatabase{{DatabaseID: "historical"}})
		return err
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx.Session(&gorm.Session{PropagateUnscoped: true}).Unscoped(), scope, loc, at.Add(time.Minute), nil)
		return err
	}))
	var historical models.AliyunDatabase
	require.NoError(t, db.Unscoped().First(&historical).Error)
	require.True(t, historical.DeletedAt.Valid)
}

func TestPublishPreservesStructuredFacts(t *testing.T) {
	db, row := setupPersistence(t)
	ctx := context.Background()
	scope := account.CaptureScope(row)
	at := time.Now().UTC().Truncate(time.Microsecond)
	regional := location("region", false)
	instanceLocation := location("region", false)
	endpoints := datatypes.JSON(`[ {"address":"db.internal","port":null,"type":"private"} ]`)
	interfaces := datatypes.JSON(`[ {"id":"nic-1","network_ref":"projects/host-project/global/networks/shared","subnet_ref":"subnet-1","private_ipv4":["10.0.0.1","10.0.0.2"],"public_ipv4":[],"private_ipv6":["fd00::1"],"public_ipv6":[]} ]`)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := PublishDatabases(ctx, tx, scope, regional, at, []models.AliyunDatabase{{DatabaseID: "nullable", Endpoints: endpoints, Tags: datatypes.JSON("null"), NativeDetails: datatypes.JSON(`{"maintenance":"weekly"}`)}})
		if err != nil {
			return err
		}
		_, _, err = PublishInstances(ctx, tx, scope, instanceLocation, at, []models.AliyunInstance{{InstanceID: "multi-nic", NetworkInterfaces: interfaces}})
		return err
	}))
	var database models.AliyunDatabase
	require.NoError(t, db.First(&database).Error)
	require.JSONEq(t, string(endpoints), string(database.Endpoints))
	require.JSONEq(t, "{}", string(database.Tags))
	require.JSONEq(t, `{"maintenance":"weekly"}`, string(database.NativeDetails))
	require.Nil(t, database.StorageGB)
	require.Nil(t, database.HAMode)
	require.Nil(t, database.PubliclyAccessible)
	var instance models.AliyunInstance
	require.NoError(t, db.First(&instance).Error)
	require.JSONEq(t, string(interfaces), string(instance.NetworkInterfaces))
}
