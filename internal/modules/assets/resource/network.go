package resource

import "encoding/json"

// NetworkInterface preserves canonical network references and all native addresses.
type NetworkInterface struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	NetworkRef  string   `json:"network_ref"`
	SubnetRef   string   `json:"subnet_ref"`
	PrivateIPv4 []string `json:"private_ipv4"`
	PublicIPv4  []string `json:"public_ipv4"`
	PrivateIPv6 []string `json:"private_ipv6"`
	PublicIPv6  []string `json:"public_ipv6"`
}

// MarshalJSON emits absent address collections as arrays without changing the caller.
func (n NetworkInterface) MarshalJSON() ([]byte, error) {
	type interfaceJSON NetworkInterface
	value := interfaceJSON(n)
	if value.PrivateIPv4 == nil {
		value.PrivateIPv4 = []string{}
	}
	if value.PublicIPv4 == nil {
		value.PublicIPv4 = []string{}
	}
	if value.PrivateIPv6 == nil {
		value.PrivateIPv6 = []string{}
	}
	if value.PublicIPv6 == nil {
		value.PublicIPv6 = []string{}
	}
	return json.Marshal(value)
}

// GCPAliasRange describes a routed alias range, not a list of runtime addresses.
type GCPAliasRange struct {
	CIDR                string `json:"cidr"`
	SubnetworkRangeName string `json:"subnetwork_range_name,omitempty"`
}

// GCPExternalIPv6 retains each access configuration's address and prefix together.
type GCPExternalIPv6 struct {
	Name         string `json:"name,omitempty"`
	Address      string `json:"address"`
	PrefixLength *int64 `json:"prefix_length,omitempty"`
}

// GCPNetworkInterfaceDetails associates allowlisted native facts with the
// zero-based index of the corresponding common NetworkInterface.
type GCPNetworkInterfaceDetails struct {
	InterfaceIndex           int               `json:"interface_index"`
	Name                     string            `json:"name,omitempty"`
	StackType                string            `json:"stack_type,omitempty"`
	IPv6AccessType           string            `json:"ipv6_access_type,omitempty"`
	InternalIPv6PrefixLength *int64            `json:"internal_ipv6_prefix_length,omitempty"`
	AliasIPv4Ranges          []GCPAliasRange   `json:"alias_ipv4_ranges,omitempty"`
	AliasIPv6Ranges          []GCPAliasRange   `json:"alias_ipv6_ranges,omitempty"`
	ExternalIPv6             []GCPExternalIPv6 `json:"external_ipv6,omitempty"`
}
