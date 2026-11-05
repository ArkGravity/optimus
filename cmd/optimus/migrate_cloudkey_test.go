//go:build dbtest

package main

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/migrations"
)

func cloudKeyBaselineDatabase(t *testing.T) (*sql.DB, int64, int64) {
	t.Helper()
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 15))
	var keyID, historyID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ('baseline-key', 'aws', $1, $2) RETURNING id`, []byte("sealed-ak"), []byte("sealed-sk")).Scan(&keyID))
	_, err := database.Exec(`INSERT INTO cloud_accounts (name, provider, cloudkey_id, enabled_regions)
        VALUES ('live', 'aws', $1, ARRAY['us-east-1'])`, keyID)
	require.NoError(t, err)
	require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts
        (name, provider, cloudkey_id, enabled_regions, deleted_at)
        VALUES ('history', 'aws', $1, ARRAY['us-east-1'], now()) RETURNING id`, keyID).Scan(&historyID))
	return database, keyID, historyID
}

func TestCloudKeyBaselineEnforcesLiveAndHistoricalReferences(t *testing.T) {
	database, keyID, historyID := cloudKeyBaselineDatabase(t)
	for range 2 {
		require.NoError(t, runGoose(database, "up"))
	}
	var accounts int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id = $1", keyID).Scan(&accounts))
	require.Equal(t, 2, accounts)
	var accessKey, secretKey []byte
	require.NoError(t, database.QueryRow("SELECT access_key_id_enc, secret_access_key_enc FROM credentials_cloud_keys WHERE id = $1", keyID).Scan(&accessKey, &secretKey))
	require.Equal(t, []byte("sealed-ak"), accessKey)
	require.Equal(t, []byte("sealed-sk"), secretKey)
	_, err := database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE deleted_at IS NULL")
	requirePostgresCode(t, err, "23514")
	_, err = database.Exec("DELETE FROM credentials_cloud_keys WHERE id = $1", keyID)
	requirePostgresCode(t, err, "23503")
	_, err = database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE id = $1", historyID)
	require.NoError(t, err)
	_, err = database.Exec("UPDATE cloud_accounts SET deleted_at = NULL WHERE id = $1", historyID)
	requirePostgresCode(t, err, "23514")
	_, err = database.Exec("DELETE FROM cloud_accounts WHERE deleted_at IS NULL")
	require.NoError(t, err)
	_, err = database.Exec("DELETE FROM credentials_cloud_keys WHERE id = $1", keyID)
	require.NoError(t, err)
	var detached bool
	require.NoError(t, database.QueryRow("SELECT cloudkey_id IS NULL FROM cloud_accounts WHERE id=$1", historyID).Scan(&detached))
	require.True(t, detached)
}
