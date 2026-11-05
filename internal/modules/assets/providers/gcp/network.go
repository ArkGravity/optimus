package gcp

import (
	"context"
	"encoding/json"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"google.golang.org/api/compute/v1"
	"gorm.io/datatypes"
)

// canonicalRef normalizes recognized Compute self-links without following URLs.
// References to Shared VPC host projects deliberately retain that project.
func canonicalRef(raw, project, kind string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "?#%\\") {
		return "", configError()
	}
	value := raw
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.User != nil || (u.Host != "www.googleapis.com" && u.Host != "compute.googleapis.com") {
			return "", configError()
		}
		value = strings.TrimPrefix(u.Path, "/compute/v1/")
		if value == u.Path {
			return "", configError()
		}
	}
	if !strings.HasPrefix(value, "projects/") {
		value = "projects/" + project + "/" + value
	}
	parts := strings.Split(value, "/")
	if len(parts) < 4 || parts[0] != "projects" || !projectID.MatchString(parts[1]) {
		return "", configError()
	}
	for _, part := range parts {
		if !nativeName.MatchString(part) && part != parts[1] {
			return "", configError()
		}
	}
	switch kind {
	case "regions", "zones":
		if len(parts) != 4 || parts[2] != kind {
			return "", configError()
		}
	case "networks", "firewalls":
		if len(parts) != 5 || parts[2] != "global" || parts[3] != kind {
			return "", configError()
		}
	case "machineTypes":
		if len(parts) != 6 || parts[2] != "zones" || parts[4] != kind || !zoneID.MatchString(parts[3]) {
			return "", configError()
		}
	case "subnetworks":
		if len(parts) != 6 || parts[2] != "regions" || parts[4] != kind || !regionID.MatchString(parts[3]) {
			return "", configError()
		}
	default:
		return "", configError()
	}
	return value, nil
}

func jsonValue(value any) datatypes.JSON { raw, _ := json.Marshal(value); return datatypes.JSON(raw) }
func partialWarning(code string, count int) error {
	if code != "NO_RESULTS_ON_PAGE" || count != 0 {
		return errs.ProviderError(errs.CodeAssetsPartial)
	}
	return nil
}
func appendCIDR(rows *[]string, raw string) error {
	if raw == "" {
		return nil
	}
	prefix, err := netip.ParsePrefix(raw)
	if err != nil || prefix.Addr().Zone() != "" {
		return configError()
	}
	value := prefix.Masked().String()
	for _, old := range *rows {
		if value == old {
			return nil
		}
	}
	*rows = append(*rows, value)
	return nil
}

type networkDetails struct {
	MTU                   *int64 `json:"mtu,omitempty"`
	RoutingMode           string `json:"routing_mode,omitempty"`
	AutoCreateSubnetworks *bool  `json:"auto_create_subnetworks,omitempty"`
}

func (p *Provider) networks(ctx context.Context, client *compute.Service, project string) ([]models.GCPNetwork, error) {
	rows := []models.GCPNetwork{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Networks.List(project).MaxResults(500).PageToken(token).Context(ctx).Do()
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
			ref := item.SelfLink
			if ref == "" {
				ref = "projects/" + project + "/global/networks/" + item.Name
			}
			ref, err = canonicalRef(ref, project, "networks")
			if err != nil || ref != "projects/"+project+"/global/networks/"+item.Name {
				return nil, configError()
			}
			cidrs := []string{}
			if err := appendCIDR(&cidrs, item.IPv4Range); err != nil {
				return nil, err
			}
			details := networkDetails{AutoCreateSubnetworks: &item.AutoCreateSubnetworks}
			if item.Mtu > 0 {
				details.MTU = &item.Mtu
			}
			if item.RoutingConfig != nil {
				details.RoutingMode = item.RoutingConfig.RoutingMode
			}
			rows = append(rows, models.GCPNetwork{NetworkID: id, Name: item.Name, NativeRef: ref, CIDRBlocks: jsonValue(cidrs), Tags: jsonValue(map[string]string{}), NativeDetails: jsonValue(details)})
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

type subnetDetails struct {
	Purpose               string `json:"purpose,omitempty"`
	PrivateIPGoogleAccess *bool  `json:"private_ip_google_access,omitempty"`
}

func (p *Provider) subnets(ctx context.Context, client *compute.Service, project, region string) ([]models.GCPSubnet, error) {
	rows := []models.GCPSubnet{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Subnetworks.List(project, region).MaxResults(500).PageToken(token).Context(ctx).Do()
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
			ref, err := canonicalRef(item.Region, project, "regions")
			if err != nil || ref != "projects/"+project+"/regions/"+region {
				return nil, configError()
			}
			network, err := canonicalRef(item.Network, project, "networks")
			if err != nil {
				return nil, err
			}
			cidrs := []string{}
			for _, raw := range []string{item.IpCidrRange, item.InternalIpv6Prefix, item.ExternalIpv6Prefix, item.Ipv6CidrRange} {
				if err := appendCIDR(&cidrs, raw); err != nil {
					return nil, err
				}
			}
			for _, secondary := range item.SecondaryIpRanges {
				if secondary == nil {
					return nil, configError()
				}
				if err := appendCIDR(&cidrs, secondary.IpCidrRange); err != nil {
					return nil, err
				}
			}
			rows = append(rows, models.GCPSubnet{SubnetID: id, Name: item.Name, NetworkRef: network, Region: region, CIDRBlocks: jsonValue(cidrs), Tags: jsonValue(map[string]string{}), NativeDetails: jsonValue(subnetDetails{Purpose: item.Purpose, PrivateIPGoogleAccess: &item.PrivateIpGoogleAccess})})
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
