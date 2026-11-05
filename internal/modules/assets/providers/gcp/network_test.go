package gcp

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworksAndSubnetsUseScopedSDKPagesAndCanonicalReferences(t *testing.T) {
	key, private := fixtureKey(t)
	calls := map[string]int{}
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls[r.URL.Path]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/global/networks"):
			if calls[r.URL.Path] == 1 {
				return 200, `{"items":[{"id":"18446744073709551615","name":"shared","selfLink":"https://www.googleapis.com/compute/v1/projects/target-project/global/networks/shared","autoCreateSubnetworks":false,"mtu":1460,"routingConfig":{"routingMode":"REGIONAL"}}],"nextPageToken":"second"}`
			}
			return 200, `{"items":[]}`
		case strings.HasSuffix(r.URL.Path, "/regions/us-central1/subnetworks"):
			return 200, `{"items":[{"id":"12345678901234567890","name":"subnet","purpose":"PRIVATE","privateIpGoogleAccess":true,"region":"projects/target-project/regions/us-central1","network":"https://compute.googleapis.com/compute/v1/projects/host-project/global/networks/shared","ipCidrRange":"10.0.0.1/24","internalIpv6Prefix":"fd00:0:0:1::/64","secondaryIpRanges":[{"rangeName":"pods","ipCidrRange":"10.1.0.0/16"}]}]}`
		default:
			t.Fatalf("unexpected API %s", r.URL)
			return 500, `{}`
		}
	})
	p := New(Options{Transport: transport})
	client, err := p.computeClient(context.Background(), key)
	require.NoError(t, err)
	networks, err := p.networks(context.Background(), client, "target-project")
	require.NoError(t, err)
	require.Len(t, networks, 1)
	require.Equal(t, "18446744073709551615", networks[0].NetworkID)
	require.Equal(t, "projects/target-project/global/networks/shared", networks[0].NativeRef)
	require.Empty(t, networks[0].Region)
	require.JSONEq(t, `[]`, string(networks[0].CIDRBlocks))
	require.JSONEq(t, `{"mtu":1460,"routing_mode":"REGIONAL","auto_create_subnetworks":false}`, string(networks[0].NativeDetails))
	subnets, err := p.subnets(context.Background(), client, "target-project", "us-central1")
	require.NoError(t, err)
	require.Len(t, subnets, 1)
	require.Equal(t, "projects/host-project/global/networks/shared", subnets[0].NetworkRef)
	require.JSONEq(t, `["10.0.0.0/24","fd00:0:0:1::/64","10.1.0.0/16"]`, string(subnets[0].CIDRBlocks))
	require.JSONEq(t, `{"purpose":"PRIVATE","private_ip_google_access":true}`, string(subnets[0].NativeDetails))
	require.Equal(t, 2, len(calls))
}
func TestNetworkCollectionRejectsIncompletePagesAndMalformedNativeIdentity(t *testing.T) {
	for _, body := range []string{`{"items":[{"id":"0","name":"invalid"}]}`, `{"items":[null]}`, `{"items":[{"id":"1","name":"same"},{"id":"1","name":"same"}]}`, `{"nextPageToken":"repeated"}`, `{"warning":{"code":"PARTIAL_SUCCESS"}}`, `{"warning":{}}`, `{"items":[{"id":"1","name":"vm"}],"warning":{"code":"NO_RESULTS_ON_PAGE"}}`} {
		t.Run(body, func(t *testing.T) {
			key, private := fixtureKey(t)
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, body })
			p := New(Options{Transport: transport})
			client, err := p.computeClient(context.Background(), key)
			require.NoError(t, err)
			rows, err := p.networks(context.Background(), client, "target-project")
			require.Error(t, err)
			require.Nil(t, rows)
		})
	}
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
		calls++
		if calls == 1 {
			return 200, `{"items":[{"id":"1","name":"main"}],"nextPageToken":"page2"}`
		}
		return 403, `{"error":{"code":403,"message":"secret-body"}}`
	})
	p := New(Options{Transport: transport})
	client, err := p.computeClient(context.Background(), key)
	require.NoError(t, err)
	rows, err := p.networks(context.Background(), client, "target-project")
	require.Error(t, err)
	require.Nil(t, rows)
	require.NotContains(t, err.Error(), "secret-body")
}
func TestCanonicalReferenceRetainsSharedProjectWithoutFollowingURLs(t *testing.T) {
	for _, raw := range []string{"projects/host/global/networks/main", "https://www.googleapis.com/compute/v1/projects/host/global/networks/main", "https://compute.googleapis.com/compute/v1/projects/host/global/networks/main"} {
		value, err := canonicalRef(raw, "target-project", "networks")
		require.NoError(t, err)
		require.Equal(t, "projects/host/global/networks/main", value)
	}
	for _, raw := range []string{"https://metadata.invalid/compute/v1/projects/host/global/networks/main", "projects/host/global/networks/../main", "projects/host/global/networks/main?secret", "projects/host/global/networks/%2e"} {
		_, err := canonicalRef(raw, "target-project", "networks")
		require.Error(t, err)
	}
}

func TestDocumentedEmptyScopedListsAreComplete(t *testing.T) {
	key, private := fixtureKey(t)
	transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, `{"warning":{"code":"NO_RESULTS_ON_PAGE"}}` })
	p := New(Options{Transport: transport})
	client, err := p.computeClient(context.Background(), key)
	require.NoError(t, err)
	networks, err := p.networks(context.Background(), client, "target-project")
	require.NoError(t, err)
	require.Empty(t, networks)
	subnets, err := p.subnets(context.Background(), client, "target-project", "us-central1")
	require.NoError(t, err)
	require.Empty(t, subnets)
	instances, err := p.instances(context.Background(), client, "target-project", "us-central1", "us-central1-a")
	require.NoError(t, err)
	require.Empty(t, instances)
}
