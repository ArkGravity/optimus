package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/require"
)

func TestSecurityGroupFetcherPaginationAndRules(t *testing.T) {
	calls := 0
	client := newEC2Client(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "DescribeSecurityGroups", r.Form.Get("Action"))
		calls++
		assertPaginationRequest(t, r, calls, "second")
		w.Header().Set("Content-Type", "text/xml")
		if calls == 1 {
			_, _ = w.Write([]byte(`<DescribeSecurityGroupsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><nextToken>second</nextToken><securityGroupInfo><item><groupId>sg-1</groupId><groupName>web</groupName><groupDescription>web firewall</groupDescription><vpcId>vpc-1</vpcId><ownerId>123</ownerId><tagSet><item><key>Name</key><value>frontend</value></item></tagSet><ipPermissions><item><ipProtocol>tcp</ipProtocol><fromPort>443</fromPort><toPort>443</toPort><ipRanges><item><cidrIp>10.0.0.0/8</cidrIp><description>office</description></item></ipRanges><ipv6Ranges><item><cidrIpv6>::/0</cidrIpv6></item></ipv6Ranges></item></ipPermissions><ipPermissionsEgress><item><ipProtocol>-1</ipProtocol><ipRanges><item><cidrIp>0.0.0.0/0</cidrIp></item></ipRanges></item></ipPermissionsEgress></item></securityGroupInfo></DescribeSecurityGroupsResponse>`))
		} else {
			_, _ = w.Write([]byte(`<DescribeSecurityGroupsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><securityGroupInfo><item><groupId>sg-2</groupId><groupName>empty</groupName></item></securityGroupInfo></DescribeSecurityGroupsResponse>`))
		}
	}))
	items, err := (SecurityGroupFetcher{}).FetchSecurityGroups(t.Context(), &Clients{EC2: client})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "frontend", items[0].Name)
	require.Equal(t, "vpc-1", items[0].VPCID)
	require.Equal(t, "123", items[0].OwnerID)
	var inbound, outbound []models.SecurityGroupRule
	require.NoError(t, json.Unmarshal(items[0].InboundRules, &inbound))
	require.NoError(t, json.Unmarshal(items[0].OutboundRules, &outbound))
	require.Len(t, inbound, 2)
	require.Equal(t, "office", inbound[0].Description)
	require.Equal(t, "ipv6", inbound[1].TargetType)
	require.Equal(t, int32(443), *inbound[0].FromPort)
	require.Nil(t, outbound[0].FromPort)
	require.JSONEq(t, "[]", string(items[1].InboundRules))
}

func TestSecurityGroupFetcherRejectsPartialAndRepeatedPages(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "repeated", true: "failed-page"}[fail], func(t *testing.T) {
			calls := 0
			client := newEC2Client(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls > 1 && fail {
					http.Error(w, "denied", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(`<DescribeSecurityGroupsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><nextToken>same</nextToken><securityGroupInfo><item><groupId>sg-1</groupId></item></securityGroupInfo></DescribeSecurityGroupsResponse>`))
			}))
			items, err := (SecurityGroupFetcher{}).FetchSecurityGroups(context.Background(), &Clients{EC2: client})
			require.Error(t, err)
			require.Nil(t, items)
		})
	}
	_, err := (SecurityGroupFetcher{}).FetchSecurityGroups(t.Context(), nil)
	require.Error(t, err)
}

func TestSecurityGroupRuleReferencesAndICMP(t *testing.T) {
	rules := securityGroupRules([]ec2types.IpPermission{{
		IpProtocol: aws.String("icmp"), FromPort: aws.Int32(8), ToPort: aws.Int32(0),
		PrefixListIds:    []ec2types.PrefixListId{{PrefixListId: aws.String("pl-1"), Description: aws.String("managed")}},
		UserIdGroupPairs: []ec2types.UserIdGroupPair{{GroupId: aws.String("sg-peer"), UserId: aws.String("owner"), VpcId: aws.String("vpc-peer"), VpcPeeringConnectionId: aws.String("pcx-1")}},
	}})
	require.Len(t, rules, 2)
	require.Equal(t, "prefix_list", rules[0].TargetType)
	require.Equal(t, int32(0), *rules[0].ToPort)
	require.Equal(t, "sg-peer", rules[1].Target)
	require.Equal(t, "pcx-1", rules[1].PeeringID)
	require.Equal(t, "owner", rules[1].OwnerID)
}
