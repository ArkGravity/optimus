package aws

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	awsdk "github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/awsclient"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func fixtureConfig(client awsdk.HTTPClient) awsdk.Config {
	return awsdk.Config{Region: "us-east-1", Credentials: awscreds.NewStaticCredentialsProvider("fixture", "secret", ""), HTTPClient: client, Retryer: func() awsdk.Retryer { return awsdk.NopRetryer{} }}
}

func TestCallerIdentityUsesAuthenticatedSTSAndRejectsMalformedAccount(t *testing.T) {
	for _, id := range []string{"111111111111", "222222222222", "", "invalid"} {
		t.Run("account-"+id, func(t *testing.T) {
			resolver := CallerIdentity{Factory: func(_ context.Context, key *credentials.CloudKey, region string, _ time.Duration) (*sts.Client, error) {
				require.Equal(t, "fixture", key.AccessKeyID)
				require.Equal(t, "us-east-1", region)
				return sts.NewFromConfig(fixtureConfig(roundTripFunc(func(request *http.Request) (*http.Response, error) {
					require.Equal(t, "sts.us-east-1.amazonaws.com", request.URL.Host)
					require.Contains(t, request.Header.Get("Authorization"), "Credential=fixture/")
					body, err := io.ReadAll(request.Body)
					require.NoError(t, err)
					require.Contains(t, string(body), "Action=GetCallerIdentity")
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Account>` + id + `</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`))}, nil
				}))), nil
			}}
			got, err := resolver.AccountID(context.Background(), fixtureKey(), "us-east-1")
			if len(id) == 12 {
				require.NoError(t, err)
				require.Equal(t, id, got)
			} else {
				require.Error(t, err)
				require.Empty(t, got)
			}
		})
	}
}

func TestRegionCatalogUsesAuthenticatedDescribeRegions(t *testing.T) {
	catalog := RegionCatalog{Factory: func(_ context.Context, key *credentials.CloudKey, region string, _ time.Duration) (*awsclient.Clients, error) {
		require.Equal(t, "fixture", key.AccessKeyID)
		require.Equal(t, "us-east-1", region)
		client := ec2.NewFromConfig(fixtureConfig(roundTripFunc(func(request *http.Request) (*http.Response, error) {
			require.Contains(t, request.Header.Get("Authorization"), "Credential=fixture/")
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), "Action=DescribeRegions")
			require.Contains(t, string(body), "AllRegions=false")
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<DescribeRegionsResponse><regionInfo><item><regionName>us-west-2</regionName><optInStatus>opt-in-not-required</optInStatus></item><item><regionName>us-east-1</regionName></item></regionInfo></DescribeRegionsResponse>`))}, nil
		})))
		return &awsclient.Clients{EC2: client}, nil
	}}
	regions, err := catalog.Regions(context.Background(), fixtureKey(), "")
	require.NoError(t, err)
	require.Len(t, regions, 2)
	require.Equal(t, "us-east-1", regions[0].ID)
	require.Equal(t, "us-west-2", regions[1].ID)
}

func TestProviderPreservesRealIdentityCancellationThroughRepeatedSanitization(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, stage := range []string{"factory", "STS request"} {
			for _, operation := range []string{"Plan", "Fetch"} {
				t.Run(cause.Error()+"/"+stage+"/"+operation, func(t *testing.T) {
					identity := CallerIdentity{Factory: func(context.Context, *credentials.CloudKey, string, time.Duration) (*sts.Client, error) {
						privateCause := fmt.Errorf("private STS configuration or request material: %w", cause)
						if stage == "factory" {
							return nil, privateCause
						}
						return sts.NewFromConfig(fixtureConfig(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, privateCause }))), nil
					}}
					resourceCalls := 0
					provider := New(Options{Identity: identity, ClientFactory: assetsync.ClientFactoryFunc(func(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
						resourceCalls++
						return &assetsync.Clients{}, nil
					})})
					var err error
					if operation == "Plan" {
						plan, planErr := provider.Plan(t.Context(), models.CloudAccount{EnabledRegions: []string{"us-east-1"}}, fixtureKey())
						err = planErr
						require.Empty(t, plan.Units)
					} else {
						snapshot, fetchErr := provider.Fetch(t.Context(), assetsync.Unit{Region: "us-east-1"}, fixtureKey())
						err = fetchErr
						require.False(t, snapshot.Complete)
						require.Nil(t, snapshot.Publish)
						require.ErrorIs(t, err, assetsync.ErrIdentityVerification)
					}
					require.ErrorIs(t, err, cause)
					require.NotContains(t, err.Error(), "private STS")
					require.Zero(t, resourceCalls)
				})
			}
		}
	}
}
