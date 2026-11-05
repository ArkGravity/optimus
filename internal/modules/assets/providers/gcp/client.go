package gcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/sqladmin/v1"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
const tokenURL = "https://oauth2.googleapis.com/token"
const computeEndpoint = "https://compute.googleapis.com/compute/v1/"
const sqlEndpoint = "https://sqladmin.googleapis.com/"

var regionID = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+[0-9]+$`)
var zoneID = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+[0-9]+-[a-z]+$`)
var projectID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
var nativeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

type Options struct {
	RequestTimeout time.Duration
	// Transport is controlled by process composition, never account data.
	Transport http.RoundTripper
}
type Provider struct{ options Options }

func New(options Options) *Provider {
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 30 * time.Second
	}
	return &Provider{options: options}
}

func (p *Provider) httpClient(ctx context.Context, key *credentials.CloudKey) (*http.Client, error) {
	if key == nil || key.Provider != "gcp" || key.AuthType != "service_account_json" || key.AccessKeyID != "" || key.SecretAccessKey != "" {
		return nil, configError()
	}
	canonical, err := credentials.ValidateServiceAccountJSON(key.ServiceAccountJSON)
	if err != nil {
		return nil, configError()
	}
	defer func() {
		for i := range canonical {
			canonical[i] = 0
		}
	}()
	var fields struct {
		Email      string `json:"client_email"`
		PrivateKey string `json:"private_key"`
		KeyID      string `json:"private_key_id"`
	}
	if json.Unmarshal(canonical, &fields) != nil {
		return nil, configError()
	}
	// Construct only the validated service-account flow. No ADC, credential
	// discovery, impersonation, executable source or arbitrary URLs are available.
	cfg := jwt.Config{Email: fields.Email, PrivateKey: []byte(fields.PrivateKey), PrivateKeyID: fields.KeyID, Scopes: []string{cloudPlatformScope}, TokenURL: tokenURL}
	transport := p.options.Transport
	if transport == nil {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: p.options.RequestTimeout, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: p.options.RequestTimeout, ResponseHeaderTimeout: p.options.RequestTimeout, IdleConnTimeout: 30 * time.Second}
	}
	rejectRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	source := oauth2.ReuseTokenSource(nil, boundedJWTSource{ctx: ctx, config: cfg, transport: transport, timeout: p.options.RequestTimeout})
	return &http.Client{Transport: &oauth2.Transport{Base: transport, Source: source}, Timeout: p.options.RequestTimeout, CheckRedirect: rejectRedirect}, nil
}
func (p *Provider) computeClient(ctx context.Context, key *credentials.CloudKey) (*compute.Service, error) {
	opts, err := p.clientOptions(ctx, key, computeEndpoint)
	if err != nil {
		return nil, err
	}
	service, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, safeError(err)
	}
	return service, nil
}
func (p *Provider) sqlClient(ctx context.Context, key *credentials.CloudKey) (*sqladmin.Service, error) {
	opts, err := p.clientOptions(ctx, key, sqlEndpoint)
	if err != nil {
		return nil, err
	}
	service, err := sqladmin.NewService(ctx, opts...)
	if err != nil {
		return nil, safeError(err)
	}
	return service, nil
}

func (p *Provider) clientOptions(ctx context.Context, key *credentials.CloudKey, endpoint string, wrappers ...func(http.RoundTripper) http.RoundTripper) ([]option.ClientOption, error) {
	// SDK v0.290.0 discovers default client certificates before returning an
	// explicit HTTP client, and rejects WithHTTPClient + WithClientCertSource.
	// Fail closed before that discovery can load or execute an ambient signer.
	if setting := os.Getenv("GOOGLE_API_USE_CLIENT_CERTIFICATE"); setting != "" && !strings.EqualFold(setting, "false") {
		return nil, configError()
	}
	client, err := p.httpClient(ctx, key)
	if err != nil {
		return nil, err
	}
	for _, wrap := range wrappers {
		client.Transport = wrap(client.Transport)
	}
	// SDK debug logging otherwise honors ambient configuration and records entire
	// API responses, including fields deliberately excluded from our inventory.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return []option.ClientOption{option.WithHTTPClient(client), option.WithUniverseDomain("googleapis.com"), option.WithEndpoint(endpoint), option.WithLogger(logger)}, nil
}

type pages struct {
	tokens map[string]bool
	ids    map[string]bool
	count  int
}

func (p *pages) next(token string) (bool, error) {
	p.count++
	if p.count > 10000 {
		return false, configError()
	}
	if token == "" {
		return true, nil
	}
	if p.tokens == nil {
		p.tokens = map[string]bool{}
	}
	if p.tokens[token] {
		return false, configError()
	}
	p.tokens[token] = true
	return false, nil
}
func (p *pages) unique(id string) error {
	if p.ids == nil {
		p.ids = map[string]bool{}
	}
	if id == "" || p.ids[id] {
		return configError()
	}
	p.ids[id] = true
	return nil
}

type RegionCatalog struct {
	Transport http.RoundTripper
	Timeout   time.Duration
}

func (r RegionCatalog) Regions(ctx context.Context, key *credentials.CloudKey, project string) ([]account.RegionOption, error) {
	if !projectID.MatchString(project) {
		return nil, configError()
	}
	p := New(Options{Transport: r.Transport, RequestTimeout: r.Timeout})
	ctx, cancel := context.WithTimeout(ctx, p.options.RequestTimeout)
	defer cancel()
	client, err := p.computeClient(ctx, key)
	if err != nil {
		return nil, err
	}
	rows := []account.RegionOption{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Regions.List(project).PageToken(token).MaxResults(500).Context(ctx).Do()
		if err != nil {
			return nil, requestError(ctx, err)
		}
		if page == nil {
			return nil, configError()
		}
		if page.Warning != nil {
			if err := partialWarning(page.Warning.Code, len(page.Items)); err != nil {
				return nil, err
			}
		}
		for _, item := range page.Items {
			if item == nil || !regionID.MatchString(item.Name) {
				return nil, configError()
			}
			if err := pagination.unique(item.Name); err != nil {
				return nil, err
			}
			rows = append(rows, account.RegionOption{ID: item.Name, Name: item.Name})
		}
		done, err := pagination.next(page.NextPageToken)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		token = page.NextPageToken
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// The pinned oauth2 JWT source uses Client.PostForm without attaching its
// context to the request. Bind it explicitly so cancellation reaches token I/O.
type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// Token owns this context until the JWT source finishes reading and closing
	// the response body, including non-success token responses.
	return t.base.RoundTrip(request.WithContext(t.ctx))
}

type boundedJWTSource struct {
	ctx       context.Context
	config    jwt.Config
	transport http.RoundTripper
	timeout   time.Duration
}

func (s boundedJWTSource) Token() (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()
	client := &http.Client{Transport: contextTransport{ctx: ctx, base: s.transport}, Timeout: s.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	authCtx := context.WithValue(ctx, oauth2.HTTPClient, client)
	token, err := s.config.TokenSource(authCtx).Token()
	if err != nil {
		return nil, requestError(ctx, err)
	}
	return token, nil
}

func (p *Provider) RequestTimeout() time.Duration { return p.options.RequestTimeout }

func (r RegionCatalog) RequestTimeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 30 * time.Second
}
