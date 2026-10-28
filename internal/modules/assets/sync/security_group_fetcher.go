package sync

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"gorm.io/datatypes"

	"github.com/ArkGravity/optimus/internal/models"
)

type SecurityGroupFetcher struct{}

// FetchSecurityGroups publishes no partial snapshot when a page fails.
func (SecurityGroupFetcher) FetchSecurityGroups(ctx context.Context, clients *Clients) ([]models.AWSSecurityGroup, error) {
	if clients == nil || clients.EC2 == nil {
		return nil, errEC2ClientRequired
	}
	paginator := ec2.NewDescribeSecurityGroupsPaginator(clients.EC2, &ec2.DescribeSecurityGroupsInput{MaxResults: aws.Int32(100)})
	seen := make(map[string]struct{})
	items := make([]models.AWSSecurityGroup, 0)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if paginationTokenRepeated(page.NextToken, seen) {
			return nil, ErrEC2PaginationTokenRepeated
		}
		for _, group := range page.SecurityGroups {
			inbound, err := json.Marshal(securityGroupRules(group.IpPermissions))
			if err != nil {
				return nil, err
			}
			outbound, err := json.Marshal(securityGroupRules(group.IpPermissionsEgress))
			if err != nil {
				return nil, err
			}
			items = append(items, models.AWSSecurityGroup{
				GroupID: aws.ToString(group.GroupId), GroupName: aws.ToString(group.GroupName),
				Name: EC2TagName(group.Tags), Description: aws.ToString(group.Description),
				VPCID: aws.ToString(group.VpcId), OwnerID: aws.ToString(group.OwnerId),
				Tags: datatypes.JSON(EC2TagJSON(group.Tags)), InboundRules: inbound, OutboundRules: outbound,
			})
		}
	}
	return items, nil
}

func securityGroupRules(permissions []ec2types.IpPermission) []models.SecurityGroupRule {
	rules := make([]models.SecurityGroupRule, 0)
	for _, permission := range permissions {
		base := models.SecurityGroupRule{Protocol: aws.ToString(permission.IpProtocol), FromPort: permission.FromPort, ToPort: permission.ToPort}
		// AWS ignores port values for protocol -1; avoid displaying them as restrictions.
		if base.Protocol == "-1" {
			base.FromPort, base.ToPort = nil, nil
		}
		for _, target := range permission.IpRanges {
			rule := base
			rule.TargetType, rule.Target, rule.Description = "ipv4", aws.ToString(target.CidrIp), aws.ToString(target.Description)
			rules = append(rules, rule)
		}
		for _, target := range permission.Ipv6Ranges {
			rule := base
			rule.TargetType, rule.Target, rule.Description = "ipv6", aws.ToString(target.CidrIpv6), aws.ToString(target.Description)
			rules = append(rules, rule)
		}
		for _, target := range permission.PrefixListIds {
			rule := base
			rule.TargetType, rule.Target, rule.Description = "prefix_list", aws.ToString(target.PrefixListId), aws.ToString(target.Description)
			rules = append(rules, rule)
		}
		for _, target := range permission.UserIdGroupPairs {
			rule := base
			rule.TargetType, rule.Target, rule.Description = "security_group", aws.ToString(target.GroupId), aws.ToString(target.Description)
			rule.OwnerID, rule.VPCID, rule.PeeringID = aws.ToString(target.UserId), aws.ToString(target.VpcId), aws.ToString(target.VpcPeeringConnectionId)
			rules = append(rules, rule)
		}
	}
	return rules
}
