package awsclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

func TestFor_RejectsInvalidProviderAndRegion(t *testing.T) {
	tests := []struct {
		name    string
		key     *credentials.CloudKey
		code    apperr.Code
		keyName string
	}{
		{"nil", nil, errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported},
		{"non-AWS", &credentials.CloudKey{Provider: "gcp"}, errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := For(context.Background(), tt.key, "us-east-1", time.Second)
			assertBizError(t, err, tt.code, tt.keyName)
		})
	}
	_, err := For(context.Background(), validCloudKey(), "", time.Second)
	assertBizError(t, err, errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid)
}

func TestFor_BuildsFreshConfiguredClients(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	first, err := For(context.Background(), validCloudKey(), "us-east-1", 5*time.Second)
	require.NoError(t, err)
	second, err := For(context.Background(), validCloudKey(), "us-east-1", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, first.EC2)
	require.NotNil(t, first.RDS)
	require.NotSame(t, first.EC2, second.EC2)
	require.NotSame(t, first.RDS, second.RDS)

	ec2Options := first.EC2.Options()
	rdsOptions := first.RDS.Options()
	require.Equal(t, "us-east-1", ec2Options.Region)
	require.Equal(t, "us-east-1", rdsOptions.Region)
	require.Equal(t, 3, ec2Options.RetryMaxAttempts)
	require.Equal(t, 3, rdsOptions.RetryMaxAttempts)
	httpClient, ok := ec2Options.HTTPClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, 5*time.Second, httpClient.Timeout)

	creds, err := ec2Options.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "AKIATEST", creds.AccessKeyID)
	require.Equal(t, "test-secret", creds.SecretAccessKey)
}

func TestFor_NonPositiveTimeoutUsesDefault(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	clients, err := For(context.Background(), validCloudKey(), "us-west-2", 0)
	require.NoError(t, err)
	httpClient, ok := clients.EC2.Options().HTTPClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, defaultRequestTimeout, httpClient.Timeout)
}

func TestFor_RejectsEndpointOverrideEnvironment(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	for _, envName := range endpointOverrideEnvNames {
		t.Run(envName, func(t *testing.T) {
			for _, name := range endpointOverrideEnvNames {
				t.Setenv(name, "")
			}
			hostile := "https://SECRET.invalid/" + envName
			t.Setenv(envName, hostile)
			_, err := For(context.Background(), validCloudKey(), "us-east-1", time.Second)
			assertBizError(t, err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig)
			var bizErr *apperr.BizError
			require.ErrorAs(t, err, &bizErr)
			require.NotContains(t, bizErr.Message, hostile)
			require.NotContains(t, bizErr.Message, "SECRET")
		})
	}
}

func TestFor_ForcesStandardRetryMode(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	for _, name := range endpointOverrideEnvNames {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_RETRY_MODE", "adaptive")
	clients, err := For(context.Background(), validCloudKey(), "us-east-1", time.Second)
	require.NoError(t, err)
	require.Equal(t, aws.RetryModeStandard, clients.EC2.Options().RetryMode)
	require.Equal(t, aws.RetryModeStandard, clients.RDS.Options().RetryMode)
	require.Equal(t, 3, clients.EC2.Options().RetryMaxAttempts)
	require.Equal(t, 3, clients.RDS.Options().RetryMaxAttempts)
}

func TestFactoriesRejectUnusableStaticCredentials(t *testing.T) {
	for _, key := range []*credentials.CloudKey{
		{Provider: "aws", AuthType: "access_key"},
		{Provider: "aws", AuthType: "access_key", AccessKeyID: "access"},
		{Provider: "aws", AuthType: "service_account_json", AccessKeyID: "access", SecretAccessKey: "secret"},
	} {
		_, err := For(context.Background(), key, "us-east-1", time.Second)
		assertBizError(t, err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig)
		_, err = STSFor(context.Background(), key, "us-east-1", time.Second)
		assertBizError(t, err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig)
	}
}

func TestSTSFactoryUsesSelectedKeyAndRegionDespiteAmbientAuth(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_SESSION_TOKEN", "ambient-session")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::222222222222:role/ambient")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/nonexistent/ambient-token")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_PROFILE", "nonexistent-profile")
	first, err := STSFor(context.Background(), validCloudKey(), "us-west-2", time.Second)
	require.NoError(t, err)
	second, err := STSFor(context.Background(), validCloudKey(), "us-west-2", time.Second)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	options := first.Options()
	require.Equal(t, "us-west-2", options.Region)
	key, err := options.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "AKIATEST", key.AccessKeyID)
	require.Equal(t, "test-secret", key.SecretAccessKey)
	require.Empty(t, key.SessionToken)
}

func validCloudKey() *credentials.CloudKey {
	return &credentials.CloudKey{
		Provider: "aws", AuthType: "access_key", AccessKeyID: "AKIATEST", SecretAccessKey: "test-secret",
	}
}

func assertBizError(t *testing.T, err error, code apperr.Code, key string) {
	t.Helper()
	var bizErr *apperr.BizError
	require.Error(t, err)
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, code, bizErr.Code)
	require.Equal(t, key, bizErr.MessageKey)
}
