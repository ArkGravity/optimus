package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

const firstInstances = `{"PageNumber":1,"PageSize":2,"TotalCount":3,"Instances":{"Instance":[{"InstanceId":"i-a","InstanceName":"Stopped","Status":"Stopped","RegionId":"cn-hangzhou","ZoneId":"cn-hangzhou-b","Cpu":4,"Memory":8192,"ImageId":"img-a","OSType":"linux","Tags":{"Tag":[{"TagKey":"team","TagValue":"synthetic"}]},"VpcAttributes":{"VpcId":"vpc-a","VSwitchId":"vsw-a","PrivateIpAddress":{"IpAddress":["10.0.0.2"]}},"PublicIpAddress":{"IpAddress":["198.51.100.10"]},"NetworkInterfaces":{"NetworkInterface":[{"NetworkInterfaceId":"eni-a","Type":"Primary"},{"NetworkInterfaceId":"eni-b","Type":"Secondary"}]},"MetadataOptions":{"Secret":"discard-me"},"UserData":"discard-me"},{"InstanceId":"i-b","Status":"Suspended","RegionId":"cn-hangzhou"}]}}`
const secondInstances = `{"PageNumber":2,"PageSize":2,"TotalCount":3,"Instances":{"Instance":[{"InstanceId":"i-c","Status":"Running","RegionId":"cn-hangzhou"}]}}`
const firstNICs = `{"PageNumber":1,"PageSize":2,"TotalCount":2,"NetworkInterfaceSets":{"NetworkInterfaceSet":[{"NetworkInterfaceId":"eni-a","InstanceId":"i-a","Type":"Primary","VpcId":"vpc-a","VSwitchId":"vsw-a","PrivateIpAddress":"10.0.0.2","AssociatedPublicIp":{"PublicIpAddress":"198.51.100.10"},"PrivateIpSets":{"PrivateIpSet":[{"PrivateIpAddress":"::ffff:10.0.0.3","AssociatedPublicIp":{"PublicIpAddress":"198.51.100.11"}}]},"Ipv6Sets":{"Ipv6Set":[{"Ipv6Address":"2001:db8:0:0::2"}]}},{"NetworkInterfaceId":"eni-b","InstanceId":"i-a","Type":"Secondary","VpcId":"vpc-b","VSwitchId":"vsw-b","PrivateIpAddress":"10.2.0.2","PrivateIpSets":{"PrivateIpSet":[{"PrivateIpAddress":"10.2.0.3"}]},"Ipv6Sets":{"Ipv6Set":[{"Ipv6Address":"fd00::2"}]}}]}}`

func instanceFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.RoundTripper {
	return fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		if override != nil && override(w, r) {
			return
		}
		switch r.Header.Get("x-acs-action") {
		case "GetCallerIdentity":
			_, _ = w.Write([]byte(`{"AccountId":"1234567890123456"}`))
		case "DescribeInstances":
			if r.FormValue("PageNumber") == "2" {
				_, _ = w.Write([]byte(secondInstances))
			} else {
				_, _ = w.Write([]byte(firstInstances))
			}
		case "DescribeNetworkInterfaces":
			if r.FormValue("InstanceId") == "" {
				t.Error("NIC query must be bounded to an instance")
			}
			if r.FormValue("InstanceId") == "i-a" {
				_, _ = w.Write([]byte(firstNICs))
			} else {
				_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":2,"TotalCount":0,"NetworkInterfaceSets":{"NetworkInterfaceSet":[]}}`))
			}
		default:
			t.Errorf("unexpected SDK action %s", r.Header.Get("x-acs-action"))
			http.Error(w, "unexpected", 500)
		}
	})
}

func TestInstancesAllStatusesPagesNativeFactsAndExactNICAssociations(t *testing.T) {
	p := New(Options{Transport: instanceFixture(t, nil)})
	rows, err := p.instances(context.Background(), fixtureKey(), "cn-hangzhou")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows[0].NativeStatus != "Stopped" || rows[1].NativeStatus != "Suspended" {
		t.Fatalf("lost native statuses %+v", rows)
	}
	if string(rows[0].Tags) != `{"team":"synthetic"}` || strings.Contains(string(rows[0].NativeDetails), "discard-me") {
		t.Fatalf("bad native payload %+v", rows[0])
	}
	var facts map[string]any
	_ = json.Unmarshal(rows[0].NativeDetails, &facts)
	if facts["cpu_count"] != float64(4) || facts["memory_mb"] != float64(8192) || facts["image_id"] != "img-a" || facts["os"] != "linux" {
		t.Fatalf("facts %s", rows[0].NativeDetails)
	}
	var unknown map[string]any
	_ = json.Unmarshal(rows[1].NativeDetails, &unknown)
	if _, ok := unknown["cpu_count"]; ok {
		t.Fatal("guessed CPU")
	}
	var nics []resource.NetworkInterface
	if err := json.Unmarshal(rows[0].NetworkInterfaces, &nics); err != nil {
		t.Fatal(err)
	}
	if len(nics) != 2 || nics[0].NetworkRef != "vpc-a" || nics[0].SubnetRef != "vsw-a" || fmt.Sprint(nics[0].PrivateIPv4) != "[10.0.0.2 10.0.0.3]" || fmt.Sprint(nics[0].PublicIPv4) != "[198.51.100.10 198.51.100.11]" || fmt.Sprint(nics[0].PrivateIPv6) != "[2001:db8::2]" || nics[1].NetworkRef != "vpc-b" || nics[1].SubnetRef != "vsw-b" || len(nics[1].PublicIPv4) != 0 {
		t.Fatalf("incorrect NIC mapping: %+v", nics)
	}
	if string(rows[1].NetworkInterfaces) != "[]" {
		t.Fatalf("no-public VM lost empty shape %s", rows[1].NetworkInterfaces)
	}
}

func TestInstancesRejectPartialPaginationAndNICFailures(t *testing.T) {
	for _, mode := range []string{"page-two-forbidden", "page-two-throttled", "repeated-page", "duplicate-id", "unexpected-token", "changed-total", "truncated-total", "malformed", "nic-forbidden", "missing-nic", "bad-ip"} {
		t.Run(mode, func(t *testing.T) {
			transport := instanceFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				action := r.Header.Get("x-acs-action")
				if action == "DescribeInstances" && r.FormValue("PageNumber") == "2" {
					switch mode {
					case "page-two-forbidden":
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret raw SDK body"}`))
						return true
					case "page-two-throttled":
						w.WriteHeader(429)
						_, _ = w.Write([]byte(`{"Code":"Throttling","Message":"synthetic-secret"}`))
						return true
					case "repeated-page":
						_, _ = w.Write([]byte(firstInstances))
						return true
					case "duplicate-id":
						_, _ = w.Write([]byte(strings.Replace(secondInstances, `"i-c"`, `"i-a"`, 1)))
						return true
					case "unexpected-token":
						_, _ = w.Write([]byte(strings.Replace(secondInstances, `"PageNumber":2`, `"PageNumber":2,"NextToken":"repeated-cursor"`, 1)))
						return true
					case "changed-total":
						_, _ = w.Write([]byte(strings.Replace(secondInstances, `"TotalCount":3`, `"TotalCount":4`, 1)))
						return true
					case "truncated-total":
						_, _ = w.Write([]byte(strings.Replace(secondInstances, `"Instance":[{"InstanceId":"i-c","Status":"Running","RegionId":"cn-hangzhou"}]`, `"Instance":[]`, 1)))
						return true
					case "malformed":
						_, _ = w.Write([]byte(`{`))
						return true
					}
				}
				if action == "DescribeNetworkInterfaces" {
					switch mode {
					case "nic-forbidden":
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
						return true
					case "missing-nic":
						_, _ = w.Write([]byte(`{"PageNumber":1,"PageSize":2,"TotalCount":0,"NetworkInterfaceSets":{}}`))
						return true
					case "bad-ip":
						_, _ = w.Write([]byte(strings.Replace(firstNICs, "10.0.0.2", "invalid-ip", 1)))
						return true
					}
				}
				return false
			})
			p := New(Options{Transport: transport})
			rows, err := p.instances(context.Background(), fixtureKey(), "cn-hangzhou")
			if err == nil || len(rows) != 0 || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("partial leaked rows=%+v err=%v", rows, err)
			}
		})
	}
}

func TestNICEnrichmentCompletesAllPagesAndExcludesUnattachedInterfaces(t *testing.T) {
	for _, failSecond := range []bool{false, true} {
		t.Run(fmt.Sprint(failSecond), func(t *testing.T) {
			transport := instanceFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != "DescribeNetworkInterfaces" || r.FormValue("InstanceId") != "i-a" {
					return false
				}
				var body map[string]any
				_ = json.Unmarshal([]byte(firstNICs), &body)
				sets := body["NetworkInterfaceSets"].(map[string]any)
				all := sets["NetworkInterfaceSet"].([]any)
				body["PageSize"] = 1
				body["TotalCount"] = 3
				page := r.FormValue("PageNumber")
				switch page {
				case "1":
					body["PageNumber"] = 1
					sets["NetworkInterfaceSet"] = all[:1]
				case "2":
					if failSecond {
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"Code":"Forbidden","Message":"synthetic-secret"}`))
						return true
					}
					body["PageNumber"] = 2
					sets["NetworkInterfaceSet"] = all[1:]
				case "3":
					body["PageNumber"] = 3
					sets["NetworkInterfaceSet"] = []any{map[string]any{"NetworkInterfaceId": "eni-unattached", "Type": "Secondary", "VpcId": "vpc-outside", "PrivateIpAddress": "192.0.2.50"}}
				default:
					t.Errorf("unexpected NIC page %s", page)
				}
				_ = json.NewEncoder(w).Encode(body)
				return true
			})
			snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), fixtureUnit("instance"), fixtureKey())
			if failSecond {
				if err == nil || snapshot.Complete || snapshot.Publish != nil {
					t.Fatalf("published incomplete NICs %+v %v", snapshot, err)
				}
				return
			}
			if err != nil || !snapshot.Complete {
				t.Fatalf("all NIC pages %+v %v", snapshot, err)
			}
			rows, err := New(Options{Transport: transport}).instances(context.Background(), fixtureKey(), "cn-hangzhou")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(rows[0].NetworkInterfaces), "eni-unattached") {
				t.Fatal("imported unattached ENI")
			}
		})
	}
}

func TestInstanceLevelIPRetainsKnownSecondaryNICAssociation(t *testing.T) {
	transport := instanceFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Header.Get("x-acs-action") {
		case "DescribeInstances":
			if r.FormValue("PageNumber") == "1" {
				_, _ = w.Write([]byte(strings.Replace(firstInstances, `["198.51.100.10"]`, `["198.51.100.10","198.51.100.22"]`, 1)))
				return true
			}
		case "DescribeNetworkInterfaces":
			if r.FormValue("InstanceId") == "i-a" {
				_, _ = w.Write([]byte(strings.Replace(firstNICs, `"PrivateIpAddress":"10.2.0.2"`, `"PrivateIpAddress":"10.2.0.2","AssociatedPublicIp":{"PublicIpAddress":"198.51.100.22"}`, 1)))
				return true
			}
		}
		return false
	})
	rows, err := New(Options{Transport: transport}).instances(context.Background(), fixtureKey(), "cn-hangzhou")
	if err != nil {
		t.Fatal(err)
	}
	var nics []resource.NetworkInterface
	_ = json.Unmarshal(rows[0].NetworkInterfaces, &nics)
	if strings.Contains(fmt.Sprint(nics[0].PublicIPv4), "198.51.100.22") || fmt.Sprint(nics[1].PublicIPv4) != "[198.51.100.22]" {
		t.Fatalf("secondary public IP assigned to primary: %+v", nics)
	}
}
