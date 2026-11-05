package awsclient

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

const defaultRequestTimeout = 30 * time.Second

var endpointOverrideEnvNames = []string{
	"AWS_ENDPOINT_URL",
	"AWS_ENDPOINT_URL_EC2",
	"AWS_ENDPOINT_URL_RDS",
	"AWS_ENDPOINT_URL_STS",
}

// Clients contains the AWS service clients used by one discovery sweep.
// Instances are constructed fresh for every call to For and are never cached.
type Clients struct {
	EC2 *ec2.Client
	RDS *rds.Client
}

// For builds AWS clients for one cloud key and region. The caller owns the
// returned clients for the duration of a single sweep.
func For(ctx context.Context, cloudKey *credentials.CloudKey, region string, timeout time.Duration) (*Clients, error) {
	config, err := configFor(ctx, cloudKey, region, timeout)
	if err != nil {
		return nil, err
	}
	return &Clients{EC2: ec2.NewFromConfig(config), RDS: rds.NewFromConfig(config)}, nil
}

// STSFor builds a separate fresh identity client with the selected static key.
// Identity verification does not count as a resource-client factory call.
func STSFor(ctx context.Context, cloudKey *credentials.CloudKey, region string, timeout time.Duration) (*sts.Client, error) {
	config, err := configFor(ctx, cloudKey, region, timeout)
	if err != nil {
		return nil, err
	}
	return sts.NewFromConfig(config), nil
}

func configFor(ctx context.Context, cloudKey *credentials.CloudKey, region string, timeout time.Duration) (aws.Config, error) {
	if err := ctx.Err(); err != nil {
		return aws.Config{}, err
	}
	if cloudKey == nil || cloudKey.Provider != "aws" {
		return aws.Config{}, apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "cloud provider not supported")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return aws.Config{}, apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "AWS region is required")
	}
	if cloudKey.AuthType != "access_key" || cloudKey.AccessKeyID == "" || cloudKey.SecretAccessKey == "" || len(cloudKey.ServiceAccountJSON) != 0 {
		return aws.Config{}, apperr.New(errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "AWS static credentials are required")
	}
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	for _, envName := range endpointOverrideEnvNames {
		if value, exists := os.LookupEnv(envName); exists && strings.TrimSpace(value) != "" {
			return aws.Config{}, apperr.New(errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "AWS endpoint overrides are not allowed")
		}
	}
	// Construct the configuration directly: loading the default configuration
	// can still consult AWS_PROFILE even when profile filenames are empty.
	// No environment, shared profile, role, web-identity or metadata chain runs.
	return aws.Config{
		Region: region,
		Credentials: aws.NewCredentialsCache(awscredentials.NewStaticCredentialsProvider(
			cloudKey.AccessKeyID,
			cloudKey.SecretAccessKey,
			"",
		)),
		RetryMaxAttempts: 3,
		RetryMode:        aws.RetryModeStandard,
		HTTPClient:       &http.Client{Timeout: timeout},
	}, nil
}
