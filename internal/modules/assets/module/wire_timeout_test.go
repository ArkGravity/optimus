//go:build dbtest

package module

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	awsdk "github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/config"
	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	awsprovider "github.com/ArkGravity/optimus/internal/modules/assets/providers/aws"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

type timeoutHTTPFunc func(*http.Request) (*http.Response, error)

func (f timeoutHTTPFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

type timeoutConsumer struct{}

func (timeoutConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "fixture", SecretAccessKey: "fixture-secret"}, nil
}

func TestAWSOverrideSurvivesSharedPlanningCatalogAndResourceBudgets(t *testing.T) {
	gdb, closeDB := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(closeDB)
	key := dbtest.SeedCloudKey(t, gdb, "timeout-key")
	row := dbtest.SeedCloudAccount(t, gdb, key.ID, "timeout-account", "us-east-1")
	assertBudget := func(ctx context.Context, minimum time.Duration) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Greater(t, time.Until(deadline), minimum)
	}
	response := func(body string) *http.Response {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	}
	sdkConfig := func(client awsdk.HTTPClient) awsdk.Config {
		return awsdk.Config{Region: "us-east-1", Credentials: awscreds.NewStaticCredentialsProvider("fixture", "fixture-secret", ""), HTTPClient: client, Retryer: func() awsdk.Retryer { return awsdk.NopRetryer{} }}
	}
	identityCalls, resourceCalls, catalogCalls := 0, 0, 0
	identity := awsprovider.CallerIdentity{Timeout: time.Minute, Factory: func(ctx context.Context, _ *credentials.CloudKey, _ string, timeout time.Duration) (*sts.Client, error) {
		require.Equal(t, time.Minute, timeout)
		assertBudget(ctx, 59*time.Second)
		return sts.NewFromConfig(sdkConfig(timeoutHTTPFunc(func(r *http.Request) (*http.Response, error) {
			identityCalls++
			assertBudget(r.Context(), 59*time.Second)
			return response(`<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Account>111111111111</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`), nil
		}))), nil
	}}
	factory := assetsync.ClientFactoryFunc(func(ctx context.Context, _ *credentials.CloudKey, _ string, timeout time.Duration) (*assetsync.Clients, error) {
		require.Equal(t, time.Minute, timeout)
		assertBudget(ctx, 14*time.Minute)
		cfg := sdkConfig(timeoutHTTPFunc(func(r *http.Request) (*http.Response, error) {
			resourceCalls++
			assertBudget(r.Context(), 59*time.Second)
			require.NoError(t, r.ParseForm())
			action := r.Form.Get("Action")
			if action == "DescribeDBInstances" {
				return response(`<DescribeDBInstancesResponse><DescribeDBInstancesResult><DBInstances/></DescribeDBInstancesResult></DescribeDBInstancesResponse>`), nil
			}
			return response("<" + action + "Response></" + action + "Response>"), nil
		}))
		return &assetsync.Clients{EC2: ec2.NewFromConfig(cfg), RDS: rds.NewFromConfig(cfg)}, nil
	})
	catalog := awsprovider.RegionCatalog{Timeout: time.Minute, Factory: func(ctx context.Context, _ *credentials.CloudKey, _ string, timeout time.Duration) (*assetsync.Clients, error) {
		assertBudget(ctx, 59*time.Second)
		require.Equal(t, time.Minute, timeout)
		cfg := sdkConfig(timeoutHTTPFunc(func(r *http.Request) (*http.Response, error) {
			catalogCalls++
			assertBudget(r.Context(), 59*time.Second)
			return response(`<DescribeRegionsResponse><regionInfo><item><regionName>us-east-1</regionName></item></regionInfo></DescribeRegionsResponse>`), nil
		}))
		return &assetsync.Clients{EC2: ec2.NewFromConfig(cfg)}, nil
	}}
	m := Wire(t.Context(), Input{DB: gdb, Config: config.AssetsConfig{SyncCron: "0 0 * * *", SyncStartupDelay: time.Hour, RequestTimeout: 30 * time.Second, AWSRequestTimeout: time.Minute, SyncTimeout: 25 * time.Minute}, CredentialsConsumer: timeoutConsumer{}, AWSIdentity: identity, ClientFactory: factory, RegionCatalogs: map[string]account.RegionCatalog{"aws": catalog}})
	t.Cleanup(func() { require.NoError(t, m.Shutdown(context.Background())) })
	require.NoError(t, m.Engine.RunAccount(t.Context(), row.ID, "cron", nil))
	require.Equal(t, 5, identityCalls)
	require.Equal(t, 5, resourceCalls)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/?provider=aws&cloudkey_id="+strconv.FormatUint(key.ID, 10), nil)
	m.accountHandler.RegionOptions(c)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, 1, catalogCalls)
}
