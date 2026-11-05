package aliyun

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/credentials"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v7/client"
	sts "github.com/alibabacloud-go/sts-20150401/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	vpc "github.com/alibabacloud-go/vpc-20160428/v7/client"
)

var regionID = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)+$`)

type Options struct {
	RequestTimeout time.Duration
	// Transport is a process composition seam, never account-controlled input.
	Transport http.RoundTripper
}

type Provider struct{ options Options }

func New(options Options) *Provider {
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 30 * time.Second
	}
	return &Provider{options: options}
}

// STS's pinned generated API lacks a context argument. The SDK's documented
// HttpClient seam binds cancellation before sending the SDK-signed request.
type sdkHTTPClient struct {
	ctx       context.Context
	timeout   time.Duration
	transport http.RoundTripper
}

func (c sdkHTTPClient) Call(req *http.Request, transport *http.Transport) (*http.Response, error) {
	roundTripper := c.transport
	if roundTripper == nil {
		roundTripper = transport
		defer transport.CloseIdleConnections()
	}
	client := http.Client{Transport: roundTripper, Timeout: c.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client.Do(req.WithContext(c.ctx))
}

func (p *Provider) config(ctx context.Context, key *credentials.CloudKey, region string) (*openapi.Config, error) {
	if key == nil || key.Provider != "aliyun" || key.AuthType != "access_key" || key.AccessKeyID == "" || key.SecretAccessKey == "" || len(key.ServiceAccountJSON) != 0 || !regionID.MatchString(region) {
		return nil, configError()
	}
	ms := int(p.options.RequestTimeout.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return &openapi.Config{
		AccessKeyId:     dara.String(key.AccessKeyID),
		AccessKeySecret: dara.String(key.SecretAccessKey),
		Type:            dara.String("access_key"),
		RegionId:        dara.String(region),
		ConnectTimeout:  dara.Int(ms),
		ReadTimeout:     dara.Int(ms),
		RetryOptions:    &dara.RetryOptions{Retryable: false, MaxAttempts: 1},
		HttpClient:      sdkHTTPClient{ctx: ctx, timeout: p.options.RequestTimeout, transport: p.options.Transport},
	}, nil
}

func runtimeOptions() *dara.RuntimeOptions {
	return &dara.RuntimeOptions{}
}

func (p *Provider) accountID(ctx context.Context, key *credentials.CloudKey, region string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.RequestTimeout)
	defer cancel()
	cfg, err := p.config(ctx, key, region)
	if err != nil {
		return "", err
	}
	client, err := sts.NewClient(cfg)
	if err != nil {
		return "", safeError(err)
	}
	result, err := client.GetCallerIdentityWithOptions(runtimeOptions())
	if ctx.Err() != nil {
		return "", safeError(ctx.Err())
	}
	if err != nil {
		return "", safeError(err)
	}
	if result == nil || result.Body == nil || !validAccountID(dara.StringValue(result.Body.AccountId)) {
		return "", configError()
	}
	return dara.StringValue(result.Body.AccountId), nil
}

func validAccountID(id string) bool {
	if len(id) < 1 || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (p *Provider) ecsClient(ctx context.Context, key *credentials.CloudKey, region string) (*ecs.Client, error) {
	cfg, err := p.config(ctx, key, region)
	if err != nil {
		return nil, err
	}
	return ecs.NewClient(cfg)
}
func (p *Provider) vpcClient(ctx context.Context, key *credentials.CloudKey, region string) (*vpc.Client, error) {
	cfg, err := p.config(ctx, key, region)
	if err != nil {
		return nil, err
	}
	return vpc.NewClient(cfg)
}

// Page-number APIs declare a stable total and echo the effective page size.
// Reject partial pages, changing totals and repeated pages before publication.
type pages struct {
	total, size, seen int32
	ids               map[string]bool
}

func (p *pages) accept(page int32, number, size, total *int32, count int) (bool, error) {
	if number == nil || size == nil || total == nil || *number != page || *size < 1 || *size > 100 || *total < 0 || count > int(*size) || page > 10000 {
		return false, configError()
	}
	if page == 1 {
		p.total = *total
		p.size = *size
		p.ids = map[string]bool{}
	} else if p.total != *total || p.size != *size {
		return false, configError()
	}
	p.seen += int32(count)
	if p.seen > p.total || (p.seen < p.total && count < int(p.size)) {
		return false, configError()
	}
	return p.seen == p.total, nil
}
func (p *pages) unique(id string) error {
	if id == "" || p.ids[id] {
		return configError()
	}
	p.ids[id] = true
	return nil
}

func (p *Provider) RequestTimeout() time.Duration { return p.options.RequestTimeout }
