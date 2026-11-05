package gcp

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/stretchr/testify/require"
)

func TestSDKClientsNeverLogRawPayloadsWithAmbientDebugEnabled(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_LOGGING_LEVEL", "debug")
	for _, kind := range []string{"database", "instance", "firewall"} {
		t.Run(kind, func(t *testing.T) {
			key, private := fixtureKey(t)
			calls := 0
			transport, _ := fixtureTransport(t, private, func(*http.Request) (int, string) {
				calls++
				if calls == 2 {
					return 403, `{"error":{"code":403,"message":"fixture-secret-error-body"}}`
				}
				switch kind {
				case "database":
					return 200, `{"items":[{"name":"db","project":"target-project","region":"us-central1","rootPassword":"fixture-secret-sql-root","onPremisesConfiguration":{"password":"fixture-secret-onprem"},"settings":{"databaseFlags":[{"name":"private","value":"fixture-secret-flag"}]}}]}`
				case "instance":
					return 200, `{"items":[{"id":"1","name":"vm","zone":"zones/us-central1-a","metadata":{"items":[{"key":"startup-script","value":"fixture-secret-startup"}]}}]}`
				default:
					return 200, `{"items":[{"id":"2","name":"allow","network":"global/networks/main","description":"fixture-secret-description","allowed":[{"IPProtocol":"all"}]}]}`
				}
			})
			unit := familyUnit(kind)
			if kind == "instance" {
				unit = assetsync.Unit{ResourceType: kind, LocationType: "zone", Location: "us-central1-a", Region: "us-central1", Zone: "us-central1-a", Scope: account.Scope{Provider: "gcp", ProjectID: "target-project"}}
			}
			output := captureSDKStderr(t, func() {
				snapshot, err := New(Options{Transport: transport}).Fetch(context.Background(), unit, key)
				require.NoError(t, err)
				require.True(t, snapshot.Complete)
				failed, err := New(Options{Transport: transport}).Fetch(context.Background(), unit, key)
				require.Error(t, err)
				require.NotContains(t, err.Error(), "fixture-secret")
				require.False(t, failed.Complete)
				require.Nil(t, failed.Publish)
			})
			require.NotContains(t, output, "fixture-secret")
			require.NotContains(t, output, "fixture-token")
			require.Empty(t, strings.TrimSpace(output), "SDK payload logging must be explicitly disabled")
		})
	}
}

func captureSDKStderr(t *testing.T, action func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = old; writer.Close(); reader.Close() }()
	done := make(chan string, 1)
	go func() { raw, _ := io.ReadAll(reader); done <- string(raw) }()
	action()
	require.NoError(t, writer.Close())
	os.Stderr = old
	return <-done
}
