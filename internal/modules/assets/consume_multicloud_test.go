//go:build dbtest

package assets

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestConsumerScopedPrivateIPMatchesAcrossProvidersAndInterfaces(t *testing.T) {
	gdb, closeDB := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(closeDB)
	key := dbtest.SeedCloudKey(t, gdb, "consumer-multicloud")
	aws := dbtest.SeedCloudAccount(t, gdb, key.ID, "aws", "us-east-1")
	ali := dbtest.SeedCloudAccount(t, gdb, key.ID, "ali", "cn-hangzhou")
	gcp := dbtest.SeedCloudAccount(t, gdb, key.ID, "gcp", "us-central1")
	require.NoError(t, gdb.Model(ali).Update("provider", "aliyun").Error)
	require.NoError(t, gdb.Model(gcp).Updates(map[string]any{"provider": "gcp", "project_id": "service-project"}).Error)
	now := time.Now().UTC()
	ip := net.ParseIP("10.0.0.8")
	require.NoError(t, gdb.Create(&models.AWSInstance{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", InstanceID: "i-aws", PrivateIP: &ip, VPCID: "vpc-aws", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AliyunInstance{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", Zone: "cn-hangzhou-h", InstanceID: "i-ali", NetworkInterfaces: datatypes.JSON(`[{"network_ref":"vpc-ali","private_ipv4":["10.0.0.8"]}]`), LastSeenAt: now}).Error)
	network := "projects/host/global/networks/secondary"
	nics := datatypes.JSON(`[{"id":"primary","network_ref":"projects/host/global/networks/primary","subnet_ref":"primary-subnet","private_ipv4":["10.1.0.1"],"public_ipv4":["203.0.113.1"]},{"id":"secondary","network_ref":"projects/host/global/networks/secondary","subnet_ref":"secondary-subnet","private_ipv4":["10.0.0.8"],"private_ipv6":["2001:0db8:0:0:0:0:0:8","::ffff:10.0.0.9"],"public_ipv4":["203.0.113.8"],"public_ipv6":["2001:db8:ffff::8"]},{"id":"repeat","network_ref":"projects/host/global/networks/repeat","private_ipv4":["10.0.0.8"]}]`)
	require.NoError(t, gdb.Create(&models.GCPInstance{ID: 42, CloudAccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "18446744073709551615", Name: "vm", InstanceType: "e2-small", NativeStatus: "RUNNING", NetworkInterfaces: nics, LastSeenAt: now}).Error)
	deleted := models.GCPInstance{CloudAccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "deleted", NetworkInterfaces: nics, LastSeenAt: now.Add(time.Hour)}
	require.NoError(t, gdb.Create(&deleted).Error)
	require.NoError(t, gdb.Delete(&deleted).Error)
	var auditBefore int64
	require.NoError(t, gdb.Table("audit_logs").Count(&auditBefore).Error)
	c := NewConsumer(gdb)
	ctx := context.Background()
	_, err := c.LookupInstanceByPrivateIP(ctx, netip.MustParseAddr("10.0.0.8"))
	require.ErrorIs(t, err, ErrAssetsInstanceAmbiguous)
	for _, scope := range []InstanceScope{{Provider: "gcp"}, {CloudAccountID: &gcp.ID}, {NetworkRef: network}, {Provider: "gcp", CloudAccountID: &gcp.ID, NetworkRef: network}} {
		got, err := c.LookupInstanceByPrivateIPScoped(ctx, netip.MustParseAddr("::ffff:10.0.0.8"), scope)
		require.NoError(t, err)
		require.Equal(t, "gcp:instance:42", got.AssetKey)
		require.Equal(t, "gcp", got.Provider)
		require.Equal(t, "18446744073709551615", got.NativeID)
		require.Equal(t, got.NativeID, got.InstanceID)
		require.Equal(t, "service-project", got.ProjectID)
		require.Equal(t, "zone", got.LocationType)
		require.Equal(t, "us-central1-a", got.Location)
		require.Equal(t, "us-central1-a", got.Zone)
		require.Equal(t, "us-central1", got.Region)
		require.Equal(t, "gcp", got.AccountName)
		require.Equal(t, "10.0.0.8", got.PrivateIP.String())
		require.Equal(t, network, got.NetworkRef)
		require.Equal(t, network, got.VPCID)
		require.Equal(t, "secondary-subnet", got.SubnetID)
		require.Equal(t, "203.0.113.8", got.PublicIP.String())
	}
	for _, address := range []string{"2001:db8::8", "::ffff:10.0.0.9"} {
		got, err := c.LookupInstanceByPrivateIPScoped(ctx, netip.MustParseAddr(address), InstanceScope{Provider: "gcp", NetworkRef: network})
		require.NoError(t, err)
		require.Equal(t, netip.MustParseAddr(address).Unmap(), got.PrivateIP)
		require.Equal(t, network, got.NetworkRef)
		require.Equal(t, "secondary-subnet", got.SubnetID)
	}
	for _, test := range []struct {
		ip    string
		scope InstanceScope
	}{
		{"203.0.113.8", InstanceScope{}}, {"2001:db8:ffff::8", InstanceScope{}}, {"10.0.0.88", InstanceScope{}},
		{"10.0.0.8", InstanceScope{Provider: "gcp", NetworkRef: "projects/host/global/networks/primary"}},
		{"10.0.0.8", InstanceScope{Provider: "gcp", NetworkRef: "secondary"}},
		{"10.0.0.8", InstanceScope{Provider: "gcp", NetworkRef: "123"}},
		{"10.0.0.8", InstanceScope{Provider: "gcp", CloudAccountID: &aws.ID}},
	} {
		_, err := c.LookupInstanceByPrivateIPScoped(ctx, netip.MustParseAddr(test.ip), test.scope)
		require.ErrorIs(t, err, ErrAssetsInstanceNotFound, fmt.Sprintf("%+v", test))
	}
	got, err := c.LookupInstanceByPrivateIPScoped(ctx, netip.MustParseAddr("10.0.0.8"), InstanceScope{Provider: "aws"})
	require.NoError(t, err)
	require.Equal(t, "aws:instance:42", got.AssetKey)
	require.Equal(t, "vpc-aws", got.VPCID)
	got, err = c.LookupInstanceByPrivateIPScoped(ctx, netip.MustParseAddr("10.0.0.8"), InstanceScope{Provider: "aliyun"})
	require.NoError(t, err)
	require.Equal(t, "aliyun:instance:42", got.AssetKey)
	var auditAfter int64
	require.NoError(t, gdb.Table("audit_logs").Count(&auditAfter).Error)
	require.Equal(t, auditBefore, auditAfter)
}
