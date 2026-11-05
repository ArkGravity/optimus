package aliyun

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	"github.com/alibabacloud-go/tea/dara"
	"gorm.io/gorm"
)

func (*Provider) Name() string { return "aliyun" }
func (p *Provider) Plan(ctx context.Context, row models.CloudAccount, key *credentials.CloudKey) (assetsync.Plan, error) {
	if len(row.EnabledRegions) == 0 {
		return assetsync.Plan{}, configError()
	}
	for _, region := range row.EnabledRegions {
		if !regionID.MatchString(region) {
			return assetsync.Plan{}, configError()
		}
	}
	id, err := p.accountID(ctx, key, row.EnabledRegions[0])
	if err != nil {
		return assetsync.Plan{}, safeError(err)
	}
	plan := assetsync.Plan{ExternalAccountID: id}
	for _, region := range row.EnabledRegions {
		for _, kind := range []string{"instance", "network", "database", "security_group"} {
			plan.Units = append(plan.Units, assetsync.Unit{Key: kind + "/" + region, ResourceType: kind, LocationType: "region", Location: region, Region: region, Budget: assetsync.DefaultAccountTimeout})
		}
	}
	return plan, nil
}

func (p *Provider) Fetch(ctx context.Context, unit assetsync.Unit, key *credentials.CloudKey) (assetsync.Snapshot, error) {
	id, err := p.accountID(ctx, key, unit.Region)
	if err != nil {
		return assetsync.Snapshot{}, errors.Join(assetsync.ErrIdentityVerification, safeError(err))
	}
	if unit.Scope.ExternalAccountID == "" || id != unit.Scope.ExternalAccountID {
		return assetsync.Snapshot{}, apperr.New(errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch, "verified provider identity does not match this cloud account")
	}
	if unit.Scope.Provider != "aliyun" || unit.LocationType != "region" || unit.Location != unit.Region || unit.Zone != "" {
		return assetsync.Snapshot{}, configError()
	}
	started := time.Now().UTC()
	loc := resource.Location{Type: "region", Name: unit.Region, Region: unit.Region}
	switch unit.ResourceType {
	case "instance":
		rows, err := p.instances(ctx, key, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishInstances(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	case "network":
		rows, switches, err := p.network(ctx, key, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishNetwork(ctx, tx, unit.Scope, loc, started, rows, switches)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	case "database":
		rows, err := p.databases(ctx, key, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishDatabases(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	case "security_group":
		rows, err := p.firewalls(ctx, key, unit.Region)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			seen, deleted, err := PublishSecurityGroups(ctx, tx, unit.Scope, loc, started, rows)
			return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
		}}, nil
	default:
		// An unimplemented family must never prune via an empty snapshot.
		return assetsync.Snapshot{}, configError()
	}
}

type RegionCatalog struct {
	Transport http.RoundTripper
	Timeout   time.Duration
}

func (r RegionCatalog) Regions(ctx context.Context, key *credentials.CloudKey, _ string) ([]account.RegionOption, error) {
	p := New(Options{Transport: r.Transport, RequestTimeout: r.Timeout})
	ctx, cancel := context.WithTimeout(ctx, p.options.RequestTimeout)
	defer cancel()
	client, err := p.ecsClient(ctx, key, "cn-hangzhou")
	if err != nil {
		return nil, safeError(err)
	}
	response, err := client.DescribeRegionsWithContext(ctx, &ecs.DescribeRegionsRequest{}, runtimeOptions())
	if ctx.Err() != nil {
		return nil, safeError(ctx.Err())
	}
	if err != nil {
		return nil, safeError(err)
	}
	if response == nil || response.Body == nil || response.Body.Regions == nil {
		return nil, configError()
	}
	items := []account.RegionOption{}
	seen := map[string]bool{}
	for _, region := range response.Body.Regions.Region {
		if region == nil || !regionID.MatchString(dara.StringValue(region.RegionId)) {
			return nil, configError()
		}
		id := dara.StringValue(region.RegionId)
		if seen[id] {
			continue
		}
		seen[id] = true
		name := dara.StringValue(region.LocalName)
		if name == "" {
			name = id
		}
		items = append(items, account.RegionOption{ID: id, Name: name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r RegionCatalog) RequestTimeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 30 * time.Second
}
