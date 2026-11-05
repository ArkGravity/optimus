//go:build dbtest

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncRunBaselinePreservesScopeAndHistory(t *testing.T) {
	database, keyID, historyID := cloudKeyBaselineDatabase(t)
	require.NoError(t, runGoose(database, "up"))
	var liveID int64
	require.NoError(t, database.QueryRow("SELECT id FROM cloud_accounts WHERE name='live'").Scan(&liveID))
	var ids []int64
	type runFacts struct{ provider, family, status, locationType string }
	expected := map[int64]runFacts{}
	for _, provider := range []string{"aws", "aliyun", "gcp"} {
		for _, accountID := range []int64{liveID, historyID} {
			for i, family := range []string{"instance", "network", "database", "security_group", "subnet", "firewall", "planning"} {
				var id int64
				locationType := []string{"global", "region", "zone"}[i%3]
				require.NoError(t, database.QueryRow(`INSERT INTO assets_sync_runs
                    (cloud_account_id,region,resource_type,started_at,status,trigger,items_seen,error,provider,location_type,location,scope_revision)
                    VALUES ($1,'fixture-region',$2,now(),$3,'cron',7,'historical-safe-message',$4,$5,'original-location',3) RETURNING id`,
					accountID, family, []string{"running", "success", "failed", "skipped"}[i%4], provider, locationType).Scan(&id))
				ids = append(ids, id)
				expected[id] = runFacts{provider, family, []string{"running", "success", "failed", "skipped"}[i%4], locationType}
			}
		}
	}
	_, err := database.Exec("UPDATE cloud_accounts SET scope_revision=9")
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, runGoose(database, "up"))
	}
	for _, id := range ids {
		var region, family, status, message, provider, locationType, location string
		var count, revision int
		require.NoError(t, database.QueryRow(`SELECT region,resource_type,status,items_seen,error,provider,location_type,location,scope_revision FROM assets_sync_runs WHERE id=$1`, id).Scan(&region, &family, &status, &count, &message, &provider, &locationType, &location, &revision))
		require.Equal(t, "fixture-region", region)
		require.Equal(t, expected[id].family, family)
		require.Equal(t, expected[id].status, status)
		require.Equal(t, 7, count)
		require.Equal(t, "historical-safe-message", message)
		require.Equal(t, expected[id].provider, provider)
		require.Equal(t, expected[id].locationType, locationType)
		require.Equal(t, "original-location", location)
		require.Equal(t, 3, revision)
	}
	for column, value := range map[string]string{"provider": "'unknown'", "location_type": "'unknown'", "scope_revision": "0", "resource_type": "'bucket'"} {
		_, err := database.Exec(fmt.Sprintf("UPDATE assets_sync_runs SET %s=%s WHERE id=$1", column, value), ids[0])
		requirePostgresCode(t, err, "23514")
	}
	for _, column := range []string{"provider", "location_type", "location", "scope_revision"} {
		_, err := database.Exec(fmt.Sprintf("UPDATE assets_sync_runs SET %s=NULL WHERE id=$1", column), ids[0])
		requirePostgresCode(t, err, "23502")
	}
	_, err = database.Exec("UPDATE cloud_accounts SET cloudkey_id=NULL WHERE id=$1", historyID)
	require.NoError(t, err)
	_, err = database.Exec("DELETE FROM assets_sync_runs WHERE cloud_account_id=$1", liveID)
	require.NoError(t, err)
	_, err = database.Exec("DELETE FROM cloud_accounts WHERE id=$1", liveID)
	require.NoError(t, err)
	_, err = database.Exec("DELETE FROM credentials_cloud_keys WHERE id=$1", keyID)
	require.NoError(t, err)
	var count int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM assets_sync_runs WHERE cloud_account_id=$1", historyID).Scan(&count))
	require.Equal(t, 21, count)
	_, err = database.Exec("DELETE FROM cloud_accounts WHERE id=$1", historyID)
	requirePostgresCode(t, err, "23503")
}
