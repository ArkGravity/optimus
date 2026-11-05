// Package aws adapts the existing AWS fetchers to the shared collection engine.
package aws

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/awsclient"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type Identity interface {
	AccountID(context.Context, *credentials.CloudKey, string) (string, error)
}

type Options struct {
	ClientFactory  assetsync.ClientFactory
	Fetcher        assetsync.Fetcher
	Identity       Identity
	RequestTimeout time.Duration
}

type Provider struct {
	factory  assetsync.ClientFactory
	fetcher  assetsync.Fetcher
	identity Identity
	timeout  time.Duration
}

func New(options Options) *Provider {
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 30 * time.Second
	}
	if options.ClientFactory == nil {
		options.ClientFactory = assetsync.ClientFactoryFunc(awsclient.For)
	}
	if options.Fetcher == nil {
		options.Fetcher = &assetsync.CompositeFetcher{Instance: assetsync.EC2Fetcher{}, Network: assetsync.VPCFetcher{}, Database: assetsync.RDSFetcher{}, SecurityGroup: assetsync.SecurityGroupFetcher{}}
	}
	if options.Identity == nil {
		options.Identity = CallerIdentity{Timeout: options.RequestTimeout}
	}
	return &Provider{factory: options.ClientFactory, fetcher: options.Fetcher, identity: options.Identity, timeout: options.RequestTimeout}
}

func (*Provider) Name() string { return "aws" }

func (p *Provider) Plan(ctx context.Context, row models.CloudAccount, key *credentials.CloudKey) (assetsync.Plan, error) {
	if len(row.EnabledRegions) == 0 {
		return assetsync.Plan{}, configError()
	}
	id, err := p.identity.AccountID(ctx, key, row.EnabledRegions[0])
	if err != nil {
		return assetsync.Plan{}, safeError(err)
	}
	if !validAccountID(id) {
		return assetsync.Plan{}, configError()
	}
	plan := assetsync.Plan{ExternalAccountID: id}
	for _, region := range row.EnabledRegions {
		for _, resource := range []string{"instance", "network", "database", "security_group"} {
			plan.Units = append(plan.Units, assetsync.Unit{Key: resource + "/" + region, ResourceType: resource, LocationType: "region", Location: region, Region: region, Budget: assetsync.DefaultAccountTimeout})
		}
	}
	return plan, nil
}

func (p *Provider) Fetch(ctx context.Context, unit assetsync.Unit, key *credentials.CloudKey) (assetsync.Snapshot, error) {
	// Each key is newly consumed. Verify it before creating resource clients or
	// enumerating any resources, including when its auth type has not changed.
	id, err := p.identity.AccountID(ctx, key, unit.Region)
	if err != nil {
		return assetsync.Snapshot{}, errors.Join(assetsync.ErrIdentityVerification, safeError(err))
	}
	if !validAccountID(id) {
		return assetsync.Snapshot{}, errors.Join(assetsync.ErrIdentityVerification, configError())
	}
	if unit.Scope.ExternalAccountID == "" || id != unit.Scope.ExternalAccountID {
		return assetsync.Snapshot{}, apperr.New(errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch, "verified provider identity does not match this cloud account")
	}
	clients, err := p.factory.For(ctx, key, unit.Region, p.timeout)
	if err != nil {
		return assetsync.Snapshot{}, safeError(err)
	}
	started := time.Now()
	accountID, region := unit.Scope.AccountID, unit.Region
	var publish func(context.Context, *gorm.DB) (assetsync.Stats, error)
	switch unit.ResourceType {
	case "instance":
		items, err := p.fetcher.FetchInstances(ctx, clients)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		publish = func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			deleted, err := assetsync.UpsertInstancesTx(ctx, tx, accountID, region, started, items)
			return assetsync.Stats{Seen: int64(len(items)), SoftDeleted: deleted}, err
		}
	case "network":
		vpcs, subnets, err := p.fetcher.FetchVPCsAndSubnets(ctx, clients)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		publish = func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			vpcDeleted, subnetDeleted, err := assetsync.UpsertVPCsAndSubnetsTx(ctx, tx, accountID, region, started, vpcs, subnets)
			return assetsync.Stats{Seen: int64(len(vpcs)) + int64(len(subnets)), SoftDeleted: vpcDeleted + subnetDeleted}, err
		}
	case "database":
		items, err := p.fetcher.FetchDatabases(ctx, clients)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		publish = func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			deleted, err := assetsync.UpsertDatabasesTx(ctx, tx, accountID, region, started, items)
			return assetsync.Stats{Seen: int64(len(items)), SoftDeleted: deleted}, err
		}
	case "security_group":
		items, err := p.fetcher.FetchSecurityGroups(ctx, clients)
		if err != nil {
			return assetsync.Snapshot{}, safeError(err)
		}
		publish = func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
			deleted, err := assetsync.UpsertSecurityGroupsTx(ctx, tx, accountID, region, started, items)
			return assetsync.Stats{Seen: int64(len(items)), SoftDeleted: deleted}, err
		}
	default:
		return assetsync.Snapshot{}, configError()
	}
	return assetsync.Snapshot{Complete: true, Publish: publish}, nil
}

func configError() *apperr.BizError {
	return apperr.New(errs.CodeAssetsConfig, errs.KeyConfig, "invalid AWS collection configuration")
}

func safeError(err error) error {
	var safe *apperr.BizError
	if biz, ok := apperr.AsBiz(err); ok {
		safe = biz
	} else {
		code, key, message := awsclient.MapError(err)
		safe = apperr.New(code, key, message)
	}
	if errors.Is(err, context.Canceled) {
		return errors.Join(safe, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(safe, context.DeadlineExceeded)
	}
	return safe
}

func validAccountID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, char := range id {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (p *Provider) RequestTimeout() time.Duration { return p.timeout }
