//go:build dbtest

package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSharedPostgresIsolationAndCleanup(t *testing.T) {
	if os.Getenv("OPTIMUS_TEST_POSTGRES_DSN") == "" {
		t.Skip("requires shared test server")
	}
	dir := filepath.Join("..", "..", "..", "migrations")
	first, cleanupFirst := StartTestPostgres(t, dir)
	second, cleanupSecond := StartTestPostgres(t, dir)
	defer cleanupSecond()
	var firstName, secondName string
	require.NoError(t, first.Raw("SELECT current_database()").Scan(&firstName).Error)
	require.NoError(t, second.Raw("SELECT current_database()").Scan(&secondName).Error)
	require.NotEqual(t, firstName, secondName)
	require.NoError(t, first.Exec("CREATE TABLE isolation_probe (id integer)").Error)
	var exists bool
	require.NoError(t, second.Raw("SELECT to_regclass('public.isolation_probe') IS NOT NULL").Scan(&exists).Error)
	require.False(t, exists)
	cleanupFirst()
	cleanupFirst()
	var count int64
	require.NoError(t, second.Raw("SELECT count(*) FROM pg_database WHERE datname = ?", firstName).Scan(&count).Error)
	require.Zero(t, count)
}
