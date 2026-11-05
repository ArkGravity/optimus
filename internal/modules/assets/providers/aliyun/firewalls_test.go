package aliyun

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/stretchr/testify/require"
)

const sgList = `{"PageNumber":1,"PageSize":100,"TotalCount":1,"RegionId":"cn-hangzhou","SecurityGroups":{"SecurityGroup":[{"SecurityGroupId":"sg-a","SecurityGroupName":"synthetic group","VpcId":"vpc-a","Description":"group description","SecurityGroupType":"normal","Tags":{"Tag":[{"TagKey":"team","TagValue":"synthetic"}]}}]}}`
const sgDetail = `{"SecurityGroupId":"sg-a","RegionId":"cn-hangzhou","VpcId":"vpc-a","NextToken":"rule-page-2","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-a","Direction":"ingress","Policy":"Accept","IpProtocol":"tcp","PortRange":"22/22","Priority":"1","SourceCidrIp":"0.0.0.0/0","SourcePortRange":"1000/2000","NicType":"intranet"},{"SecurityGroupRuleId":"sgr-b","Direction":"ingress","Policy":"Drop","IpProtocol":"icmp","PortRange":"0/0","Ipv6SourceCidrIp":"2001:db8::/64","Priority":"100"}]}}`
const sgDetail2 = `{"SecurityGroupId":"sg-a","RegionId":"cn-hangzhou","VpcId":"vpc-a","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-c","Direction":"egress","Policy":"Accept","IpProtocol":"all","PortRange":"-1/-1","DestGroupId":"sg-other","DestGroupOwnerAccount":"owner@example.invalid","DestGroupName":"other group","Priority":"5"},{"SecurityGroupRuleId":"sgr-d","Direction":"ingress","Policy":"Accept","IpProtocol":"tcp","PortRange":"-1/-1","SourcePrefixListId":"pl-a","SourcePrefixListName":"prefix name"}]}}`

func firewallFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	return fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if override != nil && override(w, r) {
			return
		}
		payload := ""
		switch r.Header.Get("x-acs-action") {
		case "GetCallerIdentity":
			payload = `{"AccountId":"1234567890123456"}`
		case "DescribeSecurityGroups":
			payload = sgList
		case "DescribeSecurityGroupAttribute":
			if r.FormValue("Direction") != "all" || r.FormValue("MaxResults") != "1000" {
				t.Errorf("unbounded/directional rule request %v", r.Form)
			}
			payload = sgDetail
			if r.FormValue("NextToken") == "rule-page-2" {
				payload = sgDetail2
			}
		default:
			t.Errorf("unexpected SDK action %s", r.Header.Get("x-acs-action"))
			http.Error(w, "unexpected", 500)
			return
		}
		_, _ = w.Write([]byte(payload))
	})
}
func TestFirewallSnapshotRequiresAllRulePages(t *testing.T) {
	p := New(Options{Transport: firewallFixture(t, nil)})
	snapshot, err := p.Fetch(context.Background(), fixtureUnit("security_group"), fixtureKey())
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NotNil(t, snapshot.Publish)
	rows, err := p.firewalls(context.Background(), fixtureKey(), "cn-hangzhou")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var in, out []resource.Rule
	require.NoError(t, json.Unmarshal(rows[0].InboundRules, &in))
	require.NoError(t, json.Unmarshal(rows[0].OutboundRules, &out))
	require.Len(t, in, 3)
	require.Len(t, out, 1)
	require.EqualValues(t, 22, *in[0].FromPort)
	require.Equal(t, "deny", in[1].Action)
	require.EqualValues(t, 0, *in[1].ICMPType)
	require.EqualValues(t, 0, *in[1].ICMPCode)
	require.Nil(t, in[1].FromPort)
	require.Nil(t, in[2].Priority)
	require.Nil(t, in[2].Enabled)
	require.EqualValues(t, -1, *in[2].FromPort)
	require.Equal(t, "prefix_list", in[2].TargetType)
	require.Equal(t, "security_group", out[0].TargetType)
	var details query.FirewallNativeDetails
	require.NoError(t, json.Unmarshal(rows[0].NativeDetails, &details))
	require.Len(t, details.AliyunInboundRuleFacts, 3)
	require.Equal(t, "1000/2000", details.AliyunInboundRuleFacts[0].SourcePortRange)
	require.EqualValues(t, 0, details.AliyunOutboundRuleFacts[0].RuleIndex)
	require.Equal(t, "owner@example.invalid", details.AliyunOutboundRuleFacts[0].DestinationGroupOwnerAccount)
	require.Equal(t, "prefix name", details.AliyunInboundRuleFacts[2].SourcePrefixListName)

}
func TestFirewallsRejectIncompleteListAndRulePages(t *testing.T) {
	for _, mode := range []string{"rule-page-failed", "repeated-token", "duplicate-rule-id", "wrong-group", "missing-group", "wrong-direction", "truncated-list", "duplicate-group", "unexpected-token", "malformed-rule", "bad-priority", "bad-port"} {
		t.Run(mode, func(t *testing.T) {
			p := New(Options{Transport: firewallFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				action := r.Header.Get("x-acs-action")
				payload := ""
				if action == "DescribeSecurityGroups" {
					switch mode {
					case "truncated-list":
						payload = strings.Replace(sgList, `"TotalCount":1`, `"TotalCount":2`, 1)
					case "duplicate-group":
						payload = strings.Replace(sgList, `"TotalCount":1`, `"TotalCount":2`, 1)
						payload = strings.Replace(payload, `"SecurityGroup":[`, `"SecurityGroup":[{"SecurityGroupId":"sg-a"},`, 1)
					case "unexpected-token":
						payload = strings.Replace(sgList, `"PageNumber":1`, `"PageNumber":1,"NextToken":"unexpected"`, 1)
					}
				}
				if action == "DescribeSecurityGroupAttribute" {
					switch mode {
					case "rule-page-failed":
						if r.FormValue("NextToken") != "" {
							w.WriteHeader(403)
							_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
							return true
						}
					case "repeated-token":
						payload = sgDetail
					case "duplicate-rule-id":
						if r.FormValue("NextToken") != "" {
							payload = strings.Replace(sgDetail2, "sgr-c", "sgr-a", 1)
						}
					case "wrong-group":
						payload = strings.Replace(sgDetail, "sg-a", "sg-other", 1)
					case "missing-group":
						payload = strings.Replace(sgDetail, `"SecurityGroupId":"sg-a",`, "", 1)
					case "wrong-direction":
						payload = strings.Replace(sgDetail, "ingress", "unknown", 1)
					case "malformed-rule":
						payload = `{"SecurityGroupId":"sg-a","Permissions":{"Permission":[null]}}`
					case "bad-priority":
						payload = strings.Replace(sgDetail, `"Priority":"1"`, `"Priority":"NaN"`, 1)
					case "bad-port":
						payload = strings.Replace(sgDetail, `"22/22"`, `"65536/22"`, 1)
					}
				}
				if payload == "" {
					return false
				}
				_, _ = w.Write([]byte(payload))
				return true
			})})
			snapshot, err := p.Fetch(context.Background(), fixtureUnit("security_group"), fixtureKey())
			require.Error(t, err)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
			require.NotContains(t, err.Error(), "synthetic-secret")
		})
	}
}

func TestFirewallDisplayEntriesRetainIndexedNativeTargets(t *testing.T) {
	p := New(Options{Transport: firewallFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("x-acs-action") != "DescribeSecurityGroupAttribute" || r.FormValue("NextToken") != "" {
			return false
		}
		payload := strings.Replace(sgDetail, `"SourceCidrIp":"0.0.0.0/0"`, `"SourceCidrIp":"0.0.0.0/0","Ipv6SourceCidrIp":"::/0","SourceGroupId":"sg-source","SourceGroupName":"source group","SourceGroupOwnerAccount":"source-owner","SourcePrefixListId":"pl-source","SourcePrefixListName":"source prefix"`, 1)
		_, _ = w.Write([]byte(payload))
		return true
	})})
	rows, err := p.firewalls(context.Background(), fixtureKey(), "cn-hangzhou")
	require.NoError(t, err)
	var in []resource.Rule
	var details query.FirewallNativeDetails
	require.NoError(t, json.Unmarshal(rows[0].InboundRules, &in))
	require.NoError(t, json.Unmarshal(rows[0].NativeDetails, &details))
	require.Len(t, in, 6)
	require.Len(t, details.AliyunInboundRuleFacts, 6)
	for i := 0; i < 4; i++ {
		require.Equal(t, i, details.AliyunInboundRuleFacts[i].RuleIndex)
		require.Equal(t, "sgr-a", details.AliyunInboundRuleFacts[i].RuleID)
		require.Equal(t, "sg-source", details.AliyunInboundRuleFacts[i].SourceGroupID)
		require.Equal(t, "source-owner", details.AliyunInboundRuleFacts[i].SourceGroupOwnerAccount)
		require.EqualValues(t, 22, *in[i].FromPort)
	}
	require.Equal(t, "ipv4", in[0].TargetType)
	require.Equal(t, "ipv6", in[1].TargetType)
	require.Equal(t, "security_group", in[2].TargetType)
	require.Equal(t, "prefix_list", in[3].TargetType)
}

func TestFirewallListCompletesMultiplePagesBeforeRuleCollection(t *testing.T) {
	var listed atomic.Int32
	p := New(Options{Transport: firewallFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Header.Get("x-acs-action") {
		case "DescribeSecurityGroups":
			listed.Add(1)
			payload := `{"PageNumber":1,"PageSize":1,"TotalCount":2,"SecurityGroups":{"SecurityGroup":[{"SecurityGroupId":"sg-a","VpcId":"vpc-a"}]}}`
			if r.FormValue("PageNumber") == "2" {
				payload = strings.Replace(payload, `"PageNumber":1`, `"PageNumber":2`, 1)
				payload = strings.Replace(payload, "sg-a", "sg-b", 1)
			}
			_, _ = w.Write([]byte(payload))
			return true
		case "DescribeSecurityGroupAttribute":
			require.EqualValues(t, 2, listed.Load())
			payload := sgDetail
			if r.FormValue("NextToken") != "" {
				payload = sgDetail2
			}
			payload = strings.Replace(payload, "sg-a", r.FormValue("SecurityGroupId"), 1)
			_, _ = w.Write([]byte(payload))
			return true
		}
		return false
	})})
	rows, err := p.firewalls(context.Background(), fixtureKey(), "cn-hangzhou")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "sg-b", rows[1].FirewallID)
}

func classicFirewallFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	return fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if override != nil && override(w, r) {
			return
		}
		switch r.Header.Get("x-acs-action") {
		case "GetCallerIdentity":
			_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
			return
		case "DescribeSecurityGroups":
			_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":100,"TotalCount":1,"SecurityGroups":{"SecurityGroup":[{"SecurityGroupId":"sg-classic","SecurityGroupName":"classic group","VpcId":"","SecurityGroupType":"normal"}]}}`))
			return
		case "DescribeSecurityGroupAttribute":
			require.Equal(t, "all", r.FormValue("Direction"))
			require.Equal(t, "1000", r.FormValue("MaxResults"))
			require.Equal(t, "sg-classic", r.FormValue("SecurityGroupId"))
			require.Equal(t, "cn-hangzhou", r.FormValue("RegionId"))
			key := r.FormValue("NicType") + "/" + r.FormValue("NextToken")
			payload := ""
			switch key {
			case "intranet/":
				payload = `{"SecurityGroupId":"sg-classic","VpcId":"","InnerAccessPolicy":"Accept","NextToken":"intranet-p2","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-private-in","NicType":"intranet","Direction":"ingress","Policy":"Accept","IpProtocol":"tcp","PortRangeListId":"prl-private","PortRangeListName":"private services","SourceGroupId":"sg-private-source","SourceGroupName":"private source","SourceGroupOwnerAccount":"private-owner"}]}}`
			case "intranet/intranet-p2":
				payload = `{"SecurityGroupId":"sg-classic","VpcId":"","InnerAccessPolicy":"Accept","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-private-out","NicType":"intranet","Direction":"egress","Policy":"Drop","IpProtocol":"udp","PortRange":"53/53","DestCidrIp":"10.0.0.0/8","DestGroupId":"sg-private-target","DestGroupName":"private target","DestGroupOwnerAccount":"private-target-owner"}]}}`
			case "internet/":
				payload = `{"SecurityGroupId":"sg-classic","VpcId":"","InnerAccessPolicy":"Accept","NextToken":"internet-p2","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-public-in","NicType":"internet","Direction":"ingress","Policy":"Accept","IpProtocol":"icmp","PortRange":"0/0","SourceCidrIp":"198.51.100.0/24"}]}}`
			case "internet/internet-p2":
				payload = `{"SecurityGroupId":"sg-classic","VpcId":"","InnerAccessPolicy":"Accept","Permissions":{"Permission":[{"SecurityGroupRuleId":"sgr-public-out","NicType":"internet","Direction":"egress","Policy":"Accept","IpProtocol":"tcp","PortRangeListId":"prl-public","PortRangeListName":"public services","DestPrefixListId":"pl-public","DestPrefixListName":"public destinations"}]}}`
			default:
				t.Errorf("unexpected classic NIC/page %s", key)
				http.Error(w, "unexpected", 500)
				return
			}
			_, _ = w.Write([]byte(payload))
			return
		default:
			t.Errorf("unexpected SDK action %s", r.Header.Get("x-acs-action"))
			http.Error(w, "unexpected", 500)
		}
	})
}

func TestClassicFirewallCollectsBothNICScopesAllDirectionsAndPortRangeLists(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	p := New(Options{Transport: classicFirewallFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("x-acs-action") == "DescribeSecurityGroupAttribute" {
			key := r.FormValue("NicType") + "/" + r.FormValue("NextToken")
			mu.Lock()
			calls[key]++
			mu.Unlock()
		}
		return false
	})})
	rows, err := p.firewalls(context.Background(), fixtureKey(), "cn-hangzhou")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].NetworkRef)
	mu.Lock()
	require.Equal(t, map[string]int{"intranet/": 1, "intranet/intranet-p2": 1, "internet/": 1, "internet/internet-p2": 1}, calls)
	mu.Unlock()
	var in, out []resource.Rule
	var details query.FirewallNativeDetails
	require.NoError(t, json.Unmarshal(rows[0].InboundRules, &in))
	require.NoError(t, json.Unmarshal(rows[0].OutboundRules, &out))
	require.NoError(t, json.Unmarshal(rows[0].NativeDetails, &details))
	require.Len(t, in, 2)
	require.Len(t, out, 3)
	require.Equal(t, "Accept", details.InnerAccessPolicy)
	require.Equal(t, "sg-private-source", in[0].Target)
	require.Nil(t, in[0].FromPort)
	require.Nil(t, in[0].ToPort)
	require.Nil(t, in[0].Priority)
	require.Nil(t, in[0].Enabled)
	require.Equal(t, "prl-private", details.AliyunInboundRuleFacts[0].PortRangeListID)
	require.Equal(t, "private services", details.AliyunInboundRuleFacts[0].PortRangeListName)
	require.Equal(t, "private-owner", details.AliyunInboundRuleFacts[0].SourceGroupOwnerAccount)
	require.Equal(t, "intranet", details.AliyunInboundRuleFacts[0].NICType)
	require.EqualValues(t, 0, *in[1].ICMPType)
	require.EqualValues(t, 0, *in[1].ICMPCode)
	require.Equal(t, "sgr-public-in", details.AliyunInboundRuleFacts[1].RuleID)
	require.Equal(t, "internet", details.AliyunInboundRuleFacts[1].NICType)
	require.Equal(t, 1, details.AliyunInboundRuleFacts[1].RuleIndex)
	for i := 0; i < 2; i++ {
		require.EqualValues(t, 53, *out[i].FromPort)
		require.Equal(t, "deny", out[i].Action)
		require.Equal(t, i, details.AliyunOutboundRuleFacts[i].RuleIndex)
		require.Equal(t, "sgr-private-out", details.AliyunOutboundRuleFacts[i].RuleID)
		require.Equal(t, "private-target-owner", details.AliyunOutboundRuleFacts[i].DestinationGroupOwnerAccount)
		require.Equal(t, "intranet", details.AliyunOutboundRuleFacts[i].NICType)
	}
	require.Equal(t, "pl-public", out[2].Target)
	require.Nil(t, out[2].FromPort)
	require.Nil(t, out[2].ToPort)
	require.Equal(t, 2, details.AliyunOutboundRuleFacts[2].RuleIndex)
	require.Equal(t, "sgr-public-out", details.AliyunOutboundRuleFacts[2].RuleID)
	require.Equal(t, "prl-public", details.AliyunOutboundRuleFacts[2].PortRangeListID)
	require.Equal(t, "public services", details.AliyunOutboundRuleFacts[2].PortRangeListName)
	require.Equal(t, "public destinations", details.AliyunOutboundRuleFacts[2].DestinationPrefixListName)
	require.Equal(t, "internet", details.AliyunOutboundRuleFacts[2].NICType)
}

func TestClassicFirewallLaterRuleFailureInEitherNICScopeNeverPublishes(t *testing.T) {
	for _, nic := range []string{"intranet", "internet"} {
		t.Run(nic, func(t *testing.T) {
			var failures atomic.Int32
			p := New(Options{Transport: classicFirewallFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != "DescribeSecurityGroupAttribute" || r.FormValue("NicType") != nic || r.FormValue("NextToken") != nic+"-p2" {
					return false
				}
				failures.Add(1)
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"Code":"Forbidden.RAM","Message":"synthetic-secret"}`))
				return true
			})})
			snapshot, err := p.Fetch(context.Background(), fixtureUnit("security_group"), fixtureKey())
			require.Error(t, err)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
			require.EqualValues(t, 1, failures.Load())
			require.NotContains(t, err.Error(), "synthetic-secret")
		})
	}
}
