package resource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleJSONPreservesZeroAndUnsupportedFacts(t *testing.T) {
	zero := int32(0)
	disabled := false
	rule := Rule{
		Direction: "inbound", Protocol: "icmp", FromPort: &zero, ToPort: &zero,
		ICMPType: &zero, ICMPCode: &zero, TargetType: "ipv6", Target: "::/0",
		Description: "ICMP zero values", Action: "deny", Priority: &zero, Enabled: &disabled,
	}
	data, err := json.Marshal(rule)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	for _, field := range []string{"from_port", "to_port", "icmp_type", "icmp_code", "priority"} {
		require.Equal(t, float64(0), fields[field], field)
	}
	require.Equal(t, false, fields["enabled"])
	var decoded Rule
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, rule, decoded)

	data, err = json.Marshal(Rule{Direction: "outbound", Protocol: "-1", Action: "allow"})
	require.NoError(t, err)
	fields = nil
	require.NoError(t, json.Unmarshal(data, &fields))
	for _, field := range []string{"from_port", "to_port", "icmp_type", "icmp_code", "priority", "enabled"} {
		require.Nil(t, fields[field], field)
	}
}

func TestRuleJSONPreservesTargetsAndICMPWildcards(t *testing.T) {
	wildcard := int32(-1)
	for _, target := range []struct {
		typeName string
		value    string
	}{
		{"ipv4", "192.0.2.0/24"},
		{"ipv6", "2001:db8::/32"},
		{"prefix_list", "pl-example"},
		{"security_group", "sg-example"},
	} {
		t.Run(target.typeName, func(t *testing.T) {
			rule := Rule{
				Direction: "inbound", Protocol: "icmp", ICMPType: &wildcard, ICMPCode: &wildcard,
				TargetType: target.typeName, Target: target.value, Description: "native target",
				Action: "allow", SourceRanges: []string{"192.0.2.0/24", "2001:db8::/32"},
				DestinationRanges: []string{"198.51.100.0/24"},
				SourceNetworkTags: []string{"source-tag"}, TargetNetworkTags: []string{"target-tag"},
				SourceServiceAccounts: []string{"source@example.invalid"},
				TargetServiceAccounts: []string{"target@example.invalid"},
			}
			data, err := json.Marshal(rule)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(data, &fields))
			require.Equal(t, target.typeName, fields["target_type"])
			require.Equal(t, target.value, fields["target"])
			require.Equal(t, []any{"source-tag"}, fields["source_network_tags"])
			require.Equal(t, []any{"target-tag"}, fields["target_network_tags"])
			require.Equal(t, []any{"source@example.invalid"}, fields["source_service_accounts"])
			require.Equal(t, []any{"target@example.invalid"}, fields["target_service_accounts"])
			var decoded Rule
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.Equal(t, rule, decoded)
		})
	}
}
