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

func TestCloudKeyProviderMigrationPreservesLegacyCredentials(t *testing.T) {
	database, keyID, _ := cloudKeyUpgradeDatabase(t)
	var legacyID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ('legacy-azure', 'azure', $1, $2) RETURNING id`, []byte("legacy-ak"), []byte("legacy-sk")).Scan(&legacyID))
	require.NoError(t, runGoose(database, "up"))
	require.NoError(t, runGoose(database, "up"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(23), version)
	var provider string
	var accessKey, secretKey []byte
	require.NoError(t, database.QueryRow("SELECT provider, access_key_id_enc, secret_access_key_enc FROM credentials_cloud_keys WHERE id = $1", legacyID).Scan(&provider, &accessKey, &secretKey))
	require.Equal(t, "azure", provider)
	require.Equal(t, []byte("legacy-ak"), accessKey)
	require.Equal(t, []byte("legacy-sk"), secretKey)
	var accounts int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id = $1", keyID).Scan(&accounts))
	require.Equal(t, 2, accounts)
	var aliyunID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ('aliyun-key', 'aliyun', $1, $2) RETURNING id`, []byte("aliyun-ak"), []byte("aliyun-sk")).Scan(&aliyunID))
	require.Error(t, runGoose(database, "down"), "downgrade must not destroy Aliyun credentials")
	version, err = goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(23), version)
	_, err = database.Exec("DELETE FROM credentials_cloud_keys WHERE id = $1", aliyunID)
	require.NoError(t, err)
	require.NoError(t, runGoose(database, "down"))
	version, err = goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Equal(t, int64(22), version)
	_, err = database.Exec(`INSERT INTO credentials_cloud_keys
        (name, provider, access_key_id_enc, secret_access_key_enc)
        VALUES ('rejected', 'aliyun', $1, $2)`, []byte("ak"), []byte("sk"))
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code)
	require.NoError(t, runGoose(database, "up"))
}
