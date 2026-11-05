package resource

// Rule is one displayed firewall entry, independent of native policy semantics.
type Rule struct {
	Direction             string   `json:"direction"`
	Protocol              string   `json:"protocol"`
	FromPort              *int32   `json:"from_port,omitempty"`
	ToPort                *int32   `json:"to_port,omitempty"`
	ICMPType              *int32   `json:"icmp_type,omitempty"`
	ICMPCode              *int32   `json:"icmp_code,omitempty"`
	TargetType            string   `json:"target_type"`
	Target                string   `json:"target"`
	Description           string   `json:"description"`
	Action                string   `json:"action"`
	Priority              *int32   `json:"priority" extensions:"x-nullable"`
	Enabled               *bool    `json:"enabled" extensions:"x-nullable"`
	SourceRanges          []string `json:"source_ranges"`
	DestinationRanges     []string `json:"destination_ranges"`
	SourceNetworkTags     []string `json:"source_network_tags"`
	TargetNetworkTags     []string `json:"target_network_tags"`
	SourceServiceAccounts []string `json:"source_service_accounts"`
	TargetServiceAccounts []string `json:"target_service_accounts"`
}
