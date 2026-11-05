package resource

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkInterfaceJSONPreservesReferencesAndAllAddresses(t *testing.T) {
	original := NetworkInterface{ID: "nic-1", Name: "primary", NetworkRef: "projects/host-project/global/networks/shared", SubnetRef: "projects/host-project/regions/us-central1/subnetworks/shared", PrivateIPv4: []string{"10.0.0.1", "10.0.0.2"}, PublicIPv4: []string{"192.0.2.1"}, PrivateIPv6: []string{"fd00::1", "fd00::2"}, PublicIPv6: []string{"2001:db8::1"}}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"nic-1","name":"primary","network_ref":"projects/host-project/global/networks/shared","subnet_ref":"projects/host-project/regions/us-central1/subnetworks/shared","private_ipv4":["10.0.0.1","10.0.0.2"],"public_ipv4":["192.0.2.1"],"private_ipv6":["fd00::1","fd00::2"],"public_ipv6":["2001:db8::1"]}`, string(data))
	var decoded NetworkInterface
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original, decoded)
}

func TestNetworkInterfaceJSONEmitsAbsentAddressesAsEmptyArrays(t *testing.T) {
	for _, tc := range []struct {
		name     string
		original NetworkInterface
		expected string
	}{
		{name: "zero", expected: `{"id":"","name":"","network_ref":"","subnet_ref":"","private_ipv4":[],"public_ipv4":[],"private_ipv6":[],"public_ipv6":[]}`},
		{name: "partial", original: NetworkInterface{ID: "nic-1", Name: "primary", NetworkRef: "projects/host-project/global/networks/shared", SubnetRef: "subnet-1", PrivateIPv4: []string{"10.0.0.1", "10.0.0.2"}, PublicIPv6: []string{}}, expected: `{"id":"nic-1","name":"primary","network_ref":"projects/host-project/global/networks/shared","subnet_ref":"subnet-1","private_ipv4":["10.0.0.1","10.0.0.2"],"public_ipv4":[],"private_ipv6":[],"public_ipv6":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.original
			before.PrivateIPv4 = slices.Clone(tc.original.PrivateIPv4)
			before.PublicIPv4 = slices.Clone(tc.original.PublicIPv4)
			before.PrivateIPv6 = slices.Clone(tc.original.PrivateIPv6)
			before.PublicIPv6 = slices.Clone(tc.original.PublicIPv6)
			data, err := json.Marshal(tc.original)
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(data))
			require.Equal(t, before, tc.original, "serialization must preserve the caller's nil and populated slices")
		})
	}
}
