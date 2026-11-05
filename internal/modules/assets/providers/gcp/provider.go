package gcp

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"gorm.io/gorm"
)

func (*Provider) Name() string { return "gcp" }
func (p *Provider) Plan(ctx context.Context, row models.CloudAccount, key *credentials.CloudKey) (assetsync.Plan, error) {
	if row.Provider != "gcp" || !projectID.MatchString(row.ProjectID) || len(row.EnabledRegions) == 0 {
		return assetsync.Plan{}, configError()
	}
	selected := map[string]bool{}
	for _, region := range row.EnabledRegions {
		if !regionID.MatchString(region) || selected[region] {
			return assetsync.Plan{}, configError()
		}
		selected[region] = true
	}
	client, err := p.computeClient(ctx, key)
	if err != nil {
		return assetsync.Plan{}, err
	}
	zones := map[string]string{}
	pagination := pages{}
	token := ""
	// Zones.List is the authoritative project catalog. VM results are never used
	// to infer locations, and DOWN zones remain collection units.
	for {
		page, err := client.Zones.List(row.ProjectID).PageToken(token).MaxResults(500).Context(ctx).Do()
		if err != nil {
			return assetsync.Plan{}, requestError(ctx, err)
		}
		if page == nil {
			return assetsync.Plan{}, configError()
		}
		if page.Warning != nil {
			if err := partialWarning(page.Warning.Code, len(page.Items)); err != nil {
				return assetsync.Plan{}, err
			}
		}
		for _, zone := range page.Items {
			if zone == nil || !zoneID.MatchString(zone.Name) {
				return assetsync.Plan{}, configError()
			}
			if err := pagination.unique(zone.Name); err != nil {
				return assetsync.Plan{}, err
			}
			ref, err := canonicalRef(zone.Region, row.ProjectID, "regions")
			if err != nil {
				return assetsync.Plan{}, err
			}
			segments := strings.Split(ref, "/")
			if segments[1] != row.ProjectID || !regionID.MatchString(segments[3]) || !strings.HasPrefix(zone.Name, segments[3]+"-") {
				return assetsync.Plan{}, configError()
			}
			if selected[segments[3]] {
				zones[zone.Name] = segments[3]
			}
		}
		done, err := pagination.next(page.NextPageToken)
		if err != nil {
			return assetsync.Plan{}, err
		}
		if done {
			break
		}
		token = page.NextPageToken
	}
	plan := assetsync.Plan{}
	for _, kind := range []string{"network", "firewall"} {
		plan.Units = append(plan.Units, assetsync.Unit{Key: kind + "/global", ResourceType: kind, LocationType: "global", Location: "global", Budget: assetsync.DefaultAccountTimeout})
	}
	regions := append([]string(nil), row.EnabledRegions...)
	sort.Strings(regions)
	for _, region := range regions {
		for _, kind := range []string{"subnet", "database"} {
			plan.Units = append(plan.Units, assetsync.Unit{Key: kind + "/" + region, ResourceType: kind, LocationType: "region", Location: region, Region: region, Budget: assetsync.DefaultAccountTimeout})
		}
	}
	ordered := make([]string, 0, len(zones))
	for zone := range zones {
		ordered = append(ordered, zone)
	}
	sort.Strings(ordered)
	for _, zone := range ordered {
		plan.Units = append(plan.Units, assetsync.Unit{Key: "instance/" + zone, ResourceType: "instance", LocationType: "zone", Location: zone, Region: zones[zone], Zone: zone, Budget: assetsync.DefaultAccountTimeout})
	}
	return plan, nil
}

func (p *Provider) Fetch(ctx context.Context, unit assetsync.Unit, key *credentials.CloudKey) (assetsync.Snapshot, error) {
	if unit.Scope.Provider != "gcp" || !projectID.MatchString(unit.Scope.ProjectID) {
		return assetsync.Snapshot{}, configError()
	}
	loc := resource.Location{Type: unit.LocationType, Name: unit.Location, Region: unit.Region, Zone: unit.Zone}
	if loc.Validate() != nil {
		return assetsync.Snapshot{}, configError()
	}
	switch unit.ResourceType {
	case "network", "firewall":
		if loc.Type != "global" {
			return assetsync.Snapshot{}, configError()
		}
	case "subnet", "database":
		if loc.Type != "region" || !regionID.MatchString(loc.Region) {
			return assetsync.Snapshot{}, configError()
		}
	case "instance":
		if loc.Type != "zone" || !zoneID.MatchString(loc.Zone) || !regionID.MatchString(loc.Region) || !strings.HasPrefix(loc.Zone, loc.Region+"-") {
			return assetsync.Snapshot{}, configError()
		}
	default:
		return assetsync.Snapshot{}, configError()
	}
	started := time.Now().UTC()
	project := unit.Scope.ProjectID
	if unit.ResourceType == "database" {
		client, err := p.sqlClient(ctx, key)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		rows, err := p.databases(ctx, client, project, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishDatabases(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	}
	if unit.ResourceType == "firewall" {
		client, presence, err := p.firewallClient(ctx, key)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		rows, err := p.firewalls(ctx, client, presence, project)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishFirewalls(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	}
	client, err := p.computeClient(ctx, key)
	if err != nil {
		return assetsync.Snapshot{}, err
	}
	switch unit.ResourceType {
	case "instance":
		rows, err := p.instances(ctx, client, project, unit.Region, unit.Zone)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishInstances(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	case "network":
		rows, err := p.networks(ctx, client, project)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishNetworks(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	case "subnet":
		rows, err := p.subnets(ctx, client, project, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, err
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishSubnets(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	}
	return assetsync.Snapshot{}, configError()
}
