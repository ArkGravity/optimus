//go:build dbtest

package main

import (
	"testing"

	"github.com/ArkGravity/optimus/migrations"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestGKEIncrementalMigrationPreservesKubeconfigAndGuardsDowngrade(t *testing.T) {
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 19))
	ciphertext := []byte{0, 1, 255, 128}
	var id, keyID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_kubeconfigs(name,kubeconfig_enc) VALUES ('preserved',$1) RETURNING id`, ciphertext).Scan(&id))
	require.NoError(t, runGoose(database, "up"))
	var stored []byte
	var mode string
	var binding *int64
	require.NoError(t, database.QueryRow(`SELECT kubeconfig_enc,auth_type,cloud_key_id FROM credentials_kubeconfigs WHERE id=$1`, id).Scan(&stored, &mode, &binding))
	require.Equal(t, ciphertext, stored)
	require.Equal(t, "kubeconfig", mode)
	require.Nil(t, binding)
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys(name,provider,auth_type,service_account_json_enc) VALUES ('gcp','gcp','service_account_json',$1) RETURNING id`, []byte{1}).Scan(&keyID))
	_, err := database.Exec(`INSERT INTO credentials_kubeconfigs(name,kubeconfig_enc,auth_type,cloud_key_id,gke_project,gke_location,gke_cluster,gke_endpoint) VALUES ('gke',$1,'gke_service_account',$2,'fixture-project','us-central1','fixture','default')`, []byte{2}, keyID)
	require.NoError(t, err)
	_, err = database.Exec(`DELETE FROM credentials_cloud_keys WHERE id=$1`, keyID)
	require.Error(t, err, "foreign key must protect GKE credential binding")
	require.Error(t, goose.DownTo(database, ".", 19), "downgrade must not discard GKE bindings")
	_, err = database.Exec(`DELETE FROM credentials_kubeconfigs WHERE auth_type='gke_service_account'`)
	require.NoError(t, err)
	require.NoError(t, goose.DownTo(database, ".", 19))
	require.NoError(t, database.QueryRow(`SELECT kubeconfig_enc FROM credentials_kubeconfigs WHERE id=$1`, id).Scan(&stored))
	require.Equal(t, ciphertext, stored)
	require.NoError(t, runGoose(database, "up"))
}
