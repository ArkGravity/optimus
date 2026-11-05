package gcp

import (
	"context"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"google.golang.org/api/compute/v1"
)

type instanceDetails struct {
	NetworkTags          []string                              `json:"network_tags,omitempty"`
	ResourceManagerTags  map[string]string                     `json:"resource_manager_tags,omitempty"`
	GCPNetworkInterfaces []resource.GCPNetworkInterfaceDetails `json:"gcp_network_interfaces,omitempty"`
}

func (p *Provider) instances(ctx context.Context, client *compute.Service, project, region, zone string) ([]models.GCPInstance, error) {
	rows := []models.GCPInstance{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Instances.List(project, zone).MaxResults(500).PageToken(token).Context(ctx).Do()
		if err != nil {
			return nil, requestError(ctx, err)
		}
		if page == nil {
			return nil, configError()
		}
		if page.Warning != nil {
			if err := partialWarning(page.Warning.Code, len(page.Items)); err != nil {
				return nil, err
			}
		}
		for _, item := range page.Items {
			if item == nil || item.Id == 0 || !nativeName.MatchString(item.Name) {
				return nil, configError()
			}
			id := strconv.FormatUint(item.Id, 10)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			ref, err := canonicalRef(item.Zone, project, "zones")
			if err != nil || ref != "projects/"+project+"/zones/"+zone {
				return nil, configError()
			}
			machine := ""
			if item.MachineType != "" {
				ref, err := canonicalRef(item.MachineType, project, "machineTypes")
				if err != nil || !strings.HasPrefix(ref, "projects/"+project+"/zones/"+zone+"/") {
					return nil, configError()
				}
				machine = ref[strings.LastIndex(ref, "/")+1:]
			}
			nics, details, err := instanceNICs(item.NetworkInterfaces, project)
			if err != nil {
				return nil, err
			}
			facts := instanceDetails{GCPNetworkInterfaces: details}
			if item.Tags != nil {
				facts.NetworkTags = append([]string(nil), item.Tags.Items...)
			}
			if item.Params != nil {
				facts.ResourceManagerTags = item.Params.ResourceManagerTags
			}
			labels := item.Labels
			if labels == nil {
				labels = map[string]string{}
			}
			rows = append(rows, models.GCPInstance{InstanceID: id, Name: item.Name, InstanceType: machine, NativeStatus: item.Status, Region: region, Zone: zone, NetworkInterfaces: jsonValue(nics), Tags: jsonValue(labels), NativeDetails: jsonValue(facts)})
		}
		done, err := pagination.next(page.NextPageToken)
		if err != nil {
			return nil, err
		}
		if done {
			return rows, nil
		}
		token = page.NextPageToken
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
func prefixLength(value int64) (*int64, error) {
	if value < 0 || value > 128 {
		return nil, configError()
	}
	if value == 0 {
		return nil, nil
	}
	return &value, nil
}
func aliasRanges(items []*compute.AliasIpRange, ipv6 bool) ([]resource.GCPAliasRange, error) {
	rows := []resource.GCPAliasRange{}
	for _, item := range items {
		if item == nil {
			return nil, configError()
		}
		prefix, err := netip.ParsePrefix(item.IpCidrRange)
		if err != nil || prefix.Addr().Is6() != ipv6 {
			return nil, configError()
		}
		rows = append(rows, resource.GCPAliasRange{CIDR: prefix.Masked().String(), SubnetworkRangeName: item.SubnetworkRangeName})
	}
	return rows, nil
}
func instanceNICs(items []*compute.NetworkInterface, project string) ([]resource.NetworkInterface, []resource.GCPNetworkInterfaceDetails, error) {
	nics := []resource.NetworkInterface{}
	details := []resource.GCPNetworkInterfaceDetails{}
	names := map[string]bool{}
	for index, item := range items {
		if item == nil || item.Name != "" && names[item.Name] {
			return nil, nil, configError()
		}
		if item.Name != "" {
			names[item.Name] = true
		}
		nic := resource.NetworkInterface{ID: item.Name, Name: item.Name}
		var err error
		if item.Network != "" {
			nic.NetworkRef, err = canonicalRef(item.Network, project, "networks")
			if err != nil {
				return nil, nil, err
			}
		}
		if item.Subnetwork != "" {
			nic.SubnetRef, err = canonicalRef(item.Subnetwork, project, "subnetworks")
			if err != nil {
				return nil, nil, err
			}
		}
		// The pinned Compute API documents Ipv6Address as internal; external IPv6
		// addresses belong to each Ipv6AccessConfigs entry. Alias CIDRs stay separate.
		if err := addIP(&nic, item.NetworkIP, false); err != nil {
			return nil, nil, err
		}
		if err := addIP(&nic, item.Ipv6Address, false); err != nil {
			return nil, nil, err
		}
		fact := resource.GCPNetworkInterfaceDetails{InterfaceIndex: index, Name: item.Name, StackType: item.StackType, IPv6AccessType: item.Ipv6AccessType}
		fact.InternalIPv6PrefixLength, err = prefixLength(item.InternalIpv6PrefixLength)
		if err != nil {
			return nil, nil, err
		}
		fact.AliasIPv4Ranges, err = aliasRanges(item.AliasIpRanges, false)
		if err != nil {
			return nil, nil, err
		}
		fact.AliasIPv6Ranges, err = aliasRanges(item.AliasIpv6Ranges, true)
		if err != nil {
			return nil, nil, err
		}
		for _, access := range item.AccessConfigs {
			if access == nil {
				return nil, nil, configError()
			}
			if err := addIP(&nic, access.NatIP, true); err != nil {
				return nil, nil, err
			}
		}
		for _, access := range item.Ipv6AccessConfigs {
			if access == nil {
				return nil, nil, configError()
			}
			if err := addIP(&nic, access.ExternalIpv6, true); err != nil {
				return nil, nil, err
			}
			if access.ExternalIpv6 != "" {
				address, err := netip.ParseAddr(access.ExternalIpv6)
				if err != nil || !address.Is6() || address.Is4In6() {
					return nil, nil, configError()
				}
				length, err := prefixLength(access.ExternalIpv6PrefixLength)
				if err != nil {
					return nil, nil, err
				}
				fact.ExternalIPv6 = append(fact.ExternalIPv6, resource.GCPExternalIPv6{Name: access.Name, Address: address.String(), PrefixLength: length})
			}
		}
		nics = append(nics, nic)
		details = append(details, fact)
	}
	return nics, details, nil
}
