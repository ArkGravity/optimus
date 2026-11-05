package gcp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestFetchDatabaseCompletesProjectListBeforePublication(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		require.Equal(t, "/v1/projects/target-project/instances", r.URL.Path)
		require.Empty(t, r.URL.Query().Get("filter"))
		if calls == 1 {
			return 200, `{"items":[{"name":"elsewhere","project":"target-project","region":"europe-west1"}],"nextPageToken":"p2"}`
		}
		require.Equal(t, "p2", r.URL.Query().Get("pageToken"))
		return 200, `{"items":[{"name":"db","project":"target-project","region":"us-central1"}]}`
	})
	snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), assetsync.Unit{ResourceType: "database", LocationType: "region", Location: "us-central1", Region: "us-central1", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}, key)
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	require.NotNil(t, snapshot.Publish)
	require.Equal(t, 2, calls)
}

func TestDatabasesPreserveObservedFactsWithoutGuessingEndpoints(t *testing.T) {
	key, private := fixtureKey(t)
	transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
		return 200, `{"items":[{"name":"db","project":"target-project","region":"us-central1","gceZone":"us-central1-a","databaseVersion":"POSTGRES_16","databaseInstalledVersion":"POSTGRES_16_3","state":"SUSPENDED","connectionName":"target-project:us-central1:db","dnsName":"db.example.test.","dnsNames":[{"name":"psc.example.test.","connectionType":"PRIVATE_SERVICE_CONNECT","dnsScope":"INSTANCE","recordManager":"CUSTOMER"}],"pscServiceAttachmentLink":"projects/target-project/regions/us-central1/serviceAttachments/sql","sqlNetworkArchitecture":"NEW_NETWORK_ARCHITECTURE","ipAddresses":[{"type":"PRIVATE","ipAddress":"10.0.0.4"},{"type":"PRIMARY","ipAddress":"203.0.113.4"},{"type":"OUTGOING","ipAddress":"203.0.113.9"}],"rootPassword":"secret-root","onPremisesConfiguration":{"password":"secret-onprem"},"settings":{"tier":"db-custom-4-15360","dataDiskSizeGb":"1234","dataDiskType":"PD_SSD","availabilityType":"REGIONAL","userLabels":{"owner":"db"},"databaseFlags":[{"name":"secret-flag","value":"secret"}]}},{"name":"unknown","project":"target-project","region":"us-central1"},{"name":"elsewhere","project":"target-project","region":"europe-west1"}]}`
	})
	p := New(Options{Transport: transport})
	client, err := p.sqlClient(context.Background(), key)
	require.NoError(t, err)
	rows, err := p.databases(context.Background(), client, "target-project", "us-central1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	row := rows[0]
	require.Equal(t, "db", row.DatabaseID)
	require.Equal(t, "cloud_sql", row.Product)
	require.Equal(t, "POSTGRES", row.Engine)
	require.Equal(t, "POSTGRES_16", row.EngineVersion)
	require.Equal(t, "SUSPENDED", row.NativeStatus)
	require.EqualValues(t, 1234, *row.StorageGB)
	require.Equal(t, "REGIONAL", *row.HAMode)
	require.True(t, *row.PubliclyAccessible)
	var endpoints []resource.DatabaseEndpoint
	require.NoError(t, json.Unmarshal(row.Endpoints, &endpoints))
	require.Len(t, endpoints, 2)
	require.Equal(t, "PRIVATE", endpoints[0].Type)
	require.Nil(t, endpoints[0].Port)
	require.NotContains(t, string(row.Endpoints), "203.0.113.9")
	require.NotContains(t, string(row.Endpoints), "example.test")
	var facts map[string]any
	require.NoError(t, json.Unmarshal(row.NativeDetails, &facts))
	require.Equal(t, "target-project:us-central1:db", facts["connection_name"])
	require.Contains(t, string(row.NativeDetails), "OUTGOING")
	require.Contains(t, string(row.NativeDetails), "psc.example.test.")
	require.Contains(t, string(row.NativeDetails), "POSTGRES_16_3")
	require.NotContains(t, string(row.NativeDetails), "secret")
	require.JSONEq(t, `{"owner":"db"}`, string(row.Tags))
	require.Nil(t, rows[1].StorageGB)
	require.Nil(t, rows[1].HAMode)
	require.Nil(t, rows[1].PubliclyAccessible)
	require.JSONEq(t, `[]`, string(rows[1].Endpoints))
}

func TestDatabasesRejectIncompleteProjectSnapshots(t *testing.T) {
	for _, body := range []string{
		`{"warnings":[{"code":"REGION_UNREACHABLE","region":"europe-west1","message":"secret"}]}`,
		`{"warnings":[{"code":"MAX_RESULTS_EXCEEDS_LIMIT"}]}`,
		`{"warnings":[{"code":"NO_RESULTS_ON_PAGE"}]}`,
		`{"warnings":[null]}`, `{"nextPageToken":"again"}`,
		`{"items":[{"name":"db","project":"foreign","region":"us-central1"}]}`,
		`{"items":[{"name":"db","project":"target-project","region":"invalid"}]}`,
		`{"items":[{"name":"db","project":"target-project","region":"us-central1","gceZone":"europe-west1-a"}]}`,
		`{"items":[{"name":"db","project":"target-project","region":"us-central1","connectionName":"foreign:us-central1:db"}]}`,
		`{"items":[{"name":"db","project":"target-project","region":"us-central1","ipAddresses":[{"type":"PRIMARY","ipAddress":"bad"}]}]}`,
		`{"items":[{"name":"db","project":"target-project","region":"us-central1"},{"name":"db","project":"target-project","region":"europe-west1"}]}`,
		`{"items":[null]}`,
	} {
		t.Run(body, func(t *testing.T) {
			key, private := fixtureKey(t)
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) { return 200, body })
			p := New(Options{Transport: transport})
			client, err := p.sqlClient(context.Background(), key)
			require.NoError(t, err)
			rows, err := p.databases(context.Background(), client, "target-project", "us-central1")
			require.Error(t, err)
			require.Nil(t, rows)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestDatabasesExcludeExternalRegistrationsAfterCompleteProjectListing(t *testing.T) {
	key, private := fixtureKey(t)
	calls := 0
	transport, _ := fixtureTransport(t, private, func(r *http.Request) (int, string) {
		calls++
		if r.URL.Query().Get("pageToken") == "" {
			return 200, `{"items":[{"name":"external","project":"target-project","instanceType":"ON_PREMISES_INSTANCE","onPremisesConfiguration":{"hostPort":"external.example.test:5432","password":"secret"}}],"nextPageToken":"p2"}`
		}
		return 200, `{"items":[{"name":"managed","project":"target-project","region":"us-central1","primaryDnsName":"primary.example.test.","writeEndpoint":"write.example.test.","ipv6Address":"2001:db8::1","secondaryGceZone":"us-central1-b"}]}`
	})
	p := New(Options{Transport: transport})
	client, err := p.sqlClient(context.Background(), key)
	require.NoError(t, err)
	rows, err := p.databases(context.Background(), client, "target-project", "us-central1")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, rows, 1)
	require.Equal(t, "managed", rows[0].DatabaseID)
	require.JSONEq(t, `[]`, string(rows[0].Endpoints))
	require.Contains(t, string(rows[0].NativeDetails), `"primary_dns_name":"primary.example.test."`)
	require.Contains(t, string(rows[0].NativeDetails), `"write_endpoint":"write.example.test."`)
	require.Contains(t, string(rows[0].NativeDetails), `"ipv6_address":"2001:db8::1"`)
	require.Contains(t, string(rows[0].NativeDetails), `"secondary_gce_zone":"us-central1-b"`)
	require.NotContains(t, string(rows[0].NativeDetails), "secret")
}

func TestSQLSelfLinkRequiresOfficialHostAndExactProjectIdentity(t *testing.T) {
	for _, raw := range []string{"https://sqladmin.googleapis.com/v1/projects/target-project/instances/db", "https://www.googleapis.com/sql/v1beta4/projects/target-project/instances/db"} {
		require.True(t, sqlSelfLink(raw, "target-project", "db"))
	}
	for _, raw := range []string{"https://sqladmin.googleapis.com/v1/projects/foreign/instances/db", "https://attacker.test/v1/projects/target-project/instances/db", "https://sqladmin.googleapis.com/v1/projects/target-project/instances/other", "https://sqladmin.googleapis.com/v1/projects/target-project/instances/db?secret=1", "https://user@sqladmin.googleapis.com/v1/projects/target-project/instances/db"} {
		require.False(t, sqlSelfLink(raw, "target-project", "db"))
	}
}
