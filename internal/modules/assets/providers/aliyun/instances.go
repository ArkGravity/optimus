package aliyun

import (
	"context"
	"encoding/json"
	"net/netip"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/alibabacloud-go/tea/dara"
	"gorm.io/datatypes"
)

func jsonValue(value any) datatypes.JSON { raw, _ := json.Marshal(value); return datatypes.JSON(raw) }

type instanceDetails struct {
	CPUCount *int64 `json:"cpu_count,omitempty"`
	MemoryMB *int64 `json:"memory_mb,omitempty"`
	ImageID  string `json:"image_id,omitempty"`
	OS       string `json:"os,omitempty"`
}

func int64Fact(value *int32) *int64 {
	if value == nil {
		return nil
	}
	n := int64(*value)
	return &n
}

func (p *Provider) instances(ctx context.Context, key *credentials.CloudKey, region string) ([]models.AliyunInstance, error) {
	client, err := p.ecsClient(ctx, key, region)
	if err != nil {
		return nil, safeError(err)
	}
	rows := []models.AliyunInstance{}
	pagination := pages{}
	for page := int32(1); ; page++ {
		response, err := client.DescribeInstancesWithContext(ctx, &ecs.DescribeInstancesRequest{RegionId: dara.String(region), PageNumber: dara.Int32(page), PageSize: dara.Int32(100)}, runtimeOptions())
		if ctx.Err() != nil {
			return nil, safeError(ctx.Err())
		}
		if err != nil {
			return nil, safeError(err)
		}
		if response == nil || response.Body == nil || response.Body.Instances == nil || dara.StringValue(response.Body.NextToken) != "" {
			return nil, configError()
		}
		body := response.Body
		done, err := pagination.accept(page, body.PageNumber, body.PageSize, body.TotalCount, len(body.Instances.Instance))
		if err != nil {
			return nil, err
		}
		for _, item := range body.Instances.Instance {
			if item == nil {
				return nil, configError()
			}
			id := dara.StringValue(item.InstanceId)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			if item.RegionId != nil && dara.StringValue(item.RegionId) != region {
				return nil, configError()
			}
			nics, err := p.instanceNICs(ctx, client, region, item)
			if err != nil {
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
			rows = append(rows, models.AliyunInstance{InstanceID: id, Name: dara.StringValue(item.InstanceName), InstanceType: dara.StringValue(item.InstanceType), NativeStatus: dara.StringValue(item.Status), Region: region, Zone: dara.StringValue(item.ZoneId), NetworkInterfaces: jsonValue(nics), Tags: jsonValue(tags), NativeDetails: jsonValue(instanceDetails{CPUCount: int64Fact(item.Cpu), MemoryMB: int64Fact(item.Memory), ImageID: dara.StringValue(item.ImageId), OS: dara.StringValue(item.OSType)})})
		}
		if done {
			return rows, nil
		}
	}
}

func addIP(nic *resource.NetworkInterface, raw string, public bool) error {
	if raw == "" {
		return nil
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil || ip.Zone() != "" {
		return configError()
	}
	ip = ip.Unmap()
	list := &nic.PrivateIPv6
	if ip.Is4() {
		list = &nic.PrivateIPv4
	}
	if public {
		if ip.Is4() {
			list = &nic.PublicIPv4
		} else {
			list = &nic.PublicIPv6
		}
	}
	for _, old := range *list {
		if old == ip.String() {
			return nil
		}
	}
	*list = append(*list, ip.String())
	return nil
}

// An instance-level address may also be associated explicitly with a secondary
// NIC. Enriched associations win over the primary-NIC fallback.
func addInstanceIP(nics []resource.NetworkInterface, fallback *resource.NetworkInterface, raw string, public bool) error {
	if raw == "" {
		return nil
	}
	address, err := netip.ParseAddr(raw)
	if err != nil || address.Zone() != "" {
		return configError()
	}
	canonical := address.Unmap().String()
	for _, nic := range nics {
		lists := [][]string{nic.PrivateIPv4, nic.PrivateIPv6}
		if public {
			lists = [][]string{nic.PublicIPv4, nic.PublicIPv6}
		}
		for _, list := range lists {
			for _, existing := range list {
				if existing == canonical {
					return nil
				}
			}
		}
	}
	return addIP(fallback, canonical, public)
}

func (p *Provider) instanceNICs(ctx context.Context, client *ecs.Client, region string, item *ecs.DescribeInstancesResponseBodyInstancesInstance) ([]resource.NetworkInterface, error) {
	nics := []resource.NetworkInterface{}
	indices := map[string]int{}
	primary := -1
	pagination := pages{}
	id := dara.StringValue(item.InstanceId)
	for page := int32(1); ; page++ {
		response, err := client.DescribeNetworkInterfacesWithContext(ctx, &ecs.DescribeNetworkInterfacesRequest{RegionId: dara.String(region), InstanceId: dara.String(id), PageNumber: dara.Int32(page), PageSize: dara.Int32(100)}, runtimeOptions())
		if ctx.Err() != nil {
			return nil, safeError(ctx.Err())
		}
		if err != nil {
			return nil, safeError(err)
		}
		if response == nil || response.Body == nil || response.Body.NetworkInterfaceSets == nil || dara.StringValue(response.Body.NextToken) != "" {
			return nil, configError()
		}
		body := response.Body
		done, err := pagination.accept(page, body.PageNumber, body.PageSize, body.TotalCount, len(body.NetworkInterfaceSets.NetworkInterfaceSet))
		if err != nil {
			return nil, err
		}
		for _, eni := range body.NetworkInterfaceSets.NetworkInterfaceSet {
			if eni == nil {
				return nil, configError()
			}
			if err := pagination.unique(dara.StringValue(eni.NetworkInterfaceId)); err != nil {
				return nil, err
			}
			// Only the requested ECS instance's attached compute NICs belong here.
			kind := dara.StringValue(eni.Type)
			if dara.StringValue(eni.InstanceId) != id || (kind != "Primary" && kind != "Secondary") {
				continue
			}
			nic := resource.NetworkInterface{ID: dara.StringValue(eni.NetworkInterfaceId), Name: dara.StringValue(eni.NetworkInterfaceName), NetworkRef: dara.StringValue(eni.VpcId), SubnetRef: dara.StringValue(eni.VSwitchId)}
			if nic.NetworkRef == "" || nic.SubnetRef == "" {
				return nil, configError()
			}
			if err := addIP(&nic, dara.StringValue(eni.PrivateIpAddress), false); err != nil {
				return nil, err
			}
			if eni.AssociatedPublicIp != nil {
				if err := addIP(&nic, dara.StringValue(eni.AssociatedPublicIp.PublicIpAddress), true); err != nil {
					return nil, err
				}
			}
			if eni.PrivateIpSets != nil {
				for _, ip := range eni.PrivateIpSets.PrivateIpSet {
					if ip == nil {
						return nil, configError()
					}
					if err := addIP(&nic, dara.StringValue(ip.PrivateIpAddress), false); err != nil {
						return nil, err
					}
					if ip.AssociatedPublicIp != nil {
						if err := addIP(&nic, dara.StringValue(ip.AssociatedPublicIp.PublicIpAddress), true); err != nil {
							return nil, err
						}
					}
				}
			}
			// IPv6Sets supplies assigned interface addresses, not an internet-access
			// guarantee. Preserve them as NIC addresses without guessing exposure.
			if eni.Ipv6Sets != nil {
				for _, ip := range eni.Ipv6Sets.Ipv6Set {
					if ip == nil {
						return nil, configError()
					}
					if err := addIP(&nic, dara.StringValue(ip.Ipv6Address), false); err != nil {
						return nil, err
					}
				}
			}
			if kind == "Primary" {
				if primary != -1 {
					return nil, configError()
				}
				primary = len(nics)
			}
			indices[nic.ID] = len(nics)
			nics = append(nics, nic)
		}
		if done {
			break
		}
	}
	if item.NetworkInterfaces != nil {
		for _, inline := range item.NetworkInterfaces.NetworkInterface {
			if inline == nil {
				return nil, configError()
			}
			kind := dara.StringValue(inline.Type)
			if kind != "Primary" && kind != "Secondary" {
				continue
			}
			index, ok := indices[dara.StringValue(inline.NetworkInterfaceId)]
			if !ok {
				return nil, configError()
			}
			nic := &nics[index]
			if err := addIP(nic, dara.StringValue(inline.PrimaryIpAddress), false); err != nil {
				return nil, err
			}
			if inline.PrivateIpSets != nil {
				for _, ip := range inline.PrivateIpSets.PrivateIpSet {
					if ip == nil {
						return nil, configError()
					}
					if err := addIP(nic, dara.StringValue(ip.PrivateIpAddress), false); err != nil {
						return nil, err
					}
				}
			}
			if inline.Ipv6Sets != nil {
				for _, ip := range inline.Ipv6Sets.Ipv6Set {
					if ip == nil {
						return nil, configError()
					}
					if err := addIP(nic, dara.StringValue(ip.Ipv6Address), false); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	// Instance-level addresses are associated with the primary NIC when known;
	// otherwise preserve them in a separate interface with only known scope.
	fallback := resource.NetworkInterface{}
	if item.VpcAttributes != nil {
		fallback.NetworkRef = dara.StringValue(item.VpcAttributes.VpcId)
		fallback.SubnetRef = dara.StringValue(item.VpcAttributes.VSwitchId)
	}
	target := &fallback
	if primary != -1 {
		target = &nics[primary]
	}
	if item.VpcAttributes != nil && item.VpcAttributes.PrivateIpAddress != nil {
		for _, ip := range item.VpcAttributes.PrivateIpAddress.IpAddress {
			if err := addInstanceIP(nics, target, dara.StringValue(ip), false); err != nil {
				return nil, err
			}
		}
	}
	if item.InnerIpAddress != nil {
		for _, ip := range item.InnerIpAddress.IpAddress {
			if err := addInstanceIP(nics, target, dara.StringValue(ip), false); err != nil {
				return nil, err
			}
		}
	}
	if item.PublicIpAddress != nil {
		for _, ip := range item.PublicIpAddress.IpAddress {
			if err := addInstanceIP(nics, target, dara.StringValue(ip), true); err != nil {
				return nil, err
			}
		}
	}
	if item.EipAddress != nil {
		if err := addInstanceIP(nics, target, dara.StringValue(item.EipAddress.IpAddress), true); err != nil {
			return nil, err
		}
	}
	if primary == -1 && len(fallback.PrivateIPv4)+len(fallback.PrivateIPv6)+len(fallback.PublicIPv4)+len(fallback.PublicIPv6) > 0 {
		nics = append(nics, fallback)
	}
	return nics, nil
}
