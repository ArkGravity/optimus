package gcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/stretchr/testify/require"
)

func TestInstancesPreserveNICAddressesAndSeparateNativeTags(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.True(t, strings.HasSuffix(r.URL.Path, "/zones/us-central1-a/instances"))
		if calls == 2 {
			return 200, `{}`
		}
		return 200, `{"items":[{"id":"18446744073709551615","name":"vm","zone":"https://www.googleapis.com/compute/v1/projects/target-project/zones/us-central1-a","machineType":"zones/us-central1-a/machineTypes/custom-7-12345","status":"TERMINATED","labels":{"label":"value"},"tags":{"items":["web"]},"params":{"resourceManagerTags":{"tagKeys/123":"tagValues/456"}},"metadata":{"items":[{"key":"startup-script","value":"secret-startup"}]},"serviceAccounts":[{"email":"private-oauth@example.test","scopes":["secret-scope"]}],"networkInterfaces":[{"name":"nic0","network":"https://www.googleapis.com/compute/v1/projects/host/global/networks/shared","subnetwork":"projects/host/regions/us-central1/subnetworks/shared-subnet","networkIP":"::ffff:10.0.0.5","ipv6Address":"fd00:0000::5","internalIpv6PrefixLength":96,"stackType":"IPV4_IPV6","ipv6AccessType":"EXTERNAL","aliasIpRanges":[{"ipCidrRange":"10.1.0.7/24","subnetworkRangeName":"pods"}],"aliasIpv6Ranges":[{"ipCidrRange":"fd01::7/96","subnetworkRangeName":"pods6"}],"accessConfigs":[{"name":"nat","natIP":"203.0.113.7"}],"ipv6AccessConfigs":[{"name":"external6","externalIpv6":"2001:0db8::5","externalIpv6PrefixLength":96}]},{"name":"nic1","network":"global/networks/other","networkIP":"10.2.0.5","ipv6Address":"fd02::5","stackType":"IPV4_IPV6","ipv6AccessType":"INTERNAL"}]}],"nextPageToken":"p2"}`
	})
	p := New(Options{Transport: transport})
	client, err := p.computeClient(context.Background(), key)
	require.NoError(t, err)
	rows, err := p.instances(context.Background(), client, "target-project", "us-central1", "us-central1-a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, "18446744073709551615", row.InstanceID)
	require.Equal(t, "custom-7-12345", row.InstanceType)
	require.Equal(t, "TERMINATED", row.NativeStatus)
	require.JSONEq(t, `{"label":"value"}`, string(row.Tags))
	require.Equal(t, 2, calls)
	require.Equal(t, 1, *tokens)
	var nics []resource.NetworkInterface
	require.NoError(t, json.Unmarshal(row.NetworkInterfaces, &nics))
	require.Len(t, nics, 2)
	require.Equal(t, "projects/host/global/networks/shared", nics[0].NetworkRef)
	require.Equal(t, []string{"10.0.0.5"}, nics[0].PrivateIPv4)
	require.Equal(t, []string{"fd00::5"}, nics[0].PrivateIPv6)
	require.Equal(t, []string{"203.0.113.7"}, nics[0].PublicIPv4)
	require.Equal(t, []string{"2001:db8::5"}, nics[0].PublicIPv6)
	require.Equal(t, "projects/target-project/global/networks/other", nics[1].NetworkRef)
	require.Empty(t, nics[1].PublicIPv6)
	var details query.InstanceNativeDetails
	require.NoError(t, json.Unmarshal(row.NativeDetails, &details))
	require.Nil(t, details.CPUCount)
	require.Nil(t, details.MemoryMB)
	require.Equal(t, []string{"web"}, details.NetworkTags)
	require.Equal(t, map[string]string{"tagKeys/123": "tagValues/456"}, details.ResourceManagerTags)
	require.Len(t, details.GCPNetworkInterfaces, 2)
	require.Equal(t, 0, details.GCPNetworkInterfaces[0].InterfaceIndex)
	require.Equal(t, "10.1.0.0/24", details.GCPNetworkInterfaces[0].AliasIPv4Ranges[0].CIDR)
	require.Equal(t, "2001:db8::5", details.GCPNetworkInterfaces[0].ExternalIPv6[0].Address)
	require.EqualValues(t, 96, *details.GCPNetworkInterfaces[0].ExternalIPv6[0].PrefixLength)
	require.Nil(t, details.GCPNetworkInterfaces[1].InternalIPv6PrefixLength)
	require.NotContains(t, string(row.NativeDetails), "secret")
	require.NotContains(t, string(row.NativeDetails), "private-oauth")
}
func TestInstancesRejectPartialOrInvalidSnapshots(t *testing.T) {
	for _, body := range []string{`{"items":[{"id":"0","name":"vm"}]}`, `{"items":[{"id":"1","name":"vm","zone":"projects/foreign/zones/us-central1-a"}]}`, `{"items":[{"id":"1","name":"vm","zone":"zones/us-central1-a","networkInterfaces":[{"network":"global/networks/main","networkIP":"bad"}]}]}`, `{"items":[{"id":"1","name":"vm","zone":"zones/us-central1-a","networkInterfaces":[null]}]}`, `{"warning":{"code":"UNREACHABLE"}}`, `{"nextPageToken":"again"}`} {
		t.Run(body, func(t *testing.T) {
			key, private := fixtureKey(t)
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, body })
			p := New(Options{Transport: transport})
			client, err := p.computeClient(context.Background(), key)
			require.NoError(t, err)
			rows, err := p.instances(context.Background(), client, "target-project", "us-central1", "us-central1-a")
			require.Error(t, err)
			require.Nil(t, rows)
		})
	}
}
