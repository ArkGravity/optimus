package gcp

import (
	"context"
	"net/netip"
	"net/url"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"google.golang.org/api/sqladmin/v1"
)

// Instances.List covers the whole project. No regional publication is eligible
// until every project page has been collected and validated.
func (p *Provider) databases(ctx context.Context, client *sqladmin.Service, project, region string) ([]models.GCPDatabase, error) {
	rows := []models.GCPDatabase{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Instances.List(project).MaxResults(500).PageToken(token).Context(ctx).Do()
		if err != nil {
			return nil, requestError(ctx, err)
		}
		if page == nil {
			return nil, configError()
		}
		// SQL warnings have no documented empty-page success code. Even a warning
		// about an unselected region prevents claiming a complete project listing.
		if len(page.Warnings) != 0 {
			return nil, errs.ProviderError(errs.CodeAssetsPartial)
		}
		for _, item := range page.Items {
			if item == nil || !nativeName.MatchString(item.Name) || item.Project != project {
				return nil, configError()
			}
			if err := pagination.unique(item.Name); err != nil {
				return nil, err
			}
			if item.SelfLink != "" && !sqlSelfLink(item.SelfLink, project, item.Name) {
				return nil, configError()
			}
			// The SQL API also lists external database registrations. They are not
			// managed Cloud SQL inventory and need not have any GCP region.
			if item.InstanceType == "ON_PREMISES_INSTANCE" {
				continue
			}
			if !regionID.MatchString(item.Region) {
				return nil, configError()
			}
			if item.GceZone != "" && (!zoneID.MatchString(item.GceZone) || !strings.HasPrefix(item.GceZone, item.Region+"-")) {
				return nil, configError()
			}
			if item.SecondaryGceZone != "" && (!zoneID.MatchString(item.SecondaryGceZone) || !strings.HasPrefix(item.SecondaryGceZone, item.Region+"-")) {
				return nil, configError()
			}
			if item.ConnectionName != "" && item.ConnectionName != project+":"+item.Region+":"+item.Name {
				return nil, configError()
			}
			row, err := databaseRow(item)
			if err != nil {
				return nil, err
			}
			if item.Region == region {
				rows = append(rows, row)
			}
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

func sqlSelfLink(raw, project, name string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Host != "sqladmin.googleapis.com" && u.Host != "www.googleapis.com") {
		return false
	}
	for _, prefix := range []string{"/v1/", "/sql/v1/", "/sql/v1beta4/"} {
		if u.EscapedPath() == prefix+"projects/"+project+"/instances/"+name {
			return true
		}
	}
	return false
}

func databaseRow(item *sqladmin.DatabaseInstance) (models.GCPDatabase, error) {
	row := models.GCPDatabase{DatabaseID: item.Name, Name: item.Name, Product: "cloud_sql", Region: item.Region, Zone: item.GceZone, EngineVersion: item.DatabaseVersion, NativeStatus: item.State}
	for _, engine := range []string{"MYSQL", "POSTGRES", "SQLSERVER"} {
		if strings.HasPrefix(item.DatabaseVersion, engine+"_") {
			row.Engine = engine
			break
		}
	}
	facts := query.DatabaseNativeDetails{InstanceType: item.InstanceType, ConnectionName: item.ConnectionName, DNSName: item.DnsName, PSCServiceAttachmentLink: item.PscServiceAttachmentLink, SQLNetworkArchitecture: item.SqlNetworkArchitecture, DatabaseInstalledVersion: item.DatabaseInstalledVersion}
	facts.PrimaryDNSName = item.PrimaryDnsName
	facts.WriteEndpoint = item.WriteEndpoint
	facts.SecondaryGCEZone = item.SecondaryGceZone
	if item.Ipv6Address != "" {
		ip, err := netip.ParseAddr(item.Ipv6Address)
		if err != nil || !ip.Is6() || ip.Is4In6() || ip.Zone() != "" {
			return models.GCPDatabase{}, configError()
		}
		facts.IPv6Address = ip.String()
	}
	endpoints := []resource.DatabaseEndpoint{}
	seen := map[string]bool{}
	for _, ip := range item.IpAddresses {
		if ip == nil {
			return models.GCPDatabase{}, configError()
		}
		address, err := netip.ParseAddr(ip.IpAddress)
		if err != nil || address.Zone() != "" {
			return models.GCPDatabase{}, configError()
		}
		value := address.Unmap().String()
		identity := ip.Type + "/" + value
		if seen[identity] {
			return models.GCPDatabase{}, configError()
		}
		seen[identity] = true
		facts.GCPIPAddresses = append(facts.GCPIPAddresses, query.GCPDatabaseIPAddress{Address: value, Type: ip.Type})
		if ip.Type == "PRIMARY" || ip.Type == "PRIVATE" {
			endpoints = append(endpoints, resource.DatabaseEndpoint{Address: value, Type: ip.Type})
			if ip.Type == "PRIMARY" {
				public := true
				row.PubliclyAccessible = &public
			}
		}
	}
	for _, dns := range item.DnsNames {
		if dns == nil || dns.Name == "" {
			return models.GCPDatabase{}, configError()
		}
		facts.GCPDNSNames = append(facts.GCPDNSNames, query.GCPDatabaseDNSName{Name: dns.Name, ConnectionType: dns.ConnectionType, DNSScope: dns.DnsScope, RecordManager: dns.RecordManager})
	}
	labels := map[string]string{}
	if settings := item.Settings; settings != nil {
		if settings.DataDiskSizeGb < 0 {
			return models.GCPDatabase{}, configError()
		}
		if settings.DataDiskSizeGb > 0 {
			size := settings.DataDiskSizeGb
			row.StorageGB = &size
		}
		if settings.AvailabilityType == "REGIONAL" || settings.AvailabilityType == "ZONAL" {
			mode := settings.AvailabilityType
			row.HAMode = &mode
		}
		row.InstanceClass = settings.Tier
		facts.StorageType = settings.DataDiskType
		facts.AvailabilityType = settings.AvailabilityType
		if settings.UserLabels != nil {
			labels = settings.UserLabels
		}
	}
	row.Endpoints = jsonValue(endpoints)
	row.Tags = jsonValue(labels)
	row.NativeDetails = jsonValue(facts)
	return row, nil
}
