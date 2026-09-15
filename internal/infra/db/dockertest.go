//go:build dbtest

package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// StartTestPostgres uses the dedicated shared test server when configured,
// otherwise it boots an ephemeral Postgres container. It
// runs migrations under migrationsDir, returns the GORM DB and a teardown
// function. Caller must defer teardown().
func StartTestPostgres(t *testing.T, migrationsDir string) (*gorm.DB, func()) {
	t.Helper()
	if dsn := os.Getenv("OPTIMUS_TEST_POSTGRES_DSN"); dsn != "" {
		return startSharedTestPostgres(t, dsn, migrationsDir)
	}
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("dockertest pool: %v", err)
	}

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)

	pool.MaxWait = 60 * time.Second
	var gdb *gorm.DB
	if err := pool.Retry(func() error {
		var openErr error
		gdb, openErr = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if openErr != nil {
			return openErr
		}
		return Ping(context.Background(), gdb)
	}); err != nil {
		_ = pool.Purge(res)
		t.Fatalf("connect postgres: %v", err)
	}

	sqlDB, _ := gdb.DB()
	migrationsAbs, _ := filepath.Abs(migrationsDir)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, migrationsAbs); err != nil {
		_ = pool.Purge(res)
		t.Fatal(err)
	}

	teardown := func() { _ = pool.Purge(res) }
	return gdb, teardown
}

// StartSharedTestDatabase creates an empty isolated database on a dedicated
// test server. Cleanup is automatic and may also be called explicitly.
func StartSharedTestDatabase(t *testing.T, dsn string) (*sql.DB, func()) {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("optimus_test_%x", randomDatabaseID(t))
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec("CREATE DATABASE " + quoted); err != nil {
		t.Fatal(err)
	}
	var testDB *sql.DB
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if testDB != nil {
				_ = testDB.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := admin.ExecContext(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
				t.Errorf("drop test database: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)
	cfg.Database = name
	testDB = stdlib.OpenDB(*cfg)
	return testDB, cleanup
}

func startSharedTestPostgres(t *testing.T, dsn, migrationsDir string) (*gorm.DB, func()) {
	t.Helper()
	testDB, cleanup := StartSharedTestDatabase(t, dsn)

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: testDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Goose dialect configuration is global within a test binary.
	migrationMu.Lock()
	defer migrationMu.Unlock()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(testDB, migrationsDir); err != nil {
		t.Fatal(err)
	}
	return gdb, cleanup
}

var migrationMu sync.Mutex

func randomDatabaseID(t *testing.T) []byte {
	t.Helper()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return id
}
