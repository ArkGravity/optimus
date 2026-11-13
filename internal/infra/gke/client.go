package gke

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"k8s.io/client-go/rest"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

const tokenURL = "https://oauth2.googleapis.com/token"
const metadataURL = "https://container.googleapis.com/v1/"
const requestTimeout = 15 * time.Second

type Options struct {
	// Transport is supplied only by trusted process composition or tests.
	Transport http.RoundTripper
}

func configError() error {
	return apperr.New(apperr.CodeValidation, "k8s.gke.invalid_config", "invalid GKE connection configuration")
}

func upstreamError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return apperr.New(apperr.CodeAPIServerUnauthorized, "k8s.gke.unauthorized", "GKE authentication failed")
	case http.StatusForbidden:
		return apperr.New(apperr.CodeAPIServerForbidden, "k8s.gke.forbidden", "GKE access denied; check the selected service account permissions")
	case http.StatusNotFound:
		return apperr.New(apperr.CodeNotFound, "k8s.gke.not_found", "GKE cluster not found")
	default:
		return apperr.New(apperr.CodeClusterUnreachable, "k8s.gke.unreachable", "GKE connection failed")
	}
}

// RESTConfig resolves trusted connection metadata and creates an automatically
// refreshing, operation-scoped token source. canonicalKey must have passed the
// credentials module's service-account validation. No ADC or plugins are used.
func RESTConfig(ctx context.Context, target Config, canonicalKey []byte, options Options) (*rest.Config, error) {
	if !target.Valid() {
		return nil, configError()
	}
	var fields struct {
		Email      string `json:"client_email"`
		PrivateKey string `json:"private_key"`
		KeyID      string `json:"private_key_id"`
	}
	if json.Unmarshal(canonicalKey, &fields) != nil || fields.Email == "" || fields.PrivateKey == "" {
		return nil, configError()
	}
	transport := options.Transport
	if transport == nil {
		owned := &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: requestTimeout, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: requestTimeout, ResponseHeaderTimeout: requestTimeout,
			IdleConnTimeout: 30 * time.Second,
		}
		transport = owned
		defer owned.CloseIdleConnections()
	}
	config := jwt.Config{Email: fields.Email, PrivateKey: []byte(fields.PrivateKey), PrivateKeyID: fields.KeyID,
		TokenURL: tokenURL, Scopes: []string{"https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/userinfo.email"}}
	source := oauth2.ReuseTokenSource(nil, tokenSource{ctx: ctx, config: config, transport: transport})
	metadataCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(metadataCtx, http.MethodGet, metadataURL+"projects/"+target.Project+"/locations/"+target.Location+"/clusters/"+target.Cluster, nil)
	if err != nil {
		return nil, configError()
	}
	client := &http.Client{Timeout: requestTimeout, CheckRedirect: noRedirect,
		Transport: authorizedTransport{base: transport, source: source, authority: "container.googleapis.com"}}
	response, err := client.Do(request)
	if err != nil {
		if be, ok := apperr.AsBiz(err); ok {
			return nil, be
		}
		return nil, upstreamError(0)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, upstreamError(response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return nil, upstreamError(0)
	}
	var cluster clusterMetadata
	if json.Unmarshal(data, &cluster) != nil {
		return nil, configError()
	}
	host, ca, err := cluster.connection(target.Endpoint)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, configError()
	}
	return &rest.Config{
		Host: host, TLSClientConfig: rest.TLSClientConfig{CAData: ca},
		WrapTransport: func(base http.RoundTripper) http.RoundTripper {
			return authorizedTransport{base: base, source: source, authority: u.Host}
		},
	}, nil
}

type clusterMetadata struct {
	Endpoint   string `json:"endpoint"`
	MasterAuth struct {
		ClusterCA string `json:"clusterCaCertificate"`
	} `json:"masterAuth"`
	Private struct {
		Private string `json:"privateEndpoint"`
		Public  string `json:"publicEndpoint"`
	} `json:"privateClusterConfig"`
	ControlPlane struct {
		IP struct {
			Private string `json:"privateEndpoint"`
			Public  string `json:"publicEndpoint"`
		} `json:"ipEndpointsConfig"`
		DNS struct {
			Endpoint string `json:"endpoint"`
		} `json:"dnsEndpointConfig"`
	} `json:"controlPlaneEndpointsConfig"`
}

func (c clusterMetadata) connection(kind string) (string, []byte, error) {
	endpoint := c.Endpoint
	switch kind {
	case "public":
		endpoint = c.ControlPlane.IP.Public
		if endpoint == "" {
			endpoint = c.Private.Public
		}
	case "private":
		endpoint = c.ControlPlane.IP.Private
		if endpoint == "" {
			endpoint = c.Private.Private
		}
	case "dns":
		endpoint = c.ControlPlane.DNS.Endpoint
	}
	if kind == "dns" {
		if !strings.HasSuffix(endpoint, ".gke.goog") || strings.ContainsAny(endpoint, "/:@?#\\ \t\r\n") {
			return "", nil, configError()
		}
		return "https://" + endpoint, nil, nil
	}
	if net.ParseIP(endpoint) == nil {
		return "", nil, configError()
	}
	ca, err := base64.StdEncoding.DecodeString(c.MasterAuth.ClusterCA)
	if err != nil || len(ca) == 0 || !x509.NewCertPool().AppendCertsFromPEM(ca) {
		return "", nil, configError()
	}
	if strings.Contains(endpoint, ":") {
		endpoint = "[" + endpoint + "]"
	}
	return "https://" + endpoint, ca, nil
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// Every actual request checks the destination before attaching the token,
// including redirected requests initiated by a Kubernetes HTTP client.
type authorizedTransport struct {
	base      http.RoundTripper
	source    oauth2.TokenSource
	authority string
}

func (t authorizedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || request.URL.Host != t.authority || request.URL.User != nil {
		return nil, configError()
	}
	if err := request.Context().Err(); err != nil {
		return nil, upstreamError(0)
	}
	token, err := t.source.Token()
	if err != nil {
		return nil, err
	}
	if token == nil || token.AccessToken == "" {
		return nil, upstreamError(http.StatusUnauthorized)
	}
	authorized := request.Clone(request.Context())
	token.SetAuthHeader(authorized)
	return t.base.RoundTrip(authorized)
}

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(request.WithContext(t.ctx))
}

type tokenSource struct {
	ctx       context.Context
	config    jwt.Config
	transport http.RoundTripper
}

func (s tokenSource) Token() (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(s.ctx, requestTimeout)
	defer cancel()
	client := &http.Client{Timeout: requestTimeout, CheckRedirect: noRedirect, Transport: contextTransport{ctx: ctx, base: s.transport}}
	token, err := s.config.TokenSource(context.WithValue(ctx, oauth2.HTTPClient, client)).Token()
	if err != nil {
		var response *oauth2.RetrieveError
		if errors.As(err, &response) && response.Response != nil {
			status := response.Response.StatusCode
			if status == http.StatusBadRequest {
				status = http.StatusUnauthorized
			}
			return nil, upstreamError(status)
		}
		return nil, upstreamError(0)
	}
	return token, nil
}
