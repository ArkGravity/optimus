//go:build dbtest

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
	"github.com/ArkGravity/optimus/internal/modules/credentials/vault"
	"github.com/ArkGravity/optimus/internal/seed"
	"github.com/ArkGravity/optimus/migrations"
)

// Exercise repeated fresh-baseline migrate + seed with populated inventory and RBAC.
func TestMulticloudFreshBaselineAndSeedAcceptance(t *testing.T) {
	for _, legacyMenu := range []bool{true, false} {
		t.Run(fmt.Sprintf("legacy_menu_%t", legacyMenu), func(t *testing.T) {
			database, cleanup := startRawPostgres(t)
			t.Cleanup(cleanup)
			goose.SetBaseFS(migrations.FS)
			require.NoError(t, goose.SetDialect("postgres"))
			require.NoError(t, runGoose(database, "up"))
			gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: database}), &gorm.Config{})
			require.NoError(t, err)
			_, err = permissions.Register(context.Background(), gdb, permissions.All)
			require.NoError(t, err)
			opts := seed.Options{AdminUsername: "admin", AdminEmail: "acceptance@example.test", BcryptCost: 4}
			_, err = seed.Run(context.Background(), gdb, opts)
			require.NoError(t, err)
			var menu models.Menu
			require.NoError(t, gdb.Where("code = ?", "assets.firewalls").First(&menu).Error)
			legacyID := menu.ID
			if legacyMenu {
				require.NoError(t, gdb.Model(&menu).Update("code", "assets.security_groups").Error)
			}
			custom := models.Role{Code: "operator-custom", Name: "operator"}
			require.NoError(t, gdb.Create(&custom).Error)
			var permission models.Permission
			require.NoError(t, gdb.Where("code = ?", "assets:resource:read").First(&permission).Error)
			grant := models.RolePermission{RoleID: custom.ID, PermissionID: permission.ID}
			require.NoError(t, gdb.Create(&grant).Error)
			child := models.Menu{Code: "operator.child", Name: "operator", ParentID: &legacyID}
			require.NoError(t, gdb.Create(&child).Error)
			var keyID, accountID, runID int64
			access, secret := []byte{0, 255, 1, 128}, []byte{255, 0, 2, 129}
			require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys(name,provider,access_key_id_enc,secret_access_key_enc) VALUES ('preserved-aws','aws',$1,$2) RETURNING id`, access, secret).Scan(&keyID))
			require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts(name,provider,cloudkey_id,enabled_regions,enabled) VALUES ('preserved-account','aws',$1,ARRAY['us-east-1'],false) RETURNING id`, keyID).Scan(&accountID))
			require.NoError(t, database.QueryRow(`INSERT INTO assets_sync_runs(cloud_account_id,region,resource_type,status,trigger,started_at,items_seen,error,provider,location_type,location,scope_revision) VALUES ($1,'us-east-1','instance','failed','cron',now(),7,'historical-safe-message','aws','region','us-east-1',1) RETURNING id`, accountID).Scan(&runID))
			snapshots := map[string]string{}
			for table, idColumn := range map[string]string{"aws_instances": "instance_id", "aws_vpcs": "vpc_id", "aws_subnets": "subnet_id", "aws_databases": "db_instance_id", "aws_security_groups": "group_id"} {
				extraColumn, extraValue := "", ""
				if table == "aws_subnets" {
					extraColumn = ",vpc_id"
					extraValue = ",'vpc-preserved'"
				}
				_, err := database.Exec(fmt.Sprintf(`INSERT INTO %s(id,cloud_account_id,region,%s,last_seen_at%s) VALUES (7001,$1,'us-east-1','native-preserved',now()%s)`, table, idColumn, extraColumn, extraValue), accountID)
				require.NoError(t, err)
				var row string
				require.NoError(t, database.QueryRow(fmt.Sprintf(`SELECT row_to_json(r)::text FROM %s r WHERE id=7001`, table)).Scan(&row))
				snapshots[table] = row
			}
			for range 2 {
				require.NoError(t, goose.UpTo(database, ".", 19))
				_, err = seed.Run(context.Background(), gdb, opts)
				require.NoError(t, err)
				version, err := goose.GetDBVersion(database)
				require.NoError(t, err)
				require.EqualValues(t, 19, version)
				for table, want := range snapshots {
					var actual string
					require.NoError(t, database.QueryRow(fmt.Sprintf(`SELECT row_to_json(r)::text FROM %s r WHERE id=7001`, table)).Scan(&actual))
					require.Equal(t, want, actual, table)
				}
				var gotAccess, gotSecret []byte
				var binding int64
				var enabled bool
				var name, provider string
				require.NoError(t, database.QueryRow(`SELECT access_key_id_enc,secret_access_key_enc FROM credentials_cloud_keys WHERE id=$1`, keyID).Scan(&gotAccess, &gotSecret))
				require.Equal(t, access, gotAccess)
				require.Equal(t, secret, gotSecret)
				require.NoError(t, database.QueryRow(`SELECT name,provider,cloudkey_id,enabled FROM cloud_accounts WHERE id=$1`, accountID).Scan(&name, &provider, &binding, &enabled))
				require.Equal(t, "preserved-account", name)
				require.Equal(t, "aws", provider)
				require.Equal(t, keyID, binding)
				require.False(t, enabled)
				var status, message, runProvider, location string
				var seen int
				require.NoError(t, database.QueryRow(`SELECT status,error,items_seen,provider,location FROM assets_sync_runs WHERE id=$1`, runID).Scan(&status, &message, &seen, &runProvider, &location))
				require.Equal(t, "failed", status)
				require.Equal(t, "historical-safe-message", message)
				require.Equal(t, 7, seen)
				require.Equal(t, "aws", runProvider)
				require.Equal(t, "us-east-1", location)
				var canonical models.Menu
				require.NoError(t, gdb.Where("code = ?", "assets.firewalls").First(&canonical).Error)
				require.Equal(t, legacyID, canonical.ID)
				var gotChild models.Menu
				require.NoError(t, gdb.First(&gotChild, child.ID).Error)
				require.Equal(t, &legacyID, gotChild.ParentID)
				var count int64
				require.NoError(t, gdb.Model(&models.RolePermission{}).Where("role_id = ? AND permission_id = ?", custom.ID, permission.ID).Count(&count).Error)
				require.EqualValues(t, 1, count)
				require.NoError(t, gdb.Model(&models.Menu{}).Where("code IN ?", []string{"assets.security_groups", "assets.firewalls"}).Count(&count).Error)
				require.EqualValues(t, 1, count)
			}
		})
	}
}

// A validated encrypted SA credential remains intact through repeated Up.
func TestMulticloudValidServiceAccountBaselineAcceptance(t *testing.T) {
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 19))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]string{"type": "service_account", "project_id": "fixture-project", "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))})
	require.NoError(t, err)
	valid, metadata, err := cloudkey.ValidateServiceAccountJSON(raw)
	require.NoError(t, err)
	cipher, err := vault.NewCipher(bytes.Repeat([]byte{0x27}, vault.KeyLen))
	require.NoError(t, err)
	sealed, err := cipher.Seal(valid)
	require.NoError(t, err)
	var id int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys(name,provider,auth_type,service_account_json_enc,service_account_email,credential_project_id) VALUES ('validated-sa','gcp','service_account_json',$1,$2,$3) RETURNING id`, sealed, metadata.Email, metadata.ProjectID).Scan(&id))
	require.NoError(t, runGoose(database, "up"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.EqualValues(t, 19, version)
	var stored []byte
	require.NoError(t, database.QueryRow(`SELECT service_account_json_enc FROM credentials_cloud_keys WHERE id=$1`, id).Scan(&stored))
	require.Equal(t, sealed, stored)
	opened, err := cipher.Open(stored)
	require.NoError(t, err)
	defer clear(opened)
	require.JSONEq(t, string(valid), string(opened))
	// A fresh baseline's reverse sequence drops the owned credential table,
	// including SA JSON rows, after all dependent tables have been removed.
	require.NoError(t, goose.DownTo(database, ".", 10))
	var exists bool
	require.NoError(t, database.QueryRow("SELECT to_regclass('credentials_cloud_keys') IS NOT NULL").Scan(&exists))
	require.False(t, exists)
	require.NoError(t, runGoose(database, "up"))
}

func TestFreshBaselinePopulatedDownAndReinitialize(t *testing.T) {
	database, accountID, _ := nativeAssetDatabase(t)
	require.NoError(t, runGoose(database, "up"))
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: database}), &gorm.Config{})
	require.NoError(t, err)
	_, err = permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	opts := seed.Options{AdminUsername: "admin", AdminEmail: "baseline@example.test", BcryptCost: 4}
	for range 2 {
		_, err = seed.Run(context.Background(), gdb, opts)
		require.NoError(t, err)
	}
	// Populate every business table so reverse ordering exercises real FK edges.
	statements := []string{
		`INSERT INTO refresh_tokens(user_id,token_hash,expires_at) SELECT id,'baseline-token',now()+interval '1 hour' FROM users LIMIT 1`,
		`INSERT INTO audit_logs(user_id,action) SELECT id,'baseline-test' FROM users LIMIT 1`,
		`INSERT INTO credentials_ssh_keys(name,username,private_key_enc) VALUES ('ssh','fixture','sealed')`,
		`INSERT INTO credentials_kubeconfigs(name,kubeconfig_enc) VALUES ('kube','sealed')`,
		`INSERT INTO clusters(name,kubeconfig_id,context) SELECT 'cluster',id,'fixture' FROM credentials_kubeconfigs LIMIT 1`,
		`INSERT INTO apps_chart_repos(name,type,url) VALUES ('repo','http','https://example.test')`,
		`INSERT INTO apps_applications(name,cluster_id,namespace,release_name,chart_repo_id,chart_name) SELECT 'app',c.id,'fixture','release',r.id,'chart' FROM clusters c CROSS JOIN apps_chart_repos r LIMIT 1`,
		`INSERT INTO aws_instances(cloud_account_id,region,instance_id,last_seen_at) VALUES ($1,'region-a','instance',now())`,
		`INSERT INTO aws_vpcs(cloud_account_id,region,vpc_id,last_seen_at) VALUES ($1,'region-a','vpc',now())`,
		`INSERT INTO aws_subnets(cloud_account_id,region,subnet_id,vpc_id,last_seen_at) VALUES ($1,'region-a','subnet','vpc',now())`,
		`INSERT INTO aws_databases(cloud_account_id,region,db_instance_id,last_seen_at) VALUES ($1,'region-a','db',now())`,
		`INSERT INTO aws_security_groups(cloud_account_id,region,group_id,last_seen_at) VALUES ($1,'region-a','sg',now())`,
		`INSERT INTO assets_sync_runs(cloud_account_id,region,resource_type,started_at,status,trigger,provider,location_type,location,scope_revision) VALUES ($1,'region-a','instance',now(),'success','cron','aws','region','region-a',1)`,
	}
	for _, statement := range statements {
		var args []any
		if bytes.Contains([]byte(statement), []byte("$1")) {
			args = append(args, accountID)
		}
		_, err = database.Exec(statement, args...)
		require.NoError(t, err)
	}
	for _, table := range nativeAssetTables {
		region := "region-a"
		if table.location == "" {
			region = ""
		}
		_, err = database.Exec(fmt.Sprintf(`INSERT INTO %s(cloud_account_id,%s,region,last_seen_at) VALUES ($1,'native',$2,now())`, table.name, table.id), accountID, region)
		require.NoError(t, err)
	}
	rows, err := database.Query(`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version'`)
	require.NoError(t, err)
	var tables []string
	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, tables, 31)
	for _, table := range tables {
		var count int
		require.NoError(t, database.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count))
		require.Positive(t, count, table)
	}
	require.NoError(t, goose.DownTo(database, ".", 0))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.Zero(t, version)
	var remaining int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version'`).Scan(&remaining))
	require.Zero(t, remaining)
	for range 2 {
		require.NoError(t, runGoose(database, "up"))
		_, err = permissions.Register(context.Background(), gdb, permissions.All)
		require.NoError(t, err)
		_, err = seed.Run(context.Background(), gdb, opts)
		require.NoError(t, err)
	}
	version, err = goose.GetDBVersion(database)
	require.NoError(t, err)
	require.EqualValues(t, 19, version)
	var menus int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ?", "assets.firewalls").Count(&menus).Error)
	require.EqualValues(t, 1, menus)
}
