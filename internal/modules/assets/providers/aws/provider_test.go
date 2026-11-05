package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type identityFunc func(context.Context, *credentials.CloudKey, string) (string, error)

func (f identityFunc) AccountID(ctx context.Context, key *credentials.CloudKey, region string) (string, error) {
	return f(ctx, key, region)
}

type fixtureFetcher struct {
	calls []string
	err   error
}

func (f *fixtureFetcher) FetchInstances(context.Context, *assetsync.Clients) ([]models.AWSInstance, error) {
	f.calls = append(f.calls, "instance")
	return nil, f.err
}
func (f *fixtureFetcher) FetchVPCsAndSubnets(context.Context, *assetsync.Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	f.calls = append(f.calls, "network")
	return nil, nil, f.err
}
func (f *fixtureFetcher) FetchDatabases(context.Context, *assetsync.Clients) ([]models.AWSDatabase, error) {
	f.calls = append(f.calls, "database")
	return nil, f.err
}
func (f *fixtureFetcher) FetchSecurityGroups(context.Context, *assetsync.Clients) ([]models.AWSSecurityGroup, error) {
	f.calls = append(f.calls, "security_group")
	return nil, f.err
}

func fixtureKey() *credentials.CloudKey {
	return &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "fixture", SecretAccessKey: "fixture-secret"}
}

func TestProviderPlansAndFetchesFourUnitsWithFreshIdentityAndClients(t *testing.T) {
	identityCalls, factoryCalls := 0, 0
	fetcher := &fixtureFetcher{}
	p := New(Options{
		Identity: identityFunc(func(_ context.Context, key *credentials.CloudKey, region string) (string, error) {
			identityCalls++
			require.Equal(t, "fixture", key.AccessKeyID)
			require.Equal(t, "us-east-1", region)
			return "111111111111", nil
		}),
		ClientFactory: assetsync.ClientFactoryFunc(func(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
			factoryCalls++
			return &assetsync.Clients{}, nil
		}),
		Fetcher: fetcher,
	})
	row := models.CloudAccount{ID: 7, Provider: "aws", EnabledRegions: []string{"us-east-1"}}
	plan, err := p.Plan(context.Background(), row, fixtureKey())
	require.NoError(t, err)
	require.Equal(t, "aws", p.Name())
	require.Equal(t, "111111111111", plan.ExternalAccountID)
	require.Len(t, plan.Units, 4)
	require.Zero(t, factoryCalls, "planning must not construct resource clients")
	for i := range plan.Units {
		unit := plan.Units[i]
		require.Equal(t, "region", unit.LocationType)
		require.Equal(t, "us-east-1", unit.Location)
		unit.Scope.ExternalAccountID = plan.ExternalAccountID
		snapshot, err := p.Fetch(context.Background(), unit, fixtureKey())
		require.NoError(t, err)
		require.True(t, snapshot.Complete, "a complete empty sweep is authoritative")
		require.NotNil(t, snapshot.Publish)
	}
	require.Equal(t, []string{"instance", "network", "database", "security_group"}, fetcher.calls)
	require.Equal(t, 5, identityCalls)
	require.Equal(t, 4, factoryCalls)
}

func TestProviderRejectsRotatedAccountBeforeResourceFactory(t *testing.T) {
	factoryCalls := 0
	p := New(Options{
		Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "222222222222", nil }),
		ClientFactory: assetsync.ClientFactoryFunc(func(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
			factoryCalls++
			return &assetsync.Clients{}, nil
		}),
	})
	unit := assetsync.Unit{ResourceType: "instance", Region: "us-east-1", Scope: assetsync.Scope{ExternalAccountID: "111111111111"}}
	snapshot, err := p.Fetch(context.Background(), unit, fixtureKey())
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errs.CodeAssetsIdentityMismatch, biz.Code)
	require.False(t, snapshot.Complete)
	require.Zero(t, factoryCalls)
}

func TestProviderPreservesIdentityCancellation(t *testing.T) {
	p := New(Options{Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "", context.Canceled })})
	_, err := p.Fetch(context.Background(), assetsync.Unit{Region: "us-east-1"}, fixtureKey())
	require.ErrorIs(t, err, context.Canceled)
}

func TestProviderFetchFailureNeverReturnsPublisher(t *testing.T) {
	for _, resource := range []string{"instance", "network", "database", "security_group"} {
		t.Run(resource, func(t *testing.T) {
			p := New(Options{
				Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "111111111111", nil }),
				ClientFactory: assetsync.ClientFactoryFunc(func(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
					return &assetsync.Clients{}, nil
				}),
				Fetcher: &fixtureFetcher{err: errors.New("partial-page-secret")},
			})
			unit := assetsync.Unit{ResourceType: resource, Region: "us-east-1", Scope: assetsync.Scope{ExternalAccountID: "111111111111"}}
			snapshot, err := p.Fetch(context.Background(), unit, fixtureKey())
			require.Error(t, err)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
			require.NotContains(t, err.Error(), "partial-page-secret")
		})
	}
}

// nativeFixture drives the existing SDK serializers, paginators and mapping
// functions through the adapter, with no fixture credentials sent to a network.
type nativeFixture struct {
	calls      map[string]int
	empty      bool
	failSecond string
}

func (f *nativeFixture) clients(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
	config := fixtureConfig(roundTripFunc(f.response))
	return &assetsync.Clients{EC2: ec2.NewFromConfig(config), RDS: rds.NewFromConfig(config)}, nil
}

func (f *nativeFixture) response(request *http.Request) (*http.Response, error) {
	if err := request.ParseForm(); err != nil {
		return nil, err
	}
	action := request.Form.Get("Action")
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[action]++
	second := request.Form.Get("NextToken") == "second" || request.Form.Get("Marker") == "second"
	if second && action == f.failSecond {
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<Response><Errors><Error><Code>UnauthorizedOperation</Code><Message>private-secret</Message></Error></Errors></Response>`))}, nil
	}
	next := "second"
	if second || f.empty {
		next = ""
	}
	id := "first"
	if second {
		id = "second"
	}
	body := ""
	switch action {
	case "DescribeInstances":
		if !f.empty {
			body = `<reservationSet><item><instancesSet><item><instanceId>i-` + id + `</instanceId><instanceType>t3.micro</instanceType><tagSet><item><key>Name</key><value>legacy-name</value></item></tagSet></item></instancesSet></item></reservationSet>`
		}
	case "DescribeVpcs":
		if !f.empty {
			body = `<vpcSet><item><vpcId>vpc-` + id + `</vpcId><cidrBlock>10.0.0.0/16</cidrBlock></item></vpcSet>`
		}
	case "DescribeSubnets":
		if !f.empty {
			body = `<subnetSet><item><subnetId>subnet-` + id + `</subnetId><vpcId>vpc-` + id + `</vpcId><availabilityZone>us-east-1a</availabilityZone></item></subnetSet>`
		}
	case "DescribeSecurityGroups":
		if !f.empty {
			body = `<securityGroupInfo><item><groupId>sg-` + id + `</groupId><groupName>fixture</groupName><ipPermissions><item><ipProtocol>tcp</ipProtocol><fromPort>443</fromPort><toPort>443</toPort><ipRanges><item><cidrIp>10.0.0.0/8</cidrIp></item></ipRanges></item></ipPermissions></item></securityGroupInfo>`
		}
	case "DescribeDBInstances":
		if !f.empty {
			body = `<DBInstances><DBInstance><DBInstanceIdentifier>db-` + id + `</DBInstanceIdentifier><Engine>postgres</Engine></DBInstance></DBInstances>`
		}
		body = `<DescribeDBInstancesResult>` + body + `<Marker>` + next + `</Marker></DescribeDBInstancesResult>`
	default:
		return nil, fmt.Errorf("unexpected AWS action %q", action)
	}
	if action != "DescribeDBInstances" {
		body += `<nextToken>` + next + `</nextToken>`
	}
	body = `<` + action + `Response>` + body + `</` + action + `Response>`
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestProviderUsesExistingPaginatorsForEveryResource(t *testing.T) {
	for _, mode := range []string{"complete", "empty", "partial-security-group"} {
		t.Run(mode, func(t *testing.T) {
			fixture := &nativeFixture{empty: mode == "empty"}
			if mode == "partial-security-group" {
				fixture.failSecond = "DescribeSecurityGroups"
			}
			p := New(Options{ClientFactory: assetsync.ClientFactoryFunc(fixture.clients), Identity: identityFunc(func(context.Context, *credentials.CloudKey, string) (string, error) { return "111111111111", nil })})
			for _, resource := range []string{"instance", "network", "database", "security_group"} {
				unit := assetsync.Unit{ResourceType: resource, Region: "us-east-1", Scope: assetsync.Scope{ExternalAccountID: "111111111111"}}
				snapshot, err := p.Fetch(context.Background(), unit, fixtureKey())
				if resource == "security_group" && fixture.failSecond != "" {
					require.Error(t, err)
					require.False(t, snapshot.Complete)
					require.Nil(t, snapshot.Publish)
					require.NotContains(t, err.Error(), "private-secret")
				} else {
					require.NoError(t, err)
					require.True(t, snapshot.Complete)
					require.NotNil(t, snapshot.Publish)
				}
			}
			for _, action := range []string{"DescribeInstances", "DescribeVpcs", "DescribeSubnets", "DescribeDBInstances", "DescribeSecurityGroups"} {
				want := 2
				if fixture.empty {
					want = 1
				}
				require.Equal(t, want, fixture.calls[action], action)
			}
		})
	}
}
