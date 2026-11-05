//go:build dbtest

package main

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/migrations"
)

func requirePostgresCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, code, pgErr.Code)
}

func TestCloudCredentialAuthMigrationEnforcesAuthShapes(t *testing.T) {
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	require.NoError(t, runGoose(database, "up"))
	for _, provider := range []string{"aws", "aliyun", "gcp", "azure"} {
		for _, authType := range []string{"access_key", "service_account_json", "legacy_invalid", "unknown"} {
			for shape := 0; shape < 8; shape++ {
				t.Run(fmt.Sprintf("%s/%s/%03b", provider, authType, shape), func(t *testing.T) {
					var accessKey, secretKey, serviceAccount []byte
					if shape&1 != 0 {
						accessKey = []byte("sealed-ak")
					}
					if shape&2 != 0 {
						secretKey = []byte("sealed-sk")
					}
					if shape&4 != 0 {
						serviceAccount = []byte("sealed-json")
					}
					_, err := database.Exec(`INSERT INTO credentials_cloud_keys
						(name, provider, auth_type, access_key_id_enc, secret_access_key_enc, service_account_json_enc)
						VALUES ($1, $2, $3, $4, $5, $6)`, fmt.Sprintf("%s-%s-%d", provider, authType, shape),
						provider, authType, accessKey, secretKey, serviceAccount)
					valid := ((provider == "aws" || provider == "aliyun") && authType == "access_key" && shape == 3) ||
						(provider == "gcp" && authType == "service_account_json" && shape == 4) ||
						(provider == "gcp" && authType == "legacy_invalid" && shape == 3)
					if valid {
						require.NoError(t, err)
					} else {
						requirePostgresCode(t, err, "23514")
					}
				})
			}
		}
	}
}

func TestCloudAccountBaselineDefaultsAndValidatesTargets(t *testing.T) {
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 15))
	var keyID, accountID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys(name,provider,auth_type,access_key_id_enc,secret_access_key_enc) VALUES ('aws','aws','access_key',$1,$2) RETURNING id`, []byte("ak"), []byte("sk")).Scan(&keyID))
	require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts(name,provider,cloudkey_id,enabled_regions,enabled) VALUES ('accepted','aws',$1,ARRAY['us-east-1'],false) RETURNING id`, keyID).Scan(&accountID))
	require.NoError(t, goose.UpTo(database, ".", 16))
	require.NoError(t, goose.UpTo(database, ".", 16))
	var project, identity string
	var revision int64
	var regions []byte
	var enabled bool
	var binding int64
	require.NoError(t, database.QueryRow(`SELECT project_id,external_account_id,scope_revision,array_to_json(enabled_regions),enabled,cloudkey_id FROM cloud_accounts WHERE id=$1`, accountID).Scan(&project, &identity, &revision, &regions, &enabled, &binding))
	require.Empty(t, project)
	require.Empty(t, identity)
	require.EqualValues(t, 1, revision)
	require.JSONEq(t, `["us-east-1"]`, string(regions))
	require.False(t, enabled)
	require.Equal(t, keyID, binding)
	for _, tc := range []struct {
		provider, project string
		valid             bool
	}{{"aws", "", true}, {"aliyun", "", true}, {"gcp", "target-project", true}, {"gcp", "", false}, {"aws", "target-project", false}, {"aliyun", "target-project", false}} {
		_, err := database.Exec(`INSERT INTO cloud_accounts(name,provider,project_id,cloudkey_id,enabled_regions) VALUES ($1,$2,$3,$4,ARRAY['fixture'])`, tc.provider+tc.project, tc.provider, tc.project, keyID)
		if tc.valid {
			require.NoError(t, err)
		} else {
			requirePostgresCode(t, err, "23514")
		}
	}
	_, err := database.Exec(`UPDATE cloud_accounts SET scope_revision=0 WHERE id=$1`, accountID)
	requirePostgresCode(t, err, "23514")
}

// Native asset migrations start from the already accepted multi-cloud account schema.
func nativeAssetDatabase(t *testing.T) (*sql.DB, int64, int64) {
	t.Helper()
	database, cleanup := startRawPostgres(t)
	t.Cleanup(cleanup)
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(database, ".", 16))
	var keyID, accountID, otherAccountID int64
	require.NoError(t, database.QueryRow(`INSERT INTO credentials_cloud_keys(name,provider,access_key_id_enc,secret_access_key_enc) VALUES ('native-test','aws',$1,$2) RETURNING id`, []byte{0, 0xff, 1}, []byte{0xff, 0, 2}).Scan(&keyID))
	require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts(name,provider,cloudkey_id,enabled_regions) VALUES ('first','aws',$1,ARRAY['region-a','region-b']) RETURNING id`, keyID).Scan(&accountID))
	require.NoError(t, database.QueryRow(`INSERT INTO cloud_accounts(name,provider,cloudkey_id,enabled_regions) VALUES ('second','aws',$1,ARRAY['region-a']) RETURNING id`, keyID).Scan(&otherAccountID))
	return database, accountID, otherAccountID
}

type nativeAssetTable struct {
	name, id, location string
	arrays, textFields []string
}

var nativeAssetTables = []nativeAssetTable{
	{"aliyun_instances", "instance_id", "region", []string{"network_interfaces"}, []string{"name", "instance_type", "native_status", "region", "zone"}},
	{"aliyun_vpcs", "network_id", "region", []string{"cidr_blocks"}, []string{"native_ref", "name", "region", "native_status"}},
	{"aliyun_vswitches", "subnet_id", "region", []string{"cidr_blocks"}, []string{"network_ref", "name", "region", "zone"}},
	{"aliyun_databases", "database_id", "region", []string{"endpoints"}, []string{"name", "product", "engine", "engine_version", "instance_class", "native_status", "region", "zone"}},
	{"aliyun_security_groups", "firewall_id", "region", []string{"inbound_rules", "outbound_rules"}, []string{"name", "network_ref", "region", "description"}},
	{"gcp_instances", "instance_id", "zone", []string{"network_interfaces"}, []string{"name", "instance_type", "native_status", "region", "zone"}},
	{"gcp_networks", "network_id", "", []string{"cidr_blocks"}, []string{"native_ref", "name", "region", "native_status"}},
	{"gcp_subnets", "subnet_id", "region", []string{"cidr_blocks"}, []string{"network_ref", "name", "region", "zone"}},
	{"gcp_databases", "database_id", "region", []string{"endpoints"}, []string{"name", "product", "engine", "engine_version", "instance_class", "native_status", "region", "zone"}},
	{"gcp_firewalls", "firewall_id", "", []string{"inbound_rules", "outbound_rules"}, []string{"name", "network_ref", "region", "description"}},
}

func TestNativeAssetMigrationsPreserveAWSAndDowngradeOnlyOwnTables(t *testing.T) {
	database, accountID, _ := nativeAssetDatabase(t)
	awsTables := map[string]string{"aws_instances": "instance_id", "aws_vpcs": "vpc_id", "aws_subnets": "subnet_id", "aws_databases": "db_instance_id", "aws_security_groups": "group_id"}
	snapshots := map[string]string{}
	for table, id := range awsTables {
		extraColumn, extraValue := "", ""
		if table == "aws_subnets" {
			extraColumn, extraValue = ",vpc_id", ",'aws-vpc'"
		}
		_, err := database.Exec(fmt.Sprintf(`INSERT INTO %s(id,cloud_account_id,region,%s,last_seen_at%s) VALUES (701,$1,'region-a','aws-preserved',now()%s)`, table, id, extraColumn, extraValue), accountID)
		require.NoError(t, err)
		var snapshot string
		require.NoError(t, database.QueryRow(fmt.Sprintf(`SELECT row_to_json(r)::text FROM %s r WHERE id=701`, table)).Scan(&snapshot))
		snapshots[table] = snapshot
	}
	assertPreserved := func() {
		for table, snapshot := range snapshots {
			var actual string
			require.NoError(t, database.QueryRow(fmt.Sprintf(`SELECT row_to_json(r)::text FROM %s r WHERE id=701`, table)).Scan(&actual))
			require.Equal(t, snapshot, actual)
		}
		var access, secret []byte
		require.NoError(t, database.QueryRow(`SELECT access_key_id_enc,secret_access_key_enc FROM credentials_cloud_keys WHERE name='native-test'`).Scan(&access, &secret))
		require.Equal(t, []byte{0, 0xff, 1}, access)
		require.Equal(t, []byte{0xff, 0, 2}, secret)
	}
	require.NoError(t, runGoose(database, "up"))
	require.NoError(t, runGoose(database, "up"))
	version, err := goose.GetDBVersion(database)
	require.NoError(t, err)
	require.EqualValues(t, 19, version)
	assertPreserved()
	require.NoError(t, goose.DownTo(database, ".", 17))
	for _, table := range nativeAssetTables {
		var exists bool
		require.NoError(t, database.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table.name).Scan(&exists))
		require.Equal(t, strings.HasPrefix(table.name, "aliyun_"), exists, table.name)
	}
	require.NoError(t, goose.DownTo(database, ".", 16))
	for _, table := range nativeAssetTables {
		var exists bool
		require.NoError(t, database.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table.name).Scan(&exists))
		require.False(t, exists, table.name)
	}
	assertPreserved()
	require.NoError(t, runGoose(database, "up"))
}

func TestNativeAssetSchemaConstraintsAndIndexes(t *testing.T) {
	database, accountID, otherID := nativeAssetDatabase(t)
	require.NoError(t, runGoose(database, "up"))
	for _, table := range nativeAssetTables {
		t.Run(table.name, func(t *testing.T) {
			columns := map[string]struct{ kind, nullable, defaultValue string }{}
			rows, err := database.Query(`SELECT column_name,data_type,is_nullable,COALESCE(column_default,'') FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table.name)
			require.NoError(t, err)
			for rows.Next() {
				var name, kind, nullable, defaultValue string
				require.NoError(t, rows.Scan(&name, &kind, &nullable, &defaultValue))
				columns[name] = struct{ kind, nullable, defaultValue string }{kind, nullable, defaultValue}
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			require.Equal(t, "bigint", columns["id"].kind)
			require.Contains(t, columns["id"].defaultValue, "nextval(")
			require.Equal(t, "bigint", columns["cloud_account_id"].kind)
			var foreignKey, deleteAction string
			require.NoError(t, database.QueryRow(`SELECT pg_get_constraintdef(oid), confdeltype::text FROM pg_constraint WHERE conrelid=$1::regclass AND contype='f'`, table.name).Scan(&foreignKey, &deleteAction))
			require.Equal(t, "FOREIGN KEY (cloud_account_id) REFERENCES cloud_accounts(id)", foreignKey)
			require.Contains(t, []string{"a", "r"}, deleteAction)
			var primaryKey string
			require.NoError(t, database.QueryRow(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid=$1::regclass AND contype='p'`, table.name).Scan(&primaryKey))
			require.Equal(t, "PRIMARY KEY (id)", primaryKey)
			for _, field := range append(table.textFields, table.id) {
				require.Equal(t, "text", columns[field].kind, field)
				require.Equal(t, "NO", columns[field].nullable, field)
			}
			for _, field := range []string{"last_seen_at", "created_at", "updated_at", "deleted_at"} {
				require.Equal(t, "timestamp with time zone", columns[field].kind, field)
			}
			require.Equal(t, "NO", columns["last_seen_at"].nullable)
			require.Equal(t, "YES", columns["deleted_at"].nullable)
			for _, field := range []string{"created_at", "updated_at"} {
				require.Contains(t, columns[field].defaultValue, "now()")
			}
			if table.id == "database_id" {
				for field, kind := range map[string]string{"storage_gb": "bigint", "ha_mode": "text", "publicly_accessible": "boolean"} {
					require.Equal(t, kind, columns[field].kind)
					require.Equal(t, "YES", columns[field].nullable)
					require.Empty(t, columns[field].defaultValue)
				}
			}
			insert := fmt.Sprintf(`INSERT INTO %s(cloud_account_id,%s,region`, table.name, table.id)
			values := ` VALUES ($1,$2,$3`
			if table.location == "zone" {
				insert += `,zone`
				values += `,$3`
			}
			insert += `,last_seen_at)` + values + `,now()) RETURNING id`
			insertRow := func(account int64, native, location string) (int64, error) {
				var id int64
				err := database.QueryRow(insert, account, native, location).Scan(&id)
				return id, err
			}
			location := "region-a"
			if table.location == "" {
				location = ""
			}
			rowID, err := insertRow(accountID, "90071992547409939999", location)
			require.NoError(t, err)
			require.Positive(t, rowID)
			_, err = insertRow(accountID, "90071992547409939999", location)
			requirePostgresCode(t, err, "23505")
			_, err = insertRow(otherID, "90071992547409939999", location)
			require.NoError(t, err)
			if table.location != "" {
				_, err = insertRow(accountID, "90071992547409939999", "region-b")
				require.NoError(t, err)
			}
			if table.name == "gcp_instances" {
				_, err = database.Exec(`INSERT INTO gcp_instances(cloud_account_id,instance_id,region,zone,last_seen_at) VALUES ($1,'90071992547409939999','region-a','other-zone',now())`, accountID)
				require.NoError(t, err)
				_, err = database.Exec(`INSERT INTO gcp_instances(cloud_account_id,instance_id,region,zone,last_seen_at) VALUES ($1,'90071992547409939999','other-region','region-a',now())`, accountID)
				requirePostgresCode(t, err, "23505")
			}
			if table.name == "aliyun_instances" {
				_, err = database.Exec(`INSERT INTO aliyun_instances(cloud_account_id,instance_id,region,zone,last_seen_at) VALUES ($1,'90071992547409939999','region-a','other-zone',now())`, accountID)
				requirePostgresCode(t, err, "23505")
			}
			if table.location == "" {
				_, err = database.Exec(fmt.Sprintf(`UPDATE %s SET region='invented-region' WHERE id=$1`, table.name), rowID)
				requirePostgresCode(t, err, "23514")
			}
			_, err = database.Exec(fmt.Sprintf(`UPDATE %s SET deleted_at=now() WHERE id=$1`, table.name), rowID)
			require.NoError(t, err)
			_, err = insertRow(accountID, "90071992547409939999", location)
			requirePostgresCode(t, err, "23505")
			_, err = insertRow(999999, "orphan", location)
			requirePostgresCode(t, err, "23503")
			_, err = database.Exec(`DELETE FROM cloud_accounts WHERE id=$1`, accountID)
			requirePostgresCode(t, err, "23503")
			for _, field := range append(append([]string{}, table.arrays...), "tags", "native_details") {
				require.Equal(t, "jsonb", columns[field].kind)
				wrong := `{}`
				if field == "tags" || field == "native_details" {
					wrong = `[]`
				}
				for _, bad := range []string{wrong, `null`, `"scalar"`} {
					_, err = database.Exec(fmt.Sprintf(`UPDATE %s SET %s=$1 WHERE id=$2`, table.name, field), bad, rowID)
					requirePostgresCode(t, err, "23514")
				}
				_, err = database.Exec(fmt.Sprintf(`UPDATE %s SET %s=NULL WHERE id=$1`, table.name, field), rowID)
				requirePostgresCode(t, err, "23502")
			}
			var indexes string
			require.NoError(t, database.QueryRow(`SELECT string_agg(indexdef,';') FROM pg_indexes WHERE schemaname='public' AND tablename=$1`, table.name).Scan(&indexes))
			require.Contains(t, indexes, "USING gin (tags)")
			require.Contains(t, indexes, "last_seen_at DESC, id DESC) WHERE (deleted_at IS NULL)")
			require.Contains(t, indexes, "(last_seen_at)")
			if table.id == "instance_id" {
				require.Contains(t, indexes, "USING gin (network_interfaces)")
			}
			if _, ok := columns["network_ref"]; ok {
				require.Contains(t, indexes, "network_ref)")
			}
		})
	}
}

func TestNativeModelsPreserveStructuredJSONAndNullableFacts(t *testing.T) {
	database, accountID, _ := nativeAssetDatabase(t)
	require.NoError(t, runGoose(database, "up"))
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: database}), &gorm.Config{})
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	endpoints := datatypes.JSON(`[{"address":"db.example.test","port":0,"type":"PRIVATE"},{"address":"fd00::1","port":null,"type":"PRIMARY"}]`)
	a := models.AliyunDatabase{CloudAccountID: uint64(accountID), DatabaseID: "rm-1", Region: "region-a", Endpoints: endpoints, LastSeenAt: now}
	g := models.GCPDatabase{CloudAccountID: uint64(accountID), DatabaseID: "sql-1", Region: "region-a", Endpoints: endpoints, LastSeenAt: now}
	require.Equal(t, "gcp_databases", g.TableName())
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&g).Error)
	var gotA models.AliyunDatabase
	var gotG models.GCPDatabase
	require.NoError(t, db.First(&gotA, a.ID).Error)
	require.NoError(t, db.First(&gotG, g.ID).Error)
	require.JSONEq(t, string(endpoints), string(gotA.Endpoints))
	require.JSONEq(t, string(endpoints), string(gotG.Endpoints))
	for _, facts := range []struct {
		storage *int64
		ha      *string
		public  *bool
	}{{gotA.StorageGB, gotA.HAMode, gotA.PubliclyAccessible}, {gotG.StorageGB, gotG.HAMode, gotG.PubliclyAccessible}} {
		require.Nil(t, facts.storage)
		require.Nil(t, facts.ha)
		require.Nil(t, facts.public)
	}
	require.Equal(t, "cloud_sql", gotG.Product)
	zero := int64(0)
	disabled := false
	ha := "single"
	require.NoError(t, db.Model(&g).Updates(map[string]any{"storage_gb": &zero, "publicly_accessible": &disabled, "ha_mode": &ha}).Error)
	require.NoError(t, db.First(&gotG, g.ID).Error)
	require.Equal(t, &zero, gotG.StorageGB)
	require.Equal(t, &disabled, gotG.PubliclyAccessible)
	require.Equal(t, &ha, gotG.HAMode)
	network := models.GCPNetwork{CloudAccountID: uint64(accountID), NetworkID: "90071992547409939999", NativeRef: "projects/host-project/global/networks/shared", LastSeenAt: now}
	require.NoError(t, db.Create(&network).Error)
	var gotNetwork models.GCPNetwork
	require.NoError(t, db.First(&gotNetwork, network.ID).Error)
	require.Equal(t, network.NativeRef, gotNetwork.NativeRef)
	require.Equal(t, network.NetworkID, gotNetwork.NetworkID)
	require.Empty(t, gotNetwork.Region)
}

func TestNativeModelsMapEveryFamilyToItsOwnSchema(t *testing.T) {
	database, accountID, _ := nativeAssetDatabase(t)
	require.NoError(t, runGoose(database, "up"))
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: database}), &gorm.Config{})
	require.NoError(t, err)
	cases := []struct {
		model           interface{ TableName() string }
		table, nativeID string
	}{
		{&models.AliyunInstance{}, "aliyun_instances", "InstanceID"},
		{&models.AliyunVPC{}, "aliyun_vpcs", "NetworkID"},
		{&models.AliyunVSwitch{}, "aliyun_vswitches", "SubnetID"},
		{&models.AliyunDatabase{}, "aliyun_databases", "DatabaseID"},
		{&models.AliyunSecurityGroup{}, "aliyun_security_groups", "FirewallID"},
		{&models.GCPInstance{}, "gcp_instances", "InstanceID"},
		{&models.GCPNetwork{}, "gcp_networks", "NetworkID"},
		{&models.GCPSubnet{}, "gcp_subnets", "SubnetID"},
		{&models.GCPDatabase{}, "gcp_databases", "DatabaseID"},
		{&models.GCPFirewall{}, "gcp_firewalls", "FirewallID"},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			require.Equal(t, tc.table, tc.model.TableName())
			fields := reflect.ValueOf(tc.model).Elem()
			fields.FieldByName("CloudAccountID").SetUint(uint64(accountID))
			fields.FieldByName(tc.nativeID).SetString("native-id")
			fields.FieldByName("LastSeenAt").Set(reflect.ValueOf(time.Now().UTC().Truncate(time.Microsecond)))
			if tc.table != "gcp_networks" && tc.table != "gcp_firewalls" {
				fields.FieldByName("Region").SetString("region-a")
			}
			if field := fields.FieldByName("CIDRBlocks"); field.IsValid() {
				field.Set(reflect.ValueOf(datatypes.JSON(`["10.0.0.0/24","2001:db8::/64"]`)))
			}
			if field := fields.FieldByName("NetworkRef"); field.IsValid() {
				field.SetString("projects/host-project/global/networks/shared")
			}
			if field := fields.FieldByName("NetworkInterfaces"); field.IsValid() {
				field.Set(reflect.ValueOf(datatypes.JSON(`[{"id":"nic","name":"primary","network_ref":"projects/host-project/global/networks/shared","subnet_ref":"subnet","private_ipv4":["10.0.0.1"],"public_ipv4":["192.0.2.1"],"private_ipv6":["fd00::1"],"public_ipv6":["2001:db8::1"]}]`)))
			}
			fields.FieldByName("NativeDetails").Set(reflect.ValueOf(datatypes.JSON(`{"native_feature":"retained"}`)))
			require.NoError(t, db.Create(tc.model).Error)
			require.Positive(t, fields.FieldByName("ID").Uint())
			got := reflect.New(fields.Type()).Interface()
			require.NoError(t, db.First(got, fields.FieldByName("ID").Uint()).Error)
			gotFields := reflect.ValueOf(got).Elem()
			for i := 0; i < fields.NumField(); i++ {
				field := fields.Type().Field(i).Name
				expected, actual := fields.Field(i).Interface(), gotFields.Field(i).Interface()
				switch value := expected.(type) {
				case datatypes.JSON:
					require.JSONEq(t, string(value), string(actual.(datatypes.JSON)), field)
				case time.Time:
					require.WithinDuration(t, value, actual.(time.Time), time.Microsecond, field)
				default:
					require.Equal(t, expected, actual, field)
				}
			}
		})
	}
}
