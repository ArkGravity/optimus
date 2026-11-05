//go:build dbtest

package main

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestAWSBaselinePreservesAccountsAndOwnsSecurityGroups(t *testing.T) {
	database, keyID, _ := cloudKeyBaselineDatabase(t)
	require.NoError(t, goose.UpTo(database, ".", 16))
	_, err := database.Exec(`INSERT INTO aws_vpcs (cloud_account_id,region,vpc_id,last_seen_at) SELECT id,'us-east-1','vpc-existing',now() FROM cloud_accounts WHERE name='live'`)
	require.NoError(t, err)
	require.NoError(t, goose.UpTo(database, ".", 16))
	require.NoError(t, goose.UpTo(database, ".", 16))
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
	require.EqualValues(t, 15, version)
	var exists bool
	require.NoError(t, database.QueryRow("SELECT to_regclass('aws_vpcs') IS NOT NULL OR to_regclass('aws_security_groups') IS NOT NULL").Scan(&exists))
	require.False(t, exists)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM cloud_accounts WHERE cloudkey_id=$1", keyID).Scan(&count))
	require.Equal(t, 2, count)
}
