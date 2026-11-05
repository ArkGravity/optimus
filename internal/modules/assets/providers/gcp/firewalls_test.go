package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestFetchFirewallCompletesGlobalListBeforePublication(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.Equal(t, "/compute/v1/projects/target-project/global/firewalls", r.URL.Path)
		return 200, `{"items":[{"id":"1","name":"web","network":"global/networks/main","allowed":[{"IPProtocol":"tcp","ports":["80","443"]}]}]}`
	})
	snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), assetsync.Unit{ResourceType: "firewall", LocationType: "global", Location: "global", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}, key)
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NotNil(t, snapshot.Publish)
	require.Equal(t, 1, calls)
}

func TestFirewallsPreserveRulesAndPriorityPresence(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(_ *http.Request) (int, string) {
		calls++
		if calls == 2 {
			return 200, `{"items":[{"id":"2","name":"egress","network":"global/networks/main","priority":65535,"disabled":true,"direction":"EGRESS","destinationRanges":["::/0"],"targetServiceAccounts":["target@example.test"],"denied":[{"IPProtocol":"all"}]}]}`
		}
		return 200, `{"items":[{"id":"18446744073709551615","name":"web","network":"https://www.googleapis.com/compute/v1/projects/host/global/networks/shared","priority":0,"disabled":false,"sourceRanges":["10.0.0.7/24","2001:db8::/32"],"sourceTags":["source"],"targetTags":["target"],"allowed":[{"IPProtocol":"tcp","ports":["0","80-81"]},{"IPProtocol":"icmp"}]},{"id":"3","name":"defaults","network":"global/networks/main","sourceServiceAccounts":["source@example.test"],"targetServiceAccounts":["target@example.test"],"allowed":[{"IPProtocol":"udp"}]}],"nextPageToken":"p2"}`
	})
	p := New(Options{Transport: transport})
	client, presence, err := p.firewallClient(context.Background(), key)
	require.NoError(t, err)
	rows, err := p.firewalls(context.Background(), client, presence, "target-project")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, 2, calls)
	require.Equal(t, "18446744073709551615", rows[0].FirewallID)
	require.Equal(t, "projects/host/global/networks/shared", rows[0].NetworkRef)
	require.JSONEq(t, `{}`, string(rows[0].Tags))
	var rules []resource.Rule
	require.NoError(t, json.Unmarshal(rows[0].InboundRules, &rules))
	require.Len(t, rules, 3)
	require.EqualValues(t, 0, *rules[0].Priority)
	require.True(t, *rules[0].Enabled)
	require.EqualValues(t, 0, *rules[0].FromPort)
	require.EqualValues(t, 81, *rules[1].ToPort)
	require.Equal(t, []string{"10.0.0.0/24", "2001:db8::/32"}, rules[0].SourceRanges)
	require.Equal(t, []string{"source"}, rules[0].SourceNetworkTags)
	require.Equal(t, []string{"target"}, rules[0].TargetNetworkTags)
	require.Nil(t, rules[2].ICMPType)
	require.Nil(t, rules[2].ICMPCode)
	require.Nil(t, rules[2].FromPort)
	rules = nil
	require.NoError(t, json.Unmarshal(rows[1].InboundRules, &rules))
	require.EqualValues(t, 1000, *rules[0].Priority)
	require.True(t, *rules[0].Enabled)
	require.Nil(t, rules[0].FromPort)
	require.Equal(t, []string{"source@example.test"}, rules[0].SourceServiceAccounts)
	rules = nil
	require.NoError(t, json.Unmarshal(rows[2].OutboundRules, &rules))
	require.Len(t, rules, 1)
	require.Equal(t, "deny", rules[0].Action)
	require.Equal(t, "all", rules[0].Protocol)
	require.EqualValues(t, 65535, *rules[0].Priority)
	require.False(t, *rules[0].Enabled)
	require.Equal(t, []string{"::/0"}, rules[0].DestinationRanges)
}

func familyUnit(kind string) assetsync.Unit {
	unit := assetsync.Unit{ResourceType: kind, LocationType: "global", Location: "global", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}
	if kind == "database" {
		unit.LocationType = "region"
		unit.Location = "us-central1"
		unit.Region = "us-central1"
	}
	return unit
}

func TestDatabaseAndFirewallErrorsStayNeutralAndDoNotPublish(t *testing.T) {
	for _, kind := range []string{"database", "firewall"} {
		for _, failure := range []struct {
			name, body string
			code       apperr.Code
			token      bool
		}{
			{name: "permission", body: `{"error":{"code":403,"message":"secret-api-body"}}`, code: errs.CodeAssetsForbidden},
			{name: "disabled", body: `{"error":{"code":403,"message":"secret-api-body","errors":[{"reason":"accessNotConfigured"}]}}`, code: errs.CodeAssetsAPIDisabled},
			{name: "token", body: `{"error":"invalid_grant","error_description":"secret-JWT-private-key"}`, code: errs.CodeAssetsUnauthorized, token: true},
		} {
			t.Run(kind+"/"+failure.name, func(t *testing.T) {
				key, private := fixtureKey(t)
				apiCalls := 0
				normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { apiCalls++; return 403, failure.body })
				transport := transportFunc(func(r *http.Request) (*http.Response, error) {
					if failure.token && r.URL.String() == tokenURL {
						return response(r, 400, failure.body), nil
					}
					return normal(r)
				})
				snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), familyUnit(kind), key)
				require.Error(t, err)
				biz, ok := apperr.AsBiz(err)
				require.True(t, ok)
				require.Equal(t, failure.code, biz.Code)
				require.False(t, snapshot.Complete)
				require.Nil(t, snapshot.Publish)
				require.NotContains(t, err.Error(), "secret")
				require.NotContains(t, err.Error(), "JWT")
				if failure.token {
					require.Zero(t, apiCalls)
				} else {
					require.Equal(t, 1, apiCalls)
				}
			})
		}
	}
}

func TestDatabaseAndFirewallBodyReadsPreserveCancellation(t *testing.T) {
	for _, kind := range []string{"database", "firewall"} {
		for _, cancelNow := range []bool{false, true} {
			t.Run(kind+"/"+fmtBool(cancelNow), func(t *testing.T) {
				key, private := fixtureKey(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				normal, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, `{}` })
				transport := transportFunc(func(r *http.Request) (*http.Response, error) {
					resp, err := normal(r)
					if r.URL.String() != tokenURL {
						resp.Body = &contextBody{ctx: r.Context(), chunks: []string{`{"items":[]}`}, stall: true}
						if cancelNow {
							cancel()
						}
					}
					return resp, err
				})
				snapshot, err := New(Options{Transport: transport, RequestTimeout: 30 * time.Millisecond}).Fetch(ctx, familyUnit(kind), key)
				if cancelNow {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
				require.False(t, snapshot.Complete)
				require.Nil(t, snapshot.Publish)
			})
		}
	}
}

func TestFirewallObserverRejectsBrokenBodiesAndClosesOriginal(t *testing.T) {
	marker := errors.New("read failed")
	for _, tt := range []struct {
		name, body string
		readErr    error
	}{
		{name: "oversize", body: strings.Repeat(" ", maxFirewallPageBytes+1)},
		{name: "malformed", body: `{"items":`},
		{name: "read-error", readErr: marker},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := &observedBody{Reader: strings.NewReader(tt.body), err: tt.readErr}
			presence := &firewallPriorities{}
			transport := firewallTransport{base: transportFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil }), priorities: presence}
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, computeEndpoint+"projects/target-project/global/firewalls", nil)
			require.NoError(t, err)
			_, err = transport.RoundTrip(request)
			require.Error(t, err)
			require.True(t, body.closed)
			require.Nil(t, presence.values)
			if tt.readErr != nil {
				require.ErrorIs(t, err, marker)
			}
		})
	}
}

func TestFirewallObserverRestoresIdenticalPageAndClosesSource(t *testing.T) {
	raw := `{"items":[{"id":"1","priority":0,"allowed":[{"IPProtocol":"tcp"}]},{"id":"2"}]}`
	body := &observedBody{Reader: strings.NewReader(raw)}
	presence := &firewallPriorities{}
	transport := firewallTransport{base: transportFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil }), priorities: presence}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, computeEndpoint+"projects/target-project/global/firewalls", nil)
	require.NoError(t, err)
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	defer response.Body.Close()
	restored, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, raw, string(restored))
	require.True(t, body.closed)
	require.Equal(t, map[string]int32{"1": 0, "2": 1000}, presence.values)
}

type observedBody struct {
	io.Reader
	err    error
	closed bool
}

func (b *observedBody) Read(raw []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	return b.Reader.Read(raw)
}
func (b *observedBody) Close() error { b.closed = true; return nil }

func TestFirewallPathRejectsAmbientCertificateDiscoveryBeforeNetwork(t *testing.T) {
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
	key, _ := fixtureKey(t)
	p := New(Options{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("must reject before network"); return nil, nil })})
	_, _, err := p.firewallClient(context.Background(), key)
	require.Error(t, err)
}

func TestFirewallsRejectIncompleteOrContradictorySnapshots(t *testing.T) {
	for _, body := range []string{
		`{"warning":{"code":"UNREACHABLE"}}`, `{"nextPageToken":"again"}`, `{"items":[null]}`,
		`{"items":[{"id":"1","name":"Invalid_Name","network":"global/networks/main","allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"duplicate","network":"global/networks/main","allowed":[{"IPProtocol":"tcp"}]},{"id":"2","name":"duplicate","network":"global/networks/main","allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","allowed":[{"IPProtocol":"icmp","ports":["0"]}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","allowed":[{"IPProtocol":"tcp","ports":[""]}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","priority":65536,"allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","priority":null,"allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","allowed":[{"IPProtocol":"tcp","ports":["81-80"]}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","allowed":[{"IPProtocol":"tcp"}],"denied":[{"IPProtocol":"udp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","direction":"EGRESS","sourceTags":["bad"],"allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","targetTags":["bad"],"targetServiceAccounts":["bad@example.test"],"allowed":[{"IPProtocol":"tcp"}]}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main"}]}`,
		`{"items":[{"id":"1","name":"bad","network":"global/networks/main","selfLink":"https://compute.googleapis.com/compute/v1/projects/foreign/global/firewalls/bad","allowed":[{"IPProtocol":"tcp"}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			key, private := fixtureKey(t)
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, body })
			snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), assetsync.Unit{ResourceType: "firewall", LocationType: "global", Location: "global", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}, key)
			require.Error(t, err)
			require.False(t, snapshot.Complete)
			require.Nil(t, snapshot.Publish)
		})
	}
}
