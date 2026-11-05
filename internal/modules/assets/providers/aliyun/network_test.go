package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const firstVPCs = `{"PageNumber":1,"PageSize":1,"TotalCount":2,"Vpcs":{"Vpc":[{"VpcId":"vpc-a","VpcName":"network-a","RegionId":"cn-hangzhou","CidrBlock":"10.0.0.0/16","Ipv6CidrBlock":"2001:db8::/56","Ipv6CidrBlocks":{"Ipv6CidrBlock":[{"Ipv6CidrBlock":"2001:db8:100::/56"}]},"SecondaryCidrBlocks":{"SecondaryCidrBlock":["10.2.0.0/16"]},"UserCidrs":{"UserCidr":["192.0.2.0/24"]},"Status":"Available","Tags":{"Tag":[{"Key":"team","Value":"synthetic"}]},"NatGatewayIds":{"NatGatewayIds":["discard-me"]}}]}}`
const secondVPCs = `{"PageNumber":2,"PageSize":1,"TotalCount":2,"Vpcs":{"Vpc":[{"VpcId":"vpc-b","RegionId":"cn-hangzhou","CidrBlock":"10.3.0.0/16"}]}}`
const switches = `{"PageNumber":1,"PageSize":50,"TotalCount":1,"VSwitches":{"VSwitch":[{"VSwitchId":"vsw-a","VSwitchName":"subnet-a","VpcId":"vpc-a","ZoneId":"cn-hangzhou-b","CidrBlock":"10.0.1.0/24","Ipv6CidrBlock":"2001:db8:0:1::/64","AvailableIpAddressCount":42,"Tags":{"Tag":[{"Key":"kind","Value":"fixture"}]}}]}}`

func networkFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	return fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if override != nil && override(w, r) {
			return
		}
		switch r.Header.Get("x-acs-action") {
		case "GetCallerIdentity":
			_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
		case "DescribeVpcs":
			if r.FormValue("PageNumber") == "2" {
				_, _ = w.Write([]byte(secondVPCs))
			} else {
				_, _ = w.Write([]byte(firstVPCs))
			}
		case "DescribeVSwitches":
			_, _ = w.Write([]byte(switches))
		default:
			t.Errorf("unexpected action %s", r.Header.Get("x-acs-action"))
			http.Error(w, "unexpected", 500)
		}
	})
}

func TestNetworkPagesCIDRsTagsAndCanonicalReferences(t *testing.T) {
	p := New(Options{Transport: networkFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if r.FormValue("PageSize") != "50" {
			t.Errorf("VPC API page size exceeds documented maximum 50: %s", r.FormValue("PageSize"))
		}
		return false
	})})
	vpcs, subnets, err := p.network(context.Background(), fixtureKey(), "cn-hangzhou")
	if err != nil || len(vpcs) != 2 || len(subnets) != 1 {
		t.Fatalf("vpcs=%+v subnets=%+v err=%v", vpcs, subnets, err)
	}
	if vpcs[0].NetworkID != "vpc-a" || vpcs[0].NativeRef != "vpc-a" || subnets[0].NetworkRef != vpcs[0].NativeRef || subnets[0].Zone != "cn-hangzhou-b" {
		t.Fatal("invalid references")
	}
	if string(vpcs[0].CIDRBlocks) != `["10.0.0.0/16","2001:db8::/56","2001:db8:100::/56","10.2.0.0/16"]` {
		t.Fatalf("CIDRs=%s", vpcs[0].CIDRBlocks)
	}
	if string(vpcs[0].Tags) != `{"team":"synthetic"}` || string(subnets[0].Tags) != `{"kind":"fixture"}` || string(subnets[0].NativeDetails) != `{"available_ip_count":42}` {
		t.Fatalf("native facts %+v %+v", vpcs[0], subnets[0])
	}
	if strings.Contains(string(vpcs[0].NativeDetails), "discard-me") {
		t.Fatal("stored raw metadata")
	}
}

func TestNetworkPartialResponsesNeverReturnRows(t *testing.T) {
	for _, mode := range []string{"vpc-page-two-403", "vpc-repeat", "vpc-truncated", "vswitch-403", "vswitch-malformed", "bad-cidr", "unknown-parent"} {
		t.Run(mode, func(t *testing.T) {
			transport := networkFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				action := r.Header.Get("x-acs-action")
				if action == "DescribeVpcs" && r.FormValue("PageNumber") == "2" {
					switch mode {
					case "vpc-page-two-403":
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
						return true
					case "vpc-repeat":
						_, _ = w.Write([]byte(firstVPCs))
						return true
					case "vpc-truncated":
						_, _ = w.Write([]byte(`{"PageNumber":2,"PageSize":1,"TotalCount":2,"Vpcs":{"Vpc":[]}}`))
						return true
					}
				}
				if action == "DescribeVSwitches" {
					switch mode {
					case "vswitch-403":
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
						return true
					case "vswitch-malformed":
						_, _ = w.Write([]byte(`{`))
						return true
					case "bad-cidr":
						_, _ = w.Write([]byte(strings.Replace(switches, "10.0.1.0/24", "not-a-cidr", 1)))
						return true
					case "unknown-parent":
						_, _ = w.Write([]byte(strings.Replace(switches, "vpc-a", "vpc-outside", 1)))
						return true
					}
				}
				return false
			})
			vpcs, subnets, err := New(Options{Transport: transport}).network(context.Background(), fixtureKey(), "cn-hangzhou")
			if err == nil || len(vpcs) != 0 || len(subnets) != 0 || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("partial snapshot vpcs=%v subnets=%v err=%v", vpcs, subnets, err)
			}
		})
	}
}

func TestVSwitchPaginationRejectsRepeatedNativeID(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprint(duplicate), func(t *testing.T) {
			transport := networkFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != "DescribeVSwitches" {
					return false
				}
				var body map[string]any
				_ = json.Unmarshal([]byte(switches), &body)
				body["PageSize"] = 1
				body["TotalCount"] = 2
				if r.FormValue("PageNumber") == "2" {
					body["PageNumber"] = 2
					if !duplicate {
						body["VSwitches"].(map[string]any)["VSwitch"].([]any)[0].(map[string]any)["VSwitchId"] = "vsw-b"
					}
				}
				_ = json.NewEncoder(w).Encode(body)
				return true
			})
			vpcs, subnets, err := New(Options{Transport: transport}).network(context.Background(), fixtureKey(), "cn-hangzhou")
			if duplicate {
				if err == nil || len(vpcs) != 0 || len(subnets) != 0 {
					t.Fatalf("duplicate ID accepted %v %v %v", vpcs, subnets, err)
				}
				return
			}
			if err != nil || len(subnets) != 2 {
				t.Fatalf("second vSwitch page missing %v %v", subnets, err)
			}
		})
	}
}
