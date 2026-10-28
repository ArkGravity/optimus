//go:build dbtest

package main

import (
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSecurityGroupMigrationPreservesAcceptedBaseline(t *testing.T) {
	database, keyID, _ := cloudKeyUpgradeDatabase(t)
	require.NoError(t, goose.UpTo(database, ".", 22))
	_, err := database.Exec(`INSERT INTO aws_vpcs (cloud_account_id,region,vpc_id,last_seen_at) SELECT id,'us-east-1','vpc-existing',now() FROM cloud_accounts WHERE name='live'`)
	require.NoError(t, err)
	require.NoError(t, goose.UpTo(database, ".", 23))
	require.NoError(t, goose.UpTo(database, ".", 23))
	var count int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id=$1", keyID).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM aws_vpcs WHERE vpc_id='vpc-existing'").Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM aws_security_groups").Scan(&count))
	require.Zero(t, count)
	require.NoError(t, goose.Down(database, "."))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.EqualValues(t, 22, version)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM aws_vpcs WHERE vpc_id='vpc-existing'").Scan(&count))
	require.Equal(t, 1, count)
}
