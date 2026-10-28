//go:build dbtest

package main

import (
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/migrations"
)

func cloudKeyUpgradeDatabase(t *testing.T) (*sql.DB, int64, int64) {
	t.Helper()
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 21))
	var keyID, historyID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ('upgrade-key', 'aws', $1, $2) RETURNING id`, []byte("sealed-ak"), []byte("sealed-sk")).Scan(&keyID))
	_, err := database.Exec(`INSERT INTO cloud_accounts (name, provider, cloudkey_id, enabled_regions)
        VALUES ('live', 'aws', $1, ARRAY['us-east-1'])`, keyID)
	require.NoError(t, err)
	require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts
        (name, provider, cloudkey_id, enabled_regions, deleted_at)
        VALUES ('history', 'aws', $1, ARRAY['us-east-1'], now()) RETURNING id`, keyID).Scan(&historyID))
	return database, keyID, historyID
}

func TestCloudKeyMigrationUpgradesExistingBaselineWithoutDataLoss(t *testing.T) {
	database, keyID, historyID := cloudKeyUpgradeDatabase(t)
	require.NoError(t, goose.UpTo(database, ".", 22))
	require.NoError(t, goose.UpTo(database, ".", 22))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(22), version)
	var accounts int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id = $1", keyID).Scan(&accounts))
	require.Equal(t, 2, accounts)
	var accessKey, secretKey []byte
	require.NoError(t, database.QueryRow("SELECT access_key_id_enc, secret_access_key_enc FROM credentials_cloud_keys WHERE id = $1", keyID).Scan(&accessKey, &secretKey))
	require.Equal(t, []byte("sealed-ak"), accessKey)
	require.Equal(t, []byte("sealed-sk"), secretKey)
	_, err = database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE deleted_at IS NULL")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code)
	_, err = database.Exec("DELETE FROM credentials_cloud_keys WHERE id = $1", keyID)
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23503", pgErr.Code)
	_, err = database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE id = $1", historyID)
	require.NoError(t, err)
	_, err = database.Exec("UPDATE cloud_accounts SET deleted_at = NULL WHERE id = $1", historyID)
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code)
}

func TestCloudKeyMigrationDowngradePreservesExistingRows(t *testing.T) {
	database, keyID, _ := cloudKeyUpgradeDatabase(t)
	require.NoError(t, goose.UpTo(database, ".", 22))
	require.NoError(t, runGoose(database, "down"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(21), version)
	var accounts int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id = $1", keyID).Scan(&accounts))
	require.Equal(t, 2, accounts)
	_, err = database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE deleted_at IS NOT NULL")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23502", pgErr.Code)
	require.NoError(t, runGoose(database, "up"))
}

func TestCloudKeyMigrationRejectsUnsafeDowngrade(t *testing.T) {
	database, _, historyID := cloudKeyUpgradeDatabase(t)
	require.NoError(t, goose.UpTo(database, ".", 22))
	_, err := database.Exec("UPDATE cloud_accounts SET cloudkey_id = NULL WHERE id = $1", historyID)
	require.NoError(t, err)
	require.Error(t, runGoose(database, "down"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(22), version)
	var detached bool
	require.NoError(t, database.QueryRow("SELECT cloudkey_id IS NULL FROM cloud_accounts WHERE id = $1", historyID).Scan(&detached))
	require.True(t, detached)
	_, err = database.Exec("UPDATE cloud_accounts SET deleted_at = NULL WHERE id = $1", historyID)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code)
}

func TestCloudKeyProviderBaselinePreservesCredentials(t *testing.T) {
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 14))
	providers := []string{"aliyun", "aws", "gcp"}
	ids := make([]int64, len(providers))
	for i, provider := range providers {
		require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys
            (name, provider, access_key_id_enc, secret_access_key_enc)
            VALUES ($1, $2, $3, $4) RETURNING id`, provider+"-key", provider,
			[]byte("sealed-ak"), []byte("sealed-sk")).Scan(&ids[i]))
	}
	require.NoError(t, runGoose(database, "up"))
	require.NoError(t, runGoose(database, "up"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(22), version)
	for i, id := range ids {
		var provider string
		var accessKey, secretKey []byte
		require.NoError(t, database.QueryRow("SELECT provider, access_key_id_enc, secret_access_key_enc FROM credentials_cloud_keys WHERE id = $1", id).Scan(&provider, &accessKey, &secretKey))
		require.Equal(t, providers[i], provider)
		require.Equal(t, []byte("sealed-ak"), accessKey)
		require.Equal(t, []byte("sealed-sk"), secretKey)
	}
	for _, provider := range []string{"azure", "ibm"} {
		_, err = database.Exec(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ($1, $2, $3, $4)`, provider+"-rejected", provider, []byte("ak"), []byte("sk"))
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		require.Equal(t, "23514", pgErr.Code)
	}
}
