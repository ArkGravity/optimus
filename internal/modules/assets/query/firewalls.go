package query

import (
	"context"
	"encoding/json"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type FirewallNativeDetails struct {
	AliyunInboundRuleFacts  []AliyunRuleFacts `json:"aliyun_inbound_rule_facts,omitempty"`
	AliyunOutboundRuleFacts []AliyunRuleFacts `json:"aliyun_outbound_rule_facts,omitempty"`
	SecurityGroupType       string            `json:"security_group_type,omitempty"`
	InnerAccessPolicy       string            `json:"inner_access_policy,omitempty"`
	Priority                *int32            `json:"priority,omitempty"`
	Disabled                *bool             `json:"disabled,omitempty"`
	Direction               string            `json:"direction,omitempty"`
	AWSInboundRuleFacts     []AWSRuleFacts    `json:"aws_inbound_rule_facts,omitempty"`
	AWSOutboundRuleFacts    []AWSRuleFacts    `json:"aws_outbound_rule_facts,omitempty"`
}

// AliyunRuleFacts preserves provider-only targeting facts for each displayed entry.
type AliyunRuleFacts struct {
	SourceGroupID                string `json:"source_group_id,omitempty"`
	DestinationGroupID           string `json:"destination_group_id,omitempty"`
	SourcePrefixListID           string `json:"source_prefix_list_id,omitempty"`
	DestinationPrefixListID      string `json:"destination_prefix_list_id,omitempty"`
	RuleIndex                    int    `json:"rule_index"`
	RuleID                       string `json:"rule_id,omitempty"`
	NICType                      string `json:"nic_type,omitempty"`
	SourceGroupName              string `json:"source_group_name,omitempty"`
	SourceGroupOwnerAccount      string `json:"source_group_owner_account,omitempty"`
	DestinationGroupName         string `json:"destination_group_name,omitempty"`
	DestinationGroupOwnerAccount string `json:"destination_group_owner_account,omitempty"`
	SourcePrefixListName         string `json:"source_prefix_list_name,omitempty"`
	DestinationPrefixListName    string `json:"destination_prefix_list_name,omitempty"`
	SourcePortRange              string `json:"source_port_range,omitempty"`
	PortRangeListID              string `json:"port_range_list_id,omitempty"`
	PortRangeListName            string `json:"port_range_list_name,omitempty"`
}

type AWSRuleFacts struct {
	// RuleIndex is the zero-based index in the corresponding common rule array.
	RuleIndex int    `json:"rule_index"`
	OwnerID   string `json:"owner_id,omitempty"`
	VPCID     string `json:"vpc_id,omitempty"`
	PeeringID string `json:"peering_id,omitempty"`
}
type Firewall struct {
	Base
	NetworkRef        string                `json:"network_ref"`
	Description       string                `json:"description"`
	GroupID           string                `json:"group_id,omitempty"`
	GroupName         string                `json:"group_name,omitempty"`
	VPCID             string                `json:"vpc_id,omitempty"`
	OwnerID           string                `json:"owner_id,omitempty"`
	InboundRuleCount  int                   `json:"inbound_rule_count"`
	OutboundRuleCount int                   `json:"outbound_rule_count"`
	InboundRules      []resource.Rule       `json:"inbound_rules"`
	OutboundRules     []resource.Rule       `json:"outbound_rules"`
	NativeDetails     FirewallNativeDetails `json:"native_details"`
}

var firewallSources = map[string]string{
	"aws":    projection("aws", "aws_security_groups", "r.group_id", "r.name", "''", "''", "region", `jsonb_build_object('group_id',r.group_id,'group_name',r.group_name,'vpc_id',r.vpc_id,'owner_id',r.owner_id,'network_ref',r.vpc_id,'description',r.description,'inbound_rules',r.inbound_rules,'outbound_rules',r.outbound_rules)`, `'{}'::jsonb`),
	"aliyun": projection("aliyun", "aliyun_security_groups", "r.firewall_id", "r.name", "''", "''", "region", `jsonb_build_object('network_ref',r.network_ref,'description',r.description,'inbound_rules',r.inbound_rules,'outbound_rules',r.outbound_rules)`, "r.native_details"),
	"gcp":    projection("gcp", "gcp_firewalls", "r.firewall_id", "r.name", "''", "''", "global", `jsonb_build_object('network_ref',r.network_ref,'description',r.description,'inbound_rules',r.inbound_rules,'outbound_rules',r.outbound_rules)`, "r.native_details"),
}

func firewallFrom(row projectionRow) (Firewall, error) {
	item := Firewall{}
	if err := json.Unmarshal(row.Facts, &item); err != nil {
		return item, err
	}
	if err := json.Unmarshal(row.NativeDetails, &item.NativeDetails); err != nil {
		return item, err
	}
	item.Base = row.base("firewall")
	if item.InboundRules == nil {
		item.InboundRules = []resource.Rule{}
	}
	if item.OutboundRules == nil {
		item.OutboundRules = []resource.Rule{}
	}
	if row.Provider == "aws" {
		// The AWS table predates common rules; preserve ICMP type/code explicitly.
		var legacy struct {
			Inbound  []models.SecurityGroupRule `json:"inbound_rules"`
			Outbound []models.SecurityGroupRule `json:"outbound_rules"`
		}
		if err := json.Unmarshal(row.Facts, &legacy); err != nil {
			return item, err
		}
		item.InboundRules = awsRules(legacy.Inbound, "inbound")
		item.OutboundRules = awsRules(legacy.Outbound, "outbound")
		item.NativeDetails.AWSInboundRuleFacts = awsRuleFacts(legacy.Inbound)
		item.NativeDetails.AWSOutboundRuleFacts = awsRuleFacts(legacy.Outbound)
	}
	item.InboundRuleCount = len(item.InboundRules)
	item.OutboundRuleCount = len(item.OutboundRules)
	return item, nil
}
func awsRules(rows []models.SecurityGroupRule, direction string) []resource.Rule {
	rules := make([]resource.Rule, len(rows))
	for i, row := range rows {
		rule := resource.Rule{Direction: direction, Protocol: row.Protocol, FromPort: row.FromPort, ToPort: row.ToPort, TargetType: row.TargetType, Target: row.Target, Description: row.Description, Action: "allow"}
		if row.Protocol == "icmp" || row.Protocol == "icmpv6" || row.Protocol == "1" || row.Protocol == "58" {
			rule.ICMPType = row.FromPort
			rule.ICMPCode = row.ToPort
			rule.FromPort = nil
			rule.ToPort = nil
		}
		rules[i] = rule
	}
	return rules
}
func awsRuleFacts(rows []models.SecurityGroupRule) []AWSRuleFacts {
	var facts []AWSRuleFacts
	for i, row := range rows {
		if row.OwnerID == "" && row.VPCID == "" && row.PeeringID == "" {
			continue
		}
		facts = append(facts, AWSRuleFacts{RuleIndex: i, OwnerID: row.OwnerID, VPCID: row.VPCID, PeeringID: row.PeeringID})
	}
	return facts
}
func (r *Repo) ListFirewalls(ctx context.Context, f Filter) ([]Firewall, int64, error) {
	extra := ""
	var args []any
	if f.NetworkRef != "" {
		extra = "facts->>'network_ref' = ?"
		args = []any{f.NetworkRef}
	}
	rows, total, err := r.page(ctx, firewallSources, f, extra, args)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Firewall, len(rows))
	for i, row := range rows {
		items[i], err = firewallFrom(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (r *Repo) FindFirewall(ctx context.Context, key string, includeDeleted bool) (*Firewall, error) {
	row, err := r.find(ctx, firewallSources, key, "firewall", includeDeleted)
	if err != nil {
		return nil, err
	}
	item, err := firewallFrom(*row)
	return &item, err
}
