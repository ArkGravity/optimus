package aliyun

import (
	"context"
	"net/netip"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/alibabacloud-go/tea/dara"
	vpc "github.com/alibabacloud-go/vpc-20160428/v7/client"
)

func addCIDR(blocks *[]string, raw string) error {
	if raw == "" {
		return nil
	}
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return configError()
	}
	canonical := prefix.Masked().String()
	for _, old := range *blocks {
		if old == canonical {
			return nil
		}
	}
	*blocks = append(*blocks, canonical)
	return nil
}

func (p *Provider) network(ctx context.Context, key *credentials.CloudKey, region string) ([]models.AliyunVPC, []models.AliyunVSwitch, error) {
	client, err := p.vpcClient(ctx, key, region)
	if err != nil {
		return nil, nil, safeError(err)
	}
	vpcs, err := fetchVPCs(ctx, client, region)
	if err != nil {
		return nil, nil, err
	}
	switches, err := fetchVSwitches(ctx, client, region)
	if err != nil {
		return nil, nil, err
	}
	parents := map[string]bool{}
	for _, row := range vpcs {
		parents[row.NativeRef] = true
	}
	for _, row := range switches {
		if !parents[row.NetworkRef] {
			return nil, nil, configError()
		}
	}
	return vpcs, switches, nil
}

func fetchVPCs(ctx context.Context, client *vpc.Client, region string) ([]models.AliyunVPC, error) {
	rows := []models.AliyunVPC{}
	pagination := pages{}
	for page := int32(1); ; page++ {
		response, err := client.DescribeVpcsWithContext(ctx, &vpc.DescribeVpcsRequest{RegionId: dara.String(region), PageNumber: dara.Int32(page), PageSize: dara.Int32(50)}, runtimeOptions())
		if ctx.Err() != nil {
			return nil, safeError(ctx.Err())
		}
		if err != nil {
			return nil, safeError(err)
		}
		if response == nil || response.Body == nil || response.Body.Vpcs == nil {
			return nil, configError()
		}
		body := response.Body
		done, err := pagination.accept(page, body.PageNumber, body.PageSize, body.TotalCount, len(body.Vpcs.Vpc))
		if err != nil {
			return nil, err
		}
		for _, item := range body.Vpcs.Vpc {
			if item == nil {
				return nil, configError()
			}
			id := dara.StringValue(item.VpcId)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			if item.RegionId != nil && dara.StringValue(item.RegionId) != region {
				return nil, configError()
			}
			blocks := []string{}
			for _, raw := range []*string{item.CidrBlock, item.Ipv6CidrBlock} {
				if err := addCIDR(&blocks, dara.StringValue(raw)); err != nil {
					return nil, err
				}
			}
			if item.Ipv6CidrBlocks != nil {
				for _, block := range item.Ipv6CidrBlocks.Ipv6CidrBlock {
					if block == nil {
						return nil, configError()
					}
					if err := addCIDR(&blocks, dara.StringValue(block.Ipv6CidrBlock)); err != nil {
						return nil, err
					}
				}
			}
			if item.SecondaryCidrBlocks != nil {
				for _, block := range item.SecondaryCidrBlocks.SecondaryCidrBlock {
					if err := addCIDR(&blocks, dara.StringValue(block)); err != nil {
						return nil, err
					}
				}
			}
			tags := map[string]string{}
			if item.Tags != nil {
				for _, tag := range item.Tags.Tag {
					if tag == nil {
						return nil, configError()
					}
					tags[dara.StringValue(tag.Key)] = dara.StringValue(tag.Value)
				}
			}
			rows = append(rows, models.AliyunVPC{NetworkID: id, NativeRef: id, Name: dara.StringValue(item.VpcName), Region: region, CIDRBlocks: jsonValue(blocks), NativeStatus: dara.StringValue(item.Status), Tags: jsonValue(tags), NativeDetails: jsonValue(struct{}{})})
		}
		if done {
			return rows, nil
		}
	}
}

type subnetDetails struct {
	AvailableIPCount *int64 `json:"available_ip_count,omitempty"`
}

func fetchVSwitches(ctx context.Context, client *vpc.Client, region string) ([]models.AliyunVSwitch, error) {
	rows := []models.AliyunVSwitch{}
	pagination := pages{}
	for page := int32(1); ; page++ {
		response, err := client.DescribeVSwitchesWithContext(ctx, &vpc.DescribeVSwitchesRequest{RegionId: dara.String(region), PageNumber: dara.Int32(page), PageSize: dara.Int32(50)}, runtimeOptions())
		if ctx.Err() != nil {
			return nil, safeError(ctx.Err())
		}
		if err != nil {
			return nil, safeError(err)
		}
		if response == nil || response.Body == nil || response.Body.VSwitches == nil {
			return nil, configError()
		}
		body := response.Body
		done, err := pagination.accept(page, body.PageNumber, body.PageSize, body.TotalCount, len(body.VSwitches.VSwitch))
		if err != nil {
			return nil, err
		}
		for _, item := range body.VSwitches.VSwitch {
			if item == nil {
				return nil, configError()
			}
			id := dara.StringValue(item.VSwitchId)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			parent := dara.StringValue(item.VpcId)
			if parent == "" {
				return nil, configError()
			}
			blocks := []string{}
			for _, raw := range []*string{item.CidrBlock, item.Ipv6CidrBlock} {
				if err := addCIDR(&blocks, dara.StringValue(raw)); err != nil {
					return nil, err
				}
			}
			tags := map[string]string{}
			if item.Tags != nil {
				for _, tag := range item.Tags.Tag {
					if tag == nil {
						return nil, configError()
					}
					tags[dara.StringValue(tag.Key)] = dara.StringValue(tag.Value)
				}
			}
			rows = append(rows, models.AliyunVSwitch{SubnetID: id, NetworkRef: parent, Name: dara.StringValue(item.VSwitchName), Region: region, Zone: dara.StringValue(item.ZoneId), CIDRBlocks: jsonValue(blocks), Tags: jsonValue(tags), NativeDetails: jsonValue(subnetDetails{AvailableIPCount: item.AvailableIpAddressCount})})
		}
		if done {
			return rows, nil
		}
	}
}
