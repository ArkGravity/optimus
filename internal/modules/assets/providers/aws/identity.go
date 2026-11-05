package aws

import (
	"context"
	"sort"
	"time"

	awsdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/awsclient"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type IdentityFactory func(context.Context, *credentials.CloudKey, string, time.Duration) (*sts.Client, error)

type CallerIdentity struct {
	Factory IdentityFactory
	Timeout time.Duration
}

func (r CallerIdentity) AccountID(ctx context.Context, key *credentials.CloudKey, region string) (string, error) {
	factory := r.Factory
	if factory == nil {
		factory = awsclient.STSFor
	}
	client, err := factory(ctx, key, region, r.Timeout)
	if err != nil {
		return "", safeError(err)
	}
	if client == nil {
		return "", configError()
	}
	response, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", safeError(err)
	}
	if response == nil || !validAccountID(awsdk.ToString(response.Account)) {
		return "", configError()
	}
	return awsdk.ToString(response.Account), nil
}

type RegionCatalog struct {
	Factory func(context.Context, *credentials.CloudKey, string, time.Duration) (*awsclient.Clients, error)
	Timeout time.Duration
}

func (r RegionCatalog) Regions(ctx context.Context, key *credentials.CloudKey, _ string) ([]account.RegionOption, error) {
	factory := r.Factory
	if factory == nil {
		factory = awsclient.For
	}
	// The bootstrap region selects an endpoint for the authenticated catalog;
	// it never expands the account's explicitly selected collection regions.
	clients, err := factory(ctx, key, "us-east-1", r.Timeout)
	if err != nil {
		return nil, safeError(err)
	}
	if clients == nil || clients.EC2 == nil {
		return nil, configError()
	}
	response, err := clients.EC2.DescribeRegions(ctx, &ec2.DescribeRegionsInput{AllRegions: awsdk.Bool(false)})
	if err != nil {
		return nil, safeError(err)
	}
	if response == nil {
		return nil, configError()
	}
	items := make([]account.RegionOption, 0, len(response.Regions))
	for _, region := range response.Regions {
		name := awsdk.ToString(region.RegionName)
		if name == "" {
			return nil, configError()
		}
		items = append(items, account.RegionOption{ID: name, Name: name})
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
