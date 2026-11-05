package migrations

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedFreshBaselineLayout(t *testing.T) {
	names := []string{"create_users", "create_roles", "create_permissions", "create_user_roles", "create_role_permissions", "create_menus", "create_refresh_tokens", "create_audit_logs", "create_credentials_ssh_keys", "create_credentials_kubeconfigs", "create_credentials_cloud_keys", "create_clusters", "create_apps_chart_repos", "create_apps_applications", "create_cloud_accounts", "create_aws_assets", "create_aliyun_assets", "create_gcp_assets", "create_assets_sync_runs"}
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)
	var actual []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			actual = append(actual, entry.Name())
		}
	}
	var expected []string
	for i, name := range names {
		expected = append(expected, fmt.Sprintf("%05d_%s.sql", i+1, name))
	}
	require.Equal(t, expected, actual)
	goose.SetBaseFS(FS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	collected, err := goose.CollectMigrations(".", 0, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, collected, 19)
	for i, migration := range collected {
		require.EqualValues(t, i+1, migration.Version)
	}
}

func TestCloudAccountsReferencesCredentialsCloudKeys(t *testing.T) {
	sql, err := FS.ReadFile("00015_create_cloud_accounts.sql")
	require.NoError(t, err)
	require.Contains(t, string(sql), "REFERENCES credentials_cloud_keys(id)")
	require.NotContains(t, string(sql), "REFERENCES credential_cloud_keys")
}
