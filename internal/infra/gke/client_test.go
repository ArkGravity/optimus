package gke

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reply(code int, data string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(data))}
}

func fixture(t *testing.T) ([]byte, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture CA"}, IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	ca := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	data, err := json.Marshal(map[string]string{"client_email": "fixture@fixture-project.iam.gserviceaccount.com", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))})
	require.NoError(t, err)
	return data, ca
}

func target() Config {
	return Config{Project: "fixture-project", Location: "us-central1", Cluster: "fixture", Endpoint: "default"}
}

func TestNativeGKEResolvesMetadataAndRefreshesTokens(t *testing.T) {
	key, ca := fixture(t)
	var tokens atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "oauth2.googleapis.com":
			require.Equal(t, tokenURL, r.URL.String())
			require.NoError(t, r.ParseForm())
			parts := strings.Split(r.Form.Get("assertion"), ".")
			require.Len(t, parts, 3)
			claims, err := base64.RawURLEncoding.DecodeString(parts[1])
			require.NoError(t, err)
			require.Contains(t, string(claims), "https://www.googleapis.com/auth/userinfo.email")
			require.Contains(t, string(claims), "https://www.googleapis.com/auth/cloud-platform")
			tokens.Add(1)
			// Short lifetime forces renewal without any sleeps or wall-clock races.
			return reply(200, `{"access_token":"fixture-token","token_type":"Bearer","expires_in":1}`), nil
		case "container.googleapis.com":
			require.Equal(t, "/v1/projects/fixture-project/locations/us-central1/clusters/fixture", r.URL.Path)
			require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
			return reply(200, `{"endpoint":"203.0.113.10","masterAuth":{"clusterCaCertificate":"`+ca+`"}}`), nil
		default:
			t.Fatalf("unexpected request to %s", r.URL.Host)
			return nil, nil
		}
	})
	cfg, err := RESTConfig(t.Context(), target(), key, Options{Transport: transport})
	require.NoError(t, err)
	require.Equal(t, "https://203.0.113.10", cfg.Host)
	require.NotEmpty(t, cfg.CAData)
	require.False(t, cfg.Insecure)
	require.Empty(t, cfg.BearerToken)
	require.Nil(t, cfg.ExecProvider)
	called := false
	api := cfg.WrapTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		return reply(200, `{}`), nil
	}))
	r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, cfg.Host+"/api/v1/pods", nil)
	require.NoError(t, err)
	response, err := api.RoundTrip(r)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.True(t, called)
	require.EqualValues(t, 2, tokens.Load())
	for _, host := range []string{"http://203.0.113.10", "https://attacker.invalid", "https://203.0.113.10:444"} {
		r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host, nil)
		require.NoError(t, err)
		_, err = api.RoundTrip(r)
		require.Error(t, err)
	}
	require.EqualValues(t, 2, tokens.Load())
}

func TestGKEEndpointSelectionAndTLS(t *testing.T) {
	_, ca := fixture(t)
	var metadata clusterMetadata
	require.NoError(t, json.Unmarshal([]byte(`{"endpoint":"203.0.113.1","masterAuth":{"clusterCaCertificate":"`+ca+`"},"privateClusterConfig":{"publicEndpoint":"203.0.113.2","privateEndpoint":"10.0.0.1"},"controlPlaneEndpointsConfig":{"ipEndpointsConfig":{"publicEndpoint":"203.0.113.3","privateEndpoint":"10.0.0.2"},"dnsEndpointConfig":{"endpoint":"fixture.gke.goog"}}}`), &metadata))
	for kind, want := range map[string]string{"default": "https://203.0.113.1", "public": "https://203.0.113.3", "private": "https://10.0.0.2", "dns": "https://fixture.gke.goog"} {
		host, data, err := metadata.connection(kind)
		require.NoError(t, err)
		require.Equal(t, want, host)
		if kind == "dns" {
			require.Empty(t, data)
		} else {
			require.NotEmpty(t, data)
		}
	}
	metadata.Endpoint = "attacker.invalid"
	_, _, err := metadata.connection("default")
	require.Error(t, err)
	metadata.ControlPlane.DNS.Endpoint = "fixture.gke.goog/attacker"
	_, _, err = metadata.connection("dns")
	require.Error(t, err)
	metadata.Endpoint = "203.0.113.1"
	metadata.MasterAuth.ClusterCA = base64.StdEncoding.EncodeToString([]byte("not a certificate"))
	_, _, err = metadata.connection("default")
	require.Error(t, err)
}

func TestGKERejectsInvalidTargetsBeforeNetworkIO(t *testing.T) {
	for _, mutate := range []func(*Config){func(c *Config) { c.Project = "../other" }, func(c *Config) { c.Location = "us-central1?override" }, func(c *Config) { c.Cluster = "name/other" }, func(c *Config) { c.Endpoint = "https://attacker.invalid" }} {
		c := target()
		mutate(&c)
		_, err := RESTConfig(t.Context(), c, []byte(`{}`), Options{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network request"); return nil, nil })})
		require.Error(t, err)
	}
}

func TestGKEDoesNotFollowRedirectsOrExposeProviderBodies(t *testing.T) {
	key, _ := fixture(t)
	for _, stage := range []string{"token", "metadata"} {
		t.Run(stage, func(t *testing.T) {
			_, err := RESTConfig(t.Context(), target(), key, Options{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "oauth2.googleapis.com" && stage != "token" {
					return reply(200, `{"access_token":"fixture-token","expires_in":3600}`), nil
				}
				require.Contains(t, []string{"oauth2.googleapis.com", "container.googleapis.com"}, r.URL.Host)
				response := reply(302, `sensitive-provider-error private-key fixture-token`)
				response.Header.Set("Location", "https://attacker.invalid")
				return response, nil
			})})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "sensitive-provider-error")
			require.NotContains(t, err.Error(), "fixture-token")
		})
	}
}

func TestGKETokenCancellationReachesHTTPTransport(t *testing.T) {
	key, _ := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := RESTConfig(ctx, target(), key, Options{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "oauth2.googleapis.com", r.URL.Host)
		cancel()
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(time.Second):
			t.Fatal("token context did not cancel")
			return nil, nil
		}
	})})
	require.Error(t, err)
}

func TestGKEAuthorizationAndTokenFailureMessagesAreSafe(t *testing.T) {
	key, _ := fixture(t)
	_, err := RESTConfig(t.Context(), target(), key, Options{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "oauth2.googleapis.com" {
			return reply(200, `{"access_token":"fixture-token","expires_in":3600}`), nil
		}
		return reply(403, `{"error":{"message":"private-provider-payload"}}`), nil
	})})
	require.Error(t, err)
	require.Contains(t, err.Error(), "access denied")
	require.NotContains(t, err.Error(), "private-provider-payload")
	// Empty token responses never cause an unauthenticated API request.
	transport := authorizedTransport{source: oauth2.StaticTokenSource(&oauth2.Token{}), authority: "fixture", base: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("empty token reached transport"); return nil, nil })}
	r, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://fixture", nil)
	_, err = transport.RoundTrip(r)
	require.Error(t, err)
}

func TestGKEPreservesOAuthAuthenticationFailureCategory(t *testing.T) {
	key, _ := fixture(t)
	_, err := RESTConfig(t.Context(), target(), key, Options{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "oauth2.googleapis.com", r.URL.Host)
		return reply(400, `{"error":"invalid_grant","error_description":"sensitive-provider-payload"}`), nil
	})})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAPIServerUnauthorized, be.Code)
	require.NotContains(t, err.Error(), "sensitive-provider-payload")
}
