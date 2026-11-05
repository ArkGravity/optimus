package aliyun

import (
	"context"
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/alibabacloud-go/tea/dara"
)

func (p *Provider) firewalls(ctx context.Context, key *credentials.CloudKey, region string) ([]models.AliyunSecurityGroup, error) {
	client, err := p.ecsClient(ctx, key, region)
	if err != nil {
		return nil, safeError(err)
	}
	rows := []models.AliyunSecurityGroup{}
	pagination := pages{}
	for page := int32(1); ; page++ {
		response, err := client.DescribeSecurityGroupsWithContext(ctx, &ecs.DescribeSecurityGroupsRequest{RegionId: dara.String(region), PageNumber: dara.Int32(page), PageSize: dara.Int32(100)}, runtimeOptions())
		if ctx.Err() != nil {
			return nil, safeError(ctx.Err())
		}
		if err != nil {
			return nil, safeError(err)
		}
		if response == nil || response.Body == nil || response.Body.SecurityGroups == nil {
			return nil, configError()
		}
		body := response.Body
		if dara.StringValue(body.NextToken) != "" || (body.RegionId != nil && *body.RegionId != region) {
			return nil, configError()
		}
		done, err := pagination.accept(page, body.PageNumber, body.PageSize, body.TotalCount, len(body.SecurityGroups.SecurityGroup))
		if err != nil {
			return nil, err
		}
		for _, item := range body.SecurityGroups.SecurityGroup {
			if item == nil {
				return nil, configError()
			}
			id := dara.StringValue(item.SecurityGroupId)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			tags := map[string]string{}
			if item.Tags != nil {
				for _, tag := range item.Tags.Tag {
					if tag == nil {
						return nil, configError()
					}
					tags[dara.StringValue(tag.TagKey)] = dara.StringValue(tag.TagValue)
				}
			}
			rows = append(rows, models.AliyunSecurityGroup{FirewallID: id, Name: dara.StringValue(item.SecurityGroupName), NetworkRef: dara.StringValue(item.VpcId), Region: region, Description: dara.StringValue(item.Description), Tags: jsonValue(tags), NativeDetails: jsonValue(query.FirewallNativeDetails{SecurityGroupType: dara.StringValue(item.SecurityGroupType)})})
		}
		if done {
			break
		}
	}
	err = collectDetails(ctx, len(rows), func(ctx context.Context, index int) error { return p.firewallRules(ctx, key, region, &rows[index]) })
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (p *Provider) firewallRules(ctx context.Context, key *credentials.CloudKey, region string, row *models.AliyunSecurityGroup) error {
	client, err := p.ecsClient(ctx, key, region)
	if err != nil {
		return safeError(err)
	}
	in, out := []resource.Rule{}, []resource.Rule{}
	details := query.FirewallNativeDetails{}
	if err := json.Unmarshal(row.NativeDetails, &details); err != nil {
		return configError()
	}
	seenIDs := map[string]bool{}
	seenRules := map[string]bool{}
	nicTypes := []string{"intranet"}
	if row.NetworkRef == "" {
		nicTypes = append(nicTypes, "internet")
	}
	for _, nic := range nicTypes {
		token := ""
		seenTokens := map[string]bool{}
		for page := 0; ; page++ {
			if page >= 10000 {
				return configError()
			}
			request := &ecs.DescribeSecurityGroupAttributeRequest{RegionId: dara.String(region), SecurityGroupId: dara.String(row.FirewallID), Direction: dara.String("all"), NicType: dara.String(nic), MaxResults: dara.Int32(1000)}
			if token != "" {
				request.NextToken = dara.String(token)
			}
			response, err := client.DescribeSecurityGroupAttributeWithContext(ctx, request, runtimeOptions())
			if ctx.Err() != nil {
				return safeError(ctx.Err())
			}
			if err != nil {
				return safeError(err)
			}
			if response == nil || response.Body == nil || response.Body.Permissions == nil {
				return configError()
			}
			body := response.Body
			if dara.StringValue(body.SecurityGroupId) != row.FirewallID || (body.RegionId != nil && *body.RegionId != region) || (body.VpcId != nil && *body.VpcId != row.NetworkRef) {
				return configError()
			}
			if body.InnerAccessPolicy != nil {
				if details.InnerAccessPolicy != "" && details.InnerAccessPolicy != *body.InnerAccessPolicy {
					return configError()
				}
				details.InnerAccessPolicy = *body.InnerAccessPolicy
			}
			if len(body.Permissions.Permission) > 1000 {
				return configError()
			}
			for _, permission := range body.Permissions.Permission {
				if permission == nil {
					return configError()
				}
				id := dara.StringValue(permission.SecurityGroupRuleId)
				if id != "" {
					if seenIDs[id] {
						return configError()
					}
					seenIDs[id] = true
				}
				fingerprint := string(jsonValue(permission))
				if seenRules[fingerprint] {
					return configError()
				}
				seenRules[fingerprint] = true
				rules, facts, err := mapPermission(permission)
				if err != nil {
					return err
				}
				for _, rule := range rules {
					indexed := facts
					if rule.Direction == "inbound" {
						indexed.RuleIndex = len(in)
						in = append(in, rule)
						details.AliyunInboundRuleFacts = append(details.AliyunInboundRuleFacts, indexed)
					} else {
						indexed.RuleIndex = len(out)
						out = append(out, rule)
						details.AliyunOutboundRuleFacts = append(details.AliyunOutboundRuleFacts, indexed)
					}
				}
			}
			next := dara.StringValue(body.NextToken)
			if next == "" {
				break
			}
			if seenTokens[next] || len(body.Permissions.Permission) == 0 {
				return configError()
			}
			seenTokens[next] = true
			token = next
		}
	}
	row.InboundRules = jsonValue(in)
	row.OutboundRules = jsonValue(out)
	row.NativeDetails = jsonValue(details)
	return nil
}

type ruleTarget struct{ kind, value string }

func mapPermission(item *ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission) ([]resource.Rule, query.AliyunRuleFacts, error) {
	facts := query.AliyunRuleFacts{SourceGroupID: dara.StringValue(item.SourceGroupId), DestinationGroupID: dara.StringValue(item.DestGroupId), SourcePrefixListID: dara.StringValue(item.SourcePrefixListId), DestinationPrefixListID: dara.StringValue(item.DestPrefixListId), RuleID: dara.StringValue(item.SecurityGroupRuleId), NICType: dara.StringValue(item.NicType), SourceGroupName: dara.StringValue(item.SourceGroupName), SourceGroupOwnerAccount: dara.StringValue(item.SourceGroupOwnerAccount), DestinationGroupName: dara.StringValue(item.DestGroupName), DestinationGroupOwnerAccount: dara.StringValue(item.DestGroupOwnerAccount), SourcePrefixListName: dara.StringValue(item.SourcePrefixListName), DestinationPrefixListName: dara.StringValue(item.DestPrefixListName), SourcePortRange: dara.StringValue(item.SourcePortRange), PortRangeListID: dara.StringValue(item.PortRangeListId), PortRangeListName: dara.StringValue(item.PortRangeListName)}
	rule := resource.Rule{Protocol: strings.ToLower(dara.StringValue(item.IpProtocol)), Description: dara.StringValue(item.Description), SourceRanges: []string{}, DestinationRanges: []string{}}
	switch dara.StringValue(item.Direction) {
	case "ingress":
		rule.Direction = "inbound"
	case "egress":
		rule.Direction = "outbound"
	default:
		return nil, facts, configError()
	}
	switch dara.StringValue(item.Policy) {
	case "Accept":
		rule.Action = "allow"
	case "Drop":
		rule.Action = "deny"
	default:
		return nil, facts, configError()
	}
	if rule.Protocol == "" {
		return nil, facts, configError()
	}
	if item.Priority != nil && *item.Priority != "" {
		priority, err := strconv.ParseInt(*item.Priority, 10, 32)
		if err != nil || priority < 1 || priority > 100 {
			return nil, facts, configError()
		}
		value := int32(priority)
		rule.Priority = &value
	}
	if item.PortRange != nil && *item.PortRange != "" {
		parts := strings.Split(*item.PortRange, "/")
		if len(parts) != 2 {
			return nil, facts, configError()
		}
		from, err1 := strconv.ParseInt(parts[0], 10, 32)
		to, err2 := strconv.ParseInt(parts[1], 10, 32)
		icmp := rule.Protocol == "icmp" || rule.Protocol == "icmpv6" || rule.Protocol == "1" || rule.Protocol == "58"
		limit := int64(65535)
		if icmp {
			limit = 255
		}
		if err1 != nil || err2 != nil || from < -1 || to < -1 || from > limit || to > limit || (!icmp && from > to) {
			return nil, facts, configError()
		}
		f, t := int32(from), int32(to)
		if icmp {
			rule.ICMPType = &f
			rule.ICMPCode = &t
		} else {
			rule.FromPort = &f
			rule.ToPort = &t
		}
	}
	targets := []ruleTarget{}
	for _, entry := range []struct {
		raw          *string
		source, ipv6 bool
	}{{item.SourceCidrIp, true, false}, {item.Ipv6SourceCidrIp, true, true}, {item.DestCidrIp, false, false}, {item.Ipv6DestCidrIp, false, true}} {
		raw := dara.StringValue(entry.raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.Addr().Is6() != entry.ipv6 {
			return nil, facts, configError()
		}
		value := prefix.Masked().String()
		if entry.source {
			rule.SourceRanges = append(rule.SourceRanges, value)
		} else {
			rule.DestinationRanges = append(rule.DestinationRanges, value)
		}
		if entry.source == (rule.Direction == "inbound") {
			kind := "ipv4"
			if entry.ipv6 {
				kind = "ipv6"
			}
			targets = append(targets, ruleTarget{kind, value})
		}
	}
	group, prefix := item.SourceGroupId, item.SourcePrefixListId
	if rule.Direction == "outbound" {
		group, prefix = item.DestGroupId, item.DestPrefixListId
	}
	if value := dara.StringValue(group); value != "" {
		targets = append(targets, ruleTarget{"security_group", value})
	}
	if value := dara.StringValue(prefix); value != "" {
		targets = append(targets, ruleTarget{"prefix_list", value})
	}
	if len(targets) == 0 {
		return nil, facts, configError()
	}
	rules := make([]resource.Rule, 0, len(targets))
	for _, target := range targets {
		entry := rule
		entry.TargetType = target.kind
		entry.Target = target.value
		rules = append(rules, entry)
	}
	return rules, facts, nil
}
