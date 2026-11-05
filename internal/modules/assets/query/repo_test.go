//go:build dbtest

package query

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

func TestUnifiedResourceQueries(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "unified-key")
	aws := dbtest.SeedCloudAccount(t, gdb, key.ID, "aws-account", "us-east-1")
	ali := dbtest.SeedCloudAccount(t, gdb, key.ID, "ali-account", "cn-hangzhou")
	gcp := dbtest.SeedCloudAccount(t, gdb, key.ID, "gcp-account", "us-central1")
	require.NoError(t, gdb.Model(ali).Updates(map[string]any{"provider": "aliyun"}).Error)
	require.NoError(t, gdb.Model(gcp).Updates(map[string]any{"provider": "gcp", "project_id": "service-project"}).Error)
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	ip := net.ParseIP("10.0.0.8")
	require.NoError(t, gdb.Create(&models.AWSInstance{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", InstanceID: "i-aws", State: "running", PrivateIP: &ip, Tags: datatypes.JSON(`{"env":"prod"}`), LastSeenAt: now}).Error)
	nics := datatypes.JSON(`[{"id":"nic-1","network_ref":"projects/host/global/networks/shared","private_ipv4":["10.0.0.7"],"public_ipv4":[],"private_ipv6":[],"public_ipv6":[]},{"id":"nic-2","network_ref":"projects/host/global/networks/secondary","private_ipv4":["10.0.0.8","malformed"],"private_ipv6":["2001:0db8:0:0:0:0:0:8","::ffff:10.0.0.9"],"public_ipv4":["203.0.113.8"],"public_ipv6":[]}]`)
	require.NoError(t, gdb.Create(&models.AliyunInstance{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", Zone: "cn-hangzhou-h", InstanceID: "i-ali", NativeStatus: "Running", NetworkInterfaces: nics, Tags: datatypes.JSON(`{"env":"prod"}`), LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPInstance{ID: 42, CloudAccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "18446744073709551615", NativeStatus: "TERMINATED", NativeDetails: datatypes.JSON(`{"network_tags":["web"],"startup_script":"must not be returned"}`), LastSeenAt: now}).Error)
	deleted := models.GCPInstance{ID: 43, CloudAccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "gone", LastSeenAt: now.Add(time.Hour)}
	require.NoError(t, gdb.Create(&deleted).Error)
	require.NoError(t, gdb.Delete(&deleted).Error)
	repo := NewRepo(gdb)
	ctx := context.Background()
	t.Run("combined count page and stable keys", func(t *testing.T) {
		first, total, err := repo.ListInstances(ctx, Filter{Page: 1, Size: 2, IncludeDeleted: true})
		require.NoError(t, err)
		require.EqualValues(t, 4, total)
		require.Len(t, first, 2)
		require.Equal(t, []string{"aliyun:instance:42", "aws:instance:42"}, []string{first[0].AssetKey, first[1].AssetKey})
		second, total, err := repo.ListInstances(ctx, Filter{Page: 2, Size: 2, IncludeDeleted: true})
		require.NoError(t, err)
		require.EqualValues(t, 4, total)
		require.Len(t, second, 2)
		require.Equal(t, "gcp:instance:42", second[0].AssetKey)
		require.Equal(t, "stopped", second[0].State)
		require.Equal(t, "18446744073709551615", second[0].NativeID)
		require.True(t, second[1].Deleted)
		require.Equal(t, "zone", second[0].LocationType)
		require.Equal(t, "service-project", second[0].ProjectID)
	})
	t.Run("exact all interface addresses and tags", func(t *testing.T) {
		for _, address := range []string{"10.0.0.8", "2001:db8::8", "::ffff:10.0.0.8", "10.0.0.9", "203.0.113.8"} {
			rows, total, err := repo.ListInstances(ctx, Filter{IP: address, Page: 1, Size: 20})
			require.NoError(t, err)
			require.NotEmpty(t, rows)
			if address == "10.0.0.8" || address == "::ffff:10.0.0.8" {
				require.EqualValues(t, 2, total)
			} else {
				require.EqualValues(t, 1, total)
			}
		}
		rows, total, err := repo.ListInstances(ctx, Filter{IP: "10.0.0.80", Page: 1, Size: 20})
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, rows)
		_, total, err = repo.ListInstances(ctx, Filter{IP: "203.0.113.8", PrivateIPOnly: true, Page: 1, Size: 20})
		require.NoError(t, err)
		require.Zero(t, total)
		rows, total, err = repo.ListInstances(ctx, Filter{AccountID: ali.ID, TagKey: "env", TagValue: "prod", Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, rows[0].NetworkInterfaces, 2)
		_, total, err = repo.ListInstances(ctx, Filter{TagKey: "web", Page: 1, Size: 20})
		require.NoError(t, err)
		require.Zero(t, total, "network tags are not labels")
	})
	t.Run("network and IP belong to the same interface", func(t *testing.T) {
		for _, privateOnly := range []bool{false, true} {
			for _, test := range []struct {
				name, network, address string
				want                   int64
			}{
				{"other interface network", "projects/host/global/networks/shared", "10.0.0.8", 0},
				{"matching interface network", "projects/host/global/networks/secondary", "10.0.0.8", 1},
				{"network only", "projects/host/global/networks/shared", "", 1},
				{"IP only", "", "10.0.0.8", 1},
			} {
				rows, total, err := repo.ListInstances(ctx, Filter{AccountID: ali.ID, NetworkRef: test.network, IP: test.address, PrivateIPOnly: privateOnly, Page: 1, Size: 20})
				require.NoError(t, err, test.name)
				require.Equal(t, test.want, total, "%s private_only=%t", test.name, privateOnly)
				require.Len(t, rows, int(test.want), test.name)
			}
		}
	})
	t.Run("key details and allowlisted native facts", func(t *testing.T) {
		row, err := repo.FindInstance(ctx, "gcp:instance:42", false)
		require.NoError(t, err)
		require.Equal(t, []string{"web"}, row.NativeDetails.NetworkTags)
		rawJSON, err := json.Marshal(row)
		require.NoError(t, err)
		require.NotContains(t, string(rawJSON), "startup_script")
		require.NotContains(t, string(rawJSON), "must not be returned")
		require.Nil(t, row.NativeDetails.CPUCount)
		require.Nil(t, row.NativeDetails.MemoryMB)
		raw, err := repo.FindInstance(ctx, "42", false)
		require.NoError(t, err)
		require.Equal(t, "aws", raw.Provider)
		_, err = repo.FindInstance(ctx, "gcp:instance:43", false)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		_, err = repo.FindInstance(ctx, "gcp:instance:43", true)
		require.NoError(t, err)
	})
	t.Run("nullable database facts", func(t *testing.T) {
		require.NoError(t, gdb.Create(&models.GCPDatabase{ID: 42, CloudAccountID: gcp.ID, Region: "us-central1", DatabaseID: "database", LastSeenAt: now, NativeDetails: datatypes.JSON(`{"password":"hidden"}`)}).Error)
		rows, total, err := repo.ListDatabases(ctx, Filter{Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Nil(t, rows[0].PubliclyAccessible)
		require.Nil(t, rows[0].StorageGB)
		require.Nil(t, rows[0].HAMode)
		require.Empty(t, rows[0].Endpoints)
	})
	t.Run("subnets use full network reference account and scope", func(t *testing.T) {
		network := models.GCPNetwork{ID: 42, CloudAccountID: gcp.ID, NetworkID: "123", NativeRef: "projects/host/global/networks/shared", Name: "shared", LastSeenAt: now}
		require.NoError(t, gdb.Create(&network).Error)
		for i, ref := range []string{network.NativeRef, "projects/other/global/networks/shared", "123"} {
			require.NoError(t, gdb.Create(&models.GCPSubnet{ID: uint64(i + 1), CloudAccountID: gcp.ID, SubnetID: ref, NetworkRef: ref, Region: "us-central1", LastSeenAt: now}).Error)
		}
		require.NoError(t, gdb.Create(&models.GCPSubnet{ID: 9, CloudAccountID: aws.ID, SubnetID: "foreign", NetworkRef: network.NativeRef, Region: "us-central1", LastSeenAt: now}).Error)
		rows, total, err := repo.ListSubnetsByVPC(ctx, "gcp:vpc:42", Filter{Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, rows, 1)
		require.EqualValues(t, 1, rows[0].ID)
		require.Equal(t, network.NativeRef, rows[0].NetworkRef)
		for _, filter := range []Filter{{Provider: "aws", Page: 1, Size: 20}, {AccountID: aws.ID, Page: 1, Size: 20}, {ProjectID: "other-project", Page: 1, Size: 20}, {Region: "europe-west1", Page: 1, Size: 20}} {
			rows, total, err := repo.ListSubnetsByVPC(ctx, "gcp:vpc:42", filter)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, rows)
		}
	})
	t.Run("native families preserve keys aliases and scope filters", func(t *testing.T) {
		cidr := "10.0.0.0/16"
		require.NoError(t, gdb.Create(&models.AWSVPC{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", VPCID: "vpc-42", CIDRBlock: &cidr, IsDefault: true, State: "available", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AliyunVPC{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", NetworkID: "vpc-ali", NativeRef: "vpc-ali", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AWSSubnet{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", SubnetID: "subnet-42", VPCID: "vpc-42", AvailabilityZone: "us-east-1a", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AliyunVSwitch{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", SubnetID: "vsw-42", NetworkRef: "vpc-ali", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AWSDatabase{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", DBInstanceID: "db-42", Engine: "postgres", Status: "available", Endpoint: "db.example.test", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AliyunDatabase{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", DatabaseID: "db-ali", Engine: "mysql", NativeStatus: "Running", LastSeenAt: now}).Error)
		networks, total, err := repo.ListVPCs(ctx, Filter{Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 3, total)
		require.Equal(t, []string{"aliyun:vpc:42", "aws:vpc:42", "gcp:vpc:42"}, []string{networks[0].AssetKey, networks[1].AssetKey, networks[2].AssetKey})
		awsNetwork, err := repo.FindVPC(ctx, "42", false)
		require.NoError(t, err)
		require.Equal(t, "vpc-42", awsNetwork.VPCID)
		require.NotNil(t, awsNetwork.IsDefault)
		require.True(t, *awsNetwork.IsDefault)
		require.Equal(t, cidr, awsNetwork.CIDRBlock)
		require.Nil(t, networks[2].IsDefault)
		require.Equal(t, "global", networks[2].LocationType)
		require.Empty(t, networks[2].Region)
		for _, key := range []string{"42", "aliyun:vpc:42"} {
			children, total, err := repo.ListSubnetsByVPC(ctx, key, Filter{Page: 1, Size: 20})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.EqualValues(t, 42, children[0].ID)
		}
		subnet, err := repo.FindSubnet(ctx, "42", false)
		require.NoError(t, err)
		require.Equal(t, "subnet-42", subnet.SubnetID)
		require.Equal(t, "vpc-42", subnet.VPCID)
		require.Equal(t, "us-east-1a", subnet.AvailabilityZone)
		databases, total, err := repo.ListDatabases(ctx, Filter{Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 3, total)
		require.Len(t, databases, 3)
		for _, provider := range []string{"aws", "aliyun", "gcp"} {
			database, err := repo.FindDatabase(ctx, provider+":database:42", false)
			require.NoError(t, err)
			require.Equal(t, provider, database.Provider)
		}
		awsDB, err := repo.FindDatabase(ctx, "42", false)
		require.NoError(t, err)
		require.Equal(t, "db-42", awsDB.DBInstanceID)
		require.Equal(t, "db.example.test", awsDB.Endpoint)
		require.NotNil(t, awsDB.MultiAZ)
		require.False(t, *awsDB.MultiAZ)
		filtered, total, err := repo.ListDatabases(ctx, Filter{Engine: "mysql", Status: "Running", Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Equal(t, "aliyun", filtered[0].Provider)
		for _, filter := range []Filter{{Provider: "gcp", AccountID: aws.ID, Page: 1, Size: 20}, {Provider: "aliyun", Region: "us-east-1", Page: 1, Size: 20}, {ProjectID: "service-project", Zone: "other-zone", Page: 1, Size: 20}} {
			rows, total, err := repo.ListInstances(ctx, filter)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, rows)
		}
		rows, total, err := repo.ListInstances(ctx, Filter{Provider: "gcp", AccountID: gcp.ID, ProjectID: "service-project", Region: "us-central1", Zone: "us-central1-a", State: "stopped", Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Equal(t, "gcp:instance:42", rows[0].AssetKey)
	})
	t.Run("account and scope indexes are available to bounded pages", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			sources map[string]string
			filter  Filter
		}{
			{"aws instances", instanceSources, Filter{Provider: "aws", AccountID: aws.ID, Region: "us-east-1", Page: 1, Size: 20}},
			{"aliyun instances", instanceSources, Filter{Provider: "aliyun", AccountID: ali.ID, Region: "cn-hangzhou", Page: 1, Size: 20}},
			{"gcp instances", instanceSources, Filter{Provider: "gcp", AccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", Page: 1, Size: 20}},
			{"gcp subnets", subnetSources, Filter{Provider: "gcp", AccountID: gcp.ID, Region: "us-central1", Page: 1, Size: 20}},
		} {
			err := gdb.Transaction(func(tx *gorm.DB) error {
				// Tiny fixtures favor sequential scans; this local setting checks available access paths only.
				if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
					return err
				}
				where, args := commonWhere(test.filter)
				args = append(args, test.filter.Size, 0)
				rows, err := tx.Raw("EXPLAIN SELECT * FROM ("+union(test.sources, test.filter.Provider)+") assets WHERE "+where+" ORDER BY "+stableOrder+" LIMIT ? OFFSET ?", args...).Rows()
				if err != nil {
					return err
				}
				defer rows.Close()
				var lines []string
				for rows.Next() {
					var line string
					if err := rows.Scan(&line); err != nil {
						return err
					}
					lines = append(lines, line)
				}
				plan := strings.Join(lines, "\n")
				t.Logf("%s:\n%s", test.name, plan)
				require.Contains(t, plan, "Limit")
				require.Contains(t, plan, "Index Cond:")
				require.Contains(t, plan, "cloud_account_id")
				require.Contains(t, plan, "region")
				return rows.Err()
			})
			require.NoError(t, err)
		}
	})
	t.Run("invalid input rejected before database", func(t *testing.T) {
		unavailable := NewRepo(nil)
		for _, f := range []Filter{{Provider: "bogus", Page: 1, Size: 20}, {Page: 1, Size: 201}, {Page: math.MaxInt, Size: 200}, {IP: "broken", Page: 1, Size: 20}, {TagValue: "v", Page: 1, Size: 20}} {
			_, _, err := unavailable.ListInstances(ctx, f)
			require.Error(t, err)
		}
		_, err := unavailable.FindInstance(ctx, "gcp:vpc:42", false)
		require.Error(t, err)
	})
	t.Run("zoned IP rejected before database", func(t *testing.T) {
		f := Filter{IP: "fe80::1%eth0", Page: 1, Size: 20}
		require.Error(t, f.Validate())
		require.NotPanics(t, func() { _, _, err := NewRepo(nil).ListInstances(ctx, f); require.Error(t, err) })
	})
	t.Run("firewalls retain known AWS facts and rule semantics", func(t *testing.T) {
		require.NoError(t, gdb.Create(&models.AWSSecurityGroup{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", GroupID: "sg-42", GroupName: "native-group-name", Name: "display-name", Description: "searchable-description", VPCID: "vpc-42", OwnerID: "123456789012", InboundRules: datatypes.JSON(`[{"protocol":"icmp","from_port":0,"to_port":0,"target_type":"ipv4","target":"0.0.0.0/0"},{"protocol":"tcp","from_port":443,"to_port":443,"target_type":"security_group","target":"sg-peer-in","owner_id":"210987654321","vpc_id":"vpc-peer-in","peering_id":"pcx-in","untrusted_extension":"must not be returned"}]`), OutboundRules: datatypes.JSON(`[{"protocol":"-1","target_type":"security_group","target":"sg-peer-out","owner_id":"345678901234","vpc_id":"vpc-peer-out","peering_id":"pcx-out"}]`), LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AliyunSecurityGroup{ID: 42, CloudAccountID: ali.ID, Region: "cn-hangzhou", FirewallID: "sg-ali", NetworkRef: "vpc-ali", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.GCPFirewall{ID: 42, CloudAccountID: gcp.ID, FirewallID: "123", NetworkRef: "projects/host/global/networks/shared", LastSeenAt: now}).Error)
		rows, total, err := repo.ListFirewalls(ctx, Filter{Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 3, total)
		require.Len(t, rows, 3)
		row, err := repo.FindFirewall(ctx, "42", false)
		require.NoError(t, err)
		raw, err := json.Marshal(row)
		require.NoError(t, err)
		var object map[string]any
		require.NoError(t, json.Unmarshal(raw, &object))
		require.Equal(t, "native-group-name", object["group_name"])
		require.Equal(t, "123456789012", object["owner_id"])
		require.Equal(t, "sg-42", object["group_id"])
		require.Equal(t, "vpc-42", object["vpc_id"])
		require.EqualValues(t, 2, object["inbound_rule_count"])
		require.EqualValues(t, 1, object["outbound_rule_count"])
		nativeFacts := object["native_details"].(map[string]any)
		require.Equal(t, []any{map[string]any{"rule_index": float64(1), "owner_id": "210987654321", "vpc_id": "vpc-peer-in", "peering_id": "pcx-in"}}, nativeFacts["aws_inbound_rule_facts"])
		require.Equal(t, []any{map[string]any{"rule_index": float64(0), "owner_id": "345678901234", "vpc_id": "vpc-peer-out", "peering_id": "pcx-out"}}, nativeFacts["aws_outbound_rule_facts"])
		require.Equal(t, "sg-peer-in", row.InboundRules[1].Target)
		require.Equal(t, "sg-peer-out", row.OutboundRules[0].Target)
		require.NotContains(t, string(raw), "untrusted_extension")
		require.NotContains(t, string(raw), "must not be returned")
		require.NotNil(t, row.InboundRules[0].ICMPType)
		require.Zero(t, *row.InboundRules[0].ICMPType)
		require.NotNil(t, row.InboundRules[0].ICMPCode)
		require.Zero(t, *row.InboundRules[0].ICMPCode)
		require.Nil(t, row.InboundRules[0].FromPort)
		require.Nil(t, row.InboundRules[0].Priority)
		require.Nil(t, row.InboundRules[0].Enabled)
		for _, term := range []string{"native-group-name", "searchable-description"} {
			rows, total, err := repo.ListFirewalls(ctx, Filter{Q: term, Page: 1, Size: 20})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Equal(t, "aws:firewall:42", rows[0].AssetKey)
		}
		native, err := repo.FindFirewall(ctx, "gcp:firewall:42", false)
		require.NoError(t, err)
		raw, err = json.Marshal(native)
		require.NoError(t, err)
		object = map[string]any{}
		require.NoError(t, json.Unmarshal(raw, &object))
		require.NotContains(t, object, "owner_id")
		require.NotContains(t, object, "group_name")
		require.EqualValues(t, 0, object["inbound_rule_count"])
		require.Nil(t, native.NativeDetails.Priority)
		require.Nil(t, native.NativeDetails.Disabled)
		require.NotContains(t, object["native_details"], "aws_inbound_rule_facts")
		require.NotContains(t, object["native_details"], "aws_outbound_rule_facts")
	})
}
