package gcp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/stretchr/testify/require"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func fixtureKey(t *testing.T) (*credentials.CloudKey, *rsa.PrivateKey) {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]string{"type": "service_account", "project_id": "home-project", "client_email": "fixture@home-project.iam.gserviceaccount.com", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": "https://oauth2.googleapis.com/token"})
	require.NoError(t, err)
	return &credentials.CloudKey{Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSON: raw}, private
}
func fixtureTransport(t *testing.T, private *rsa.PrivateKey, api func(*http.Request) (int, string)) (transportFunc, *int) {
	t.Helper()
	tokens := 0
	return func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == "https://oauth2.googleapis.com/token" {
			tokens++
			require.Equal(t, "POST", r.Method)
			require.NoError(t, r.ParseForm())
			require.Equal(t, "urn:ietf:params:oauth:grant-type:jwt-bearer", r.Form.Get("grant_type"))
			parts := strings.Split(r.Form.Get("assertion"), ".")
			require.Len(t, parts, 3)
			signature, err := base64.RawURLEncoding.DecodeString(parts[2])
			require.NoError(t, err)
			hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			require.NoError(t, rsa.VerifyPKCS1v15(&private.PublicKey, crypto.SHA256, hash[:], signature))
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			require.NoError(t, err)
			var claims map[string]any
			require.NoError(t, json.Unmarshal(payload, &claims))
			require.Equal(t, "fixture@home-project.iam.gserviceaccount.com", claims["iss"])
			require.Equal(t, "https://oauth2.googleapis.com/token", claims["aud"])
			require.Equal(t, "https://www.googleapis.com/auth/cloud-platform", claims["scope"])
			return response(r, 200, `{"access_token":"fixture-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		require.Contains(t, r.URL.Path, "/projects/target-project/")
		require.NotContains(t, r.URL.Path, "home-project")
		require.True(t, r.URL.Host == "compute.googleapis.com" || r.URL.Host == "www.googleapis.com" || r.URL.Host == "sqladmin.googleapis.com", r.URL.String())
		status, body := api(r)
		return response(r, status, body), nil
	}, &tokens
}
func TestRegionCatalogUsesExplicitJWTAndTargetProject(t *testing.T) {
	key, private := fixtureKey(t)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/must-not-read/ambient.json")
	t.Setenv("GCE_METADATA_HOST", "metadata.invalid")
	calls := 0
	transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.True(t, strings.HasSuffix(r.URL.Path, "/regions"))
		if calls == 1 {
			return 200, `{"items":[{"name":"us-central1"}],"nextPageToken":"page2"}`
		}
		require.Equal(t, "page2", r.URL.Query().Get("pageToken"))
		return 200, `{"items":[{"name":"europe-west1"}]}`
	})
	items, err := (RegionCatalog{Transport: transport, Timeout: time.Second}).Regions(context.Background(), key, "target-project")
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "europe-west1", items[0].ID)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, *tokens)
}
func TestSQLClientUsesExplicitJWTAndTargetProject(t *testing.T) {
	key, private := fixtureKey(t)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/must-not-read/ambient.json")
	t.Setenv("GCE_METADATA_HOST", "metadata.invalid")
	calls := 0
	transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.Equal(t, "https", r.URL.Scheme)
		require.Equal(t, "sqladmin.googleapis.com", r.URL.Host)
		require.Equal(t, "/v1/projects/target-project/instances", r.URL.Path)
		return 200, `{"kind":"sql#instancesList","items":[{"name":"fixture-db","project":"target-project","region":"us-central1"}]}`
	})
	ctx := context.Background()
	client, err := New(Options{Transport: transport, RequestTimeout: time.Second}).sqlClient(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "https://sqladmin.googleapis.com/", client.BasePath)
	items, err := client.Instances.List("target-project").Context(ctx).Do()
	require.NoError(t, err)
	require.Len(t, items.Items, 1)
	require.Equal(t, "fixture-db", items.Items[0].Name)
	require.Equal(t, "target-project", items.Items[0].Project)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, *tokens)
}

type clientFixture struct {
	name, host, path string
	request          func(context.Context, *Provider, *credentials.CloudKey) error
}

func clientFixtures() []clientFixture {
	return []clientFixture{
		{name: "compute", host: "compute.googleapis.com", path: "/compute/v1/projects/target-project/regions", request: func(ctx context.Context, p *Provider, key *credentials.CloudKey) error {
			client, err := p.computeClient(ctx, key)
			if err != nil {
				return err
			}
			_, err = client.Regions.List("target-project").Context(ctx).Do()
			return err
		}},
		{name: "sql", host: "sqladmin.googleapis.com", path: "/v1/projects/target-project/instances", request: func(ctx context.Context, p *Provider, key *credentials.CloudKey) error {
			client, err := p.sqlClient(ctx, key)
			if err != nil {
				return err
			}
			_, err = client.Instances.List("target-project").Context(ctx).Do()
			return err
		}},
		{name: "catalog", host: "compute.googleapis.com", path: "/compute/v1/projects/target-project/regions", request: func(ctx context.Context, p *Provider, key *credentials.CloudKey) error {
			_, err := (RegionCatalog{Transport: p.options.Transport, Timeout: p.options.RequestTimeout}).Regions(ctx, key, "target-project")
			return err
		}},
	}
}

func TestClientsPinOfficialEndpointsDespiteAmbientOverrides(t *testing.T) {
	key, private := fixtureKey(t)
	for _, env := range []struct{ name, universe, mtls, legacyMTLS, certificate string }{
		{name: "universe", universe: "attacker.invalid", mtls: "never", certificate: "false"},
		{name: "universe-and-mtls", universe: "attacker.invalid", mtls: "always", certificate: "FaLsE"},
		{name: "mtls", mtls: "always"},
		{name: "legacy-mtls", legacyMTLS: "always"},
	} {
		t.Run(env.name, func(t *testing.T) {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", env.certificate)
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", env.universe)
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", env.mtls)
			t.Setenv("GOOGLE_API_USE_MTLS", env.legacyMTLS)
			for _, fixture := range clientFixtures() {
				t.Run(fixture.name, func(t *testing.T) {
					calls := 0
					transport, tokens := fixtureTransport(t, private, func(r *http.Request) (int, string) {
						calls++
						require.Equal(t, "https", r.URL.Scheme)
						require.Equal(t, fixture.host, r.URL.Host)
						require.Equal(t, fixture.path, r.URL.Path)
						return 200, `{}`
					})
					require.NoError(t, fixture.request(context.Background(), New(Options{Transport: transport, RequestTimeout: time.Second}), key))
					require.Equal(t, 1, calls)
					require.Equal(t, 1, *tokens)
				})
			}
		})
	}
}

func TestClientsRejectAmbientCertificateDiscoveryBeforeSignerLookup(t *testing.T) {
	key, _ := fixtureKey(t)
	dir := t.TempDir()
	signer := filepath.Join(dir, "signer")
	config := filepath.Join(dir, "certificate.json")
	// A vulnerable constructor can only create this marker and immediately exit;
	// the fixture never signs or waits for RPC input.
	require.NoError(t, os.WriteFile(signer, []byte("#!/bin/sh\n: > \"$1.executed\"\nexit 0\n"), 0o700))
	raw, err := json.Marshal(map[string]any{"libs": map[string]string{"ecp": signer}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(config, raw, 0o600))
	broken := filepath.Join(dir, "broken.json")
	require.NoError(t, os.WriteFile(broken, []byte("{"), 0o600))
	for _, setting := range []string{"true", "TrUe", "unsupported"} {
		t.Run(setting, func(t *testing.T) {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", setting)
			for _, configPath := range []string{config, broken} {
				t.Run(filepath.Base(configPath), func(t *testing.T) {
					t.Setenv("GOOGLE_API_CERTIFICATE_CONFIG", configPath)
					for _, fixture := range clientFixtures() {
						t.Run(fixture.name, func(t *testing.T) {
							calls := 0
							transport := transportFunc(func(*http.Request) (*http.Response, error) {
								calls++
								return nil, errors.New("unexpected HTTP request")
							})
							start := time.Now()
							err := fixture.request(context.Background(), New(Options{Transport: transport, RequestTimeout: time.Second}), key)
							require.Less(t, time.Since(start), time.Second)
							require.Zero(t, calls)
							require.NoFileExists(t, config+".executed")
							biz, ok := apperr.AsBiz(err)
							require.True(t, ok)
							require.Equal(t, errs.CodeAssetsConfig, biz.Code)
							require.NotContains(t, err.Error(), configPath)
						})
					}
				})
			}
		})
	}
}

func TestClientRejectsUnsafeAuthenticationBeforeNetwork(t *testing.T) {
	key, _ := fixtureKey(t)
	for _, mutate := range []func(*credentials.CloudKey){func(k *credentials.CloudKey) { k.Provider = "aws" }, func(k *credentials.CloudKey) { k.AuthType = "access_key" }, func(k *credentials.CloudKey) { k.AccessKeyID = "mixed" }, func(k *credentials.CloudKey) { k.SecretAccessKey = "mixed" }, func(k *credentials.CloudKey) {
		k.ServiceAccountJSON = []byte(`{"type":"external_account","credential_source":{"url":"http://metadata"}}`)
	}, func(k *credentials.CloudKey) {
		k.ServiceAccountJSON = []byte(strings.ReplaceAll(string(k.ServiceAccountJSON), "https://oauth2.googleapis.com/token", "http://attacker.invalid/token"))
	}} {
		copyKey := *key
		mutate(&copyKey)
		_, err := (RegionCatalog{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unsafe auth reached network"); return nil, nil })}).Regions(context.Background(), &copyKey, "target-project")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE KEY")
	}
}

func TestOAuthAndAPIRequestsHonorCancellationAndDeadlines(t *testing.T) {
	for _, stage := range []string{"oauth", "api"} {
		for _, deadline := range []bool{false, true} {
			t.Run(stage+fmtBool(deadline), func(t *testing.T) {
				key, private := fixtureKey(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if deadline {
					var end context.CancelFunc
					ctx, end = context.WithTimeout(ctx, 20*time.Millisecond)
					defer end()
				}
				normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, `{}` })
				transport := transportFunc(func(r *http.Request) (*http.Response, error) {
					isToken := r.URL.String() == "https://oauth2.googleapis.com/token"
					if (stage == "oauth" && isToken) || (stage == "api" && !isToken) {
						if !deadline {
							cancel()
						}
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					return normal(r)
				})
				start := time.Now()
				_, err := (RegionCatalog{Transport: transport, Timeout: time.Second}).Regions(ctx, key, "target-project")
				require.Error(t, err)
				require.Less(t, time.Since(start), time.Second)
				if deadline {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				} else {
					require.ErrorIs(t, err, context.Canceled)
				}
			})
		}
	}
}
func fmtBool(value bool) string {
	if value {
		return "/deadline"
	}
	return "/cancel"
}
func TestOAuthStallHasIndependentRequestDeadline(t *testing.T) {
	key, _ := fixtureKey(t)
	transport := transportFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	_, err := (RegionCatalog{Transport: transport, Timeout: 20 * time.Millisecond}).Regions(context.Background(), key, "target-project")
	require.True(t, errors.Is(err, context.DeadlineExceeded))
}
func TestTokenRefreshStaysInMemoryWithinExplicitClient(t *testing.T) {
	key, private := fixtureKey(t)
	tokens := 0
	apiCalls := 0
	normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { apiCalls++; return 200, `{}` })
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := normal(r)
		if r.URL.String() == "https://oauth2.googleapis.com/token" {
			tokens++
			if tokens == 1 {
				resp.Body = io.NopCloser(strings.NewReader(`{"access_token":"fixture-token","token_type":"Bearer","expires_in":1}`))
			}
		}
		return resp, err
	})
	client, err := New(Options{Transport: transport}).computeClient(context.Background(), key)
	require.NoError(t, err)
	_, err = client.Regions.List("target-project").Context(context.Background()).Do()
	require.NoError(t, err)
	_, err = client.Regions.List("target-project").Context(context.Background()).Do()
	require.NoError(t, err)
	_, err = client.Regions.List("target-project").Context(context.Background()).Do()
	require.NoError(t, err)
	require.Equal(t, 2, tokens)
	require.Equal(t, 3, apiCalls)
}

type contextBody struct {
	ctx    context.Context
	chunks []string
	stall  bool
}

func (b *contextBody) Read(out []byte) (int, error) {
	if b.ctx.Err() != nil {
		return 0, b.ctx.Err()
	}
	if len(b.chunks) > 0 {
		chunk := b.chunks[0]
		n := copy(out, chunk)
		if n == len(chunk) {
			b.chunks = b.chunks[1:]
		} else {
			b.chunks[0] = chunk[n:]
		}
		return n, nil
	}
	if b.stall {
		<-b.ctx.Done()
		return 0, b.ctx.Err()
	}
	return 0, io.EOF
}
func (*contextBody) Close() error { return nil }
func TestTokenContextLivesThroughBodyReadAndBoundsSlowBody(t *testing.T) {
	for _, stall := range []bool{false, true} {
		t.Run(fmtBool(stall), func(t *testing.T) {
			key, private := fixtureKey(t)
			normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, `{}` })
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				resp, err := normal(r)
				if r.URL.String() == "https://oauth2.googleapis.com/token" {
					resp.Body = &contextBody{ctx: r.Context(), chunks: []string{`{"access_token":"fixture-token",`, `"token_type":"Bearer","expires_in":3600}`}, stall: stall}
				}
				return resp, err
			})
			_, err := (RegionCatalog{Transport: transport, Timeout: 30 * time.Millisecond}).Regions(context.Background(), key, "target-project")
			if stall {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSDKDistinguishesOAuthUnauthorizedFromAPIForbidden(t *testing.T) {
	for _, oauthFailure := range []bool{true, false} {
		t.Run(fmtBool(oauthFailure), func(t *testing.T) {
			key, private := fixtureKey(t)
			apiCalls := 0
			normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
				apiCalls++
				return 403, `{"error":{"code":403,"message":"secret-api-body"}}`
			})
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				if oauthFailure && r.URL.String() == "https://oauth2.googleapis.com/token" {
					return response(r, 400, `{"error":"invalid_grant","error_description":"private-key JWT secret"}`), nil
				}
				return normal(r)
			})
			_, err := (RegionCatalog{Transport: transport}).Regions(context.Background(), key, "target-project")
			require.Error(t, err)
			biz, ok := apperr.AsBiz(err)
			require.True(t, ok)
			if oauthFailure {
				require.Equal(t, errs.CodeAssetsUnauthorized, biz.Code)
				require.Zero(t, apiCalls)
			} else {
				require.Equal(t, errs.CodeAssetsForbidden, biz.Code)
				require.Equal(t, 1, apiCalls)
			}
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "JWT")
		})
	}
}
