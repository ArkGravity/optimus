//go:build dbtest

package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/database"
	"github.com/ArkGravity/optimus/internal/modules/assets/instance"
	"github.com/ArkGravity/optimus/internal/modules/assets/vpc"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

func TestFamilyAdaptersReadNativeTablesAndPreserveNullableJSON(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "adapter-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "adapter-account", "us-central1")
	require.NoError(t, gdb.Model(account).Updates(map[string]any{"provider": "gcp", "project_id": "project"}).Error)
	now := time.Now().UTC()
	require.NoError(t, gdb.Create(&models.GCPInstance{ID: 42, CloudAccountID: account.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "123", NativeStatus: "TERMINATED", NetworkInterfaces: datatypes.JSON(`[{"id":"nic0","network_ref":"projects/host/global/networks/shared","private_ipv4":["10.0.0.5","10.0.0.6"],"public_ipv6":["2001:db8::1"]}]`), NativeDetails: datatypes.JSON(`{"cpu_count":4,"token":"must-not-leak"}`), LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPNetwork{ID: 42, CloudAccountID: account.ID, NetworkID: "456", NativeRef: "projects/host/global/networks/shared", CIDRBlocks: datatypes.JSON(`["10.0.0.0/16","2001:db8::/48"]`), LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPDatabase{ID: 42, CloudAccountID: account.ID, Region: "us-central1", DatabaseID: "sql", Name: "native sql", Endpoints: datatypes.JSON(`[{"address":"10.0.0.7","type":"PRIVATE","port":null},{"address":"db.example","type":"PUBLIC","port":5432}]`), LastSeenAt: now}).Error)
	instances, total, err := instance.NewRepo(gdb).List(context.Background(), instance.ListFilter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "123", instances[0].InstanceID)
	require.Equal(t, "stopped", instances[0].State)
	require.Equal(t, "TERMINATED", instances[0].NativeStatus)
	require.Len(t, instances[0].NetworkInterfaces, 1)
	require.Equal(t, []string{"10.0.0.5", "10.0.0.6"}, instances[0].NetworkInterfaces[0].PrivateIPv4)
	require.Equal(t, []string{"2001:db8::1"}, instances[0].NetworkInterfaces[0].PublicIPv6)
	rawInstance, err := json.Marshal(instances[0])
	require.NoError(t, err)
	require.NotContains(t, string(rawInstance), "must-not-leak")
	networks, total, err := vpc.NewRepo(gdb).List(context.Background(), vpc.ListFilter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	databases, total, err := database.NewRepo(gdb).List(context.Background(), database.ListFilter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, []string{"10.0.0.0/16", "2001:db8::/48"}, networks[0].CIDRBlocks)
	require.Len(t, databases[0].Endpoints, 2)
	rawDatabase, err := json.Marshal(databases[0])
	require.NoError(t, err)
	var databaseObject map[string]any
	require.NoError(t, json.Unmarshal(rawDatabase, &databaseObject))
	require.Equal(t, "native sql", databaseObject["name"])
	require.Nil(t, databases[0].Endpoints[0].Port)
	for _, test := range []struct {
		row     any
		unknown []string
	}{{networks[0], []string{"is_default"}}, {databases[0], []string{"multi_az", "publicly_accessible", "storage_gb", "ha_mode"}}} {
		raw, err := json.Marshal(test.row)
		require.NoError(t, err)
		var object map[string]any
		require.NoError(t, json.Unmarshal(raw, &object))
		require.Equal(t, "gcp", object["provider"])
		require.Equal(t, "project", object["project_id"])
		for _, key := range test.unknown {
			require.Nil(t, object[key], "%s must be absent or null: %s", key, raw)
		}
	}
}

func TestFamilyKeyAdaptersScopeSharedVPCAndKeepProviderIdentity(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	credential := dbtest.SeedCloudKey(t, gdb, "key-seam")
	account := dbtest.SeedCloudAccount(t, gdb, credential.ID, "key-account", "us-central1")
	require.NoError(t, gdb.Model(account).Updates(map[string]any{"provider": "gcp", "project_id": "service-project"}).Error)
	now := time.Now().UTC()
	ref := "projects/host-project/global/networks/shared"
	require.NoError(t, gdb.Create(&models.GCPNetwork{ID: 42, CloudAccountID: account.ID, NetworkID: "12345678901234567890", NativeRef: ref, LastSeenAt: now}).Error)
	for _, row := range []models.GCPSubnet{
		{CloudAccountID: account.ID, SubnetID: "match-central", NetworkRef: ref, Region: "us-central1", LastSeenAt: now},
		{CloudAccountID: account.ID, SubnetID: "match-east", NetworkRef: ref, Region: "us-east1", LastSeenAt: now},
		{CloudAccountID: account.ID, SubnetID: "wrong-host", NetworkRef: "projects/other-host/global/networks/shared", Region: "us-central1", LastSeenAt: now},
	} {
		require.NoError(t, gdb.Create(&row).Error)
	}
	service := vpc.NewService(vpc.NewRepo(gdb))
	parent, err := service.FindByKey(context.Background(), "gcp:vpc:42", false)
	require.NoError(t, err)
	require.Equal(t, "12345678901234567890", parent.NativeID)
	children, err := service.ListSubnetsByVPCKey(context.Background(), "gcp:vpc:42", vpc.ListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 2, children.Total)
	for _, child := range children.Items {
		raw, err := json.Marshal(child)
		require.NoError(t, err)
		var object map[string]any
		require.NoError(t, json.Unmarshal(raw, &object))
		require.Equal(t, "gcp", object["provider"])
		require.Equal(t, "service-project", object["project_id"])
		require.Equal(t, "key-account", object["cloud_account_name"])
		require.Equal(t, ref, object["network_ref"])
	}
	filtered, err := service.ListSubnetsByVPCKey(context.Background(), "gcp:vpc:42", vpc.ListQuery{Region: "us-east1"})
	require.NoError(t, err)
	require.EqualValues(t, 1, filtered.Total)
	require.Equal(t, "match-east", filtered.Items[0].SubnetID)
	_, err = service.FindByKey(context.Background(), "42", false)
	require.Error(t, err, "legacy IDs must read AWS only")
	// The same table row ID must not conflate AWS and GCP resources.
	require.NoError(t, gdb.Create(&models.AWSVPC{ID: 42, CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-aws", LastSeenAt: now}).Error)
	aws, err := service.FindByKey(context.Background(), "42", false)
	require.NoError(t, err)
	require.Equal(t, "aws", aws.Provider)
	require.Equal(t, "aws:vpc:42", aws.AssetKey)
	capacity := int64(1) << 40
	require.NoError(t, gdb.Create(&models.GCPDatabase{ID: 42, CloudAccountID: account.ID, Region: "us-central1", DatabaseID: "large", StorageGB: &capacity, LastSeenAt: now}).Error)
	databaseRow, err := database.NewService(database.NewRepo(gdb)).FindByKey(context.Background(), "gcp:database:42", false)
	require.NoError(t, err)
	require.Nil(t, databaseRow.StorageGB, "legacy int32 alias must not truncate native capacity")
	raw, err := json.Marshal(databaseRow)
	require.NoError(t, err)
	require.Contains(t, string(raw), fmt.Sprintf(`"storage_gb":%d`, capacity))
	require.NoError(t, gdb.Create(&models.GCPInstance{ID: 42, CloudAccountID: account.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "18446744073709551615", LastSeenAt: now}).Error)
	instanceRow, err := instance.NewService(instance.NewRepo(gdb)).FindByKey(context.Background(), "gcp:instance:42", false)
	require.NoError(t, err)
	require.Equal(t, "18446744073709551615", instanceRow.InstanceID)
	require.Equal(t, "gcp:instance:42", instanceRow.AssetKey)
}
