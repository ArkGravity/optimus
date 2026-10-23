//go:build dbtest

package main

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"

	testdb "github.com/ArkGravity/optimus/internal/infra/db"
)

func TestRunGoose_UpAppliesAllMigrations(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))

	var maxVersion int64
	require.NoError(t,
		db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&maxVersion))
	require.Equal(t, int64(21), maxVersion, "fresh baseline must end at 00021")

	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name")
	require.NoError(t, err)
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	require.ElementsMatch(t, []string{
		"users", "roles", "permissions", "user_roles", "role_permissions", "menus", "refresh_tokens", "audit_logs",
		"credentials_ssh_keys", "credentials_kubeconfigs", "credentials_cloud_keys", "clusters",
		"apps_chart_repos", "apps_applications", "goose_db_version",
		"cloud_accounts", "aws_instances", "aws_vpcs", "aws_subnets", "aws_databases", "assets_sync_runs",
	}, tables)

}

func TestRunGoose_UpIsIdempotent(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))
	// Second invocation must succeed without changing anything.
	require.NoError(t, runGoose(db, "up"))
}

func TestRunGoose_RejectsUnknownDirection(t *testing.T) {
	// No DB needed — runGoose returns the error before touching the DB.
	err := runGoose(nil, "sideways")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown direction")
}

// startRawPostgres boots a clean Postgres container with NO migrations
// pre-applied. Distinct from internal/infra/db.StartTestPostgres which
// auto-migrates — we explicitly want a virgin DB so runGoose has work to do.
func startRawPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	if dsn := os.Getenv("OPTIMUS_TEST_POSTGRES_DSN"); dsn != "" {
		return testdb.StartSharedTestDatabase(t, dsn)
	}
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "17-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err)

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)
	pool.MaxWait = 60 * time.Second

	var db *sql.DB
	require.NoError(t, pool.Retry(func() error {
		var openErr error
		db, openErr = sql.Open("pgx", dsn)
		if openErr != nil {
			return openErr
		}
		return db.Ping()
	}))

	return db, func() {
		_ = db.Close()
		_ = pool.Purge(res)
	}
}
