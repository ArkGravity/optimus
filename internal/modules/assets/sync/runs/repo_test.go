//go:build dbtest

package runs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

func TestRepoInsertFinishAndPrune(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "runs-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "runs-account", "us-east-1")
	repo := NewRepo(gdb)

	id, err := repo.Insert(context.Background(), InsertRequest{
		CloudAccountID: account.ID,
		Region:         "us-east-1",
		ResourceType:   "instance",
		Trigger:        "test",
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	require.NoError(t, repo.Finish(context.Background(), id, FinishRequest{
		Status:           "success",
		ItemsSeen:        2,
		ItemsSoftDeleted: 1,
	}))
	var row models.AssetsSyncRun
	require.NoError(t, gdb.First(&row, id).Error)
	require.NotNil(t, row.FinishedAt)
	require.Equal(t, "success", row.Status)
	require.Equal(t, "aws", row.Provider)
	require.Equal(t, "region", row.LocationType)
	require.Equal(t, "us-east-1", row.Location)
	require.EqualValues(t, 1, row.ScopeRevision)
	require.EqualValues(t, 2, row.ItemsSeen)
	require.EqualValues(t, 1, row.ItemsSoftDeleted)
	require.Error(t, repo.Finish(context.Background(), id, FinishRequest{Status: "failed"}))
	require.Error(t, repo.Finish(context.Background(), id+999, FinishRequest{Status: "failed"}))

	require.NoError(t, gdb.Model(&row).Update("started_at", time.Now().AddDate(0, 0, -31)).Error)
	pruned, err := repo.Prune(context.Background(), 30)
	require.NoError(t, err)
	require.EqualValues(t, 1, pruned)
}

func TestRepoScopeDefaultsUseAuthoritativeDeletedAccountAndFilters(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "scope-default-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "scope-default-account", "us-east-1")
	require.NoError(t, gdb.Model(account).Updates(map[string]any{"provider": "aliyun", "scope_revision": 7}).Error)
	require.NoError(t, gdb.Delete(account).Error)
	repo := NewRepo(gdb)
	id, err := repo.Insert(context.Background(), InsertRequest{CloudAccountID: account.ID, Region: "us-east-1", ResourceType: "instance", Trigger: "test"})
	require.NoError(t, err)
	require.NoError(t, repo.Finish(context.Background(), id, FinishRequest{Status: "skipped"}))
	items, total, err := repo.List(context.Background(), ListFilter{Provider: "aliyun", LocationType: "region", Location: "us-east-1", Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "aliyun", items[0].Provider)
	require.EqualValues(t, 7, items[0].ScopeRevision)
	for _, filter := range []ListFilter{{Provider: "aws"}, {LocationType: "global"}, {Location: "other"}} {
		filter.Page, filter.Size = 1, 20
		items, total, err := repo.List(context.Background(), filter)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, items)
	}
}

func TestRepoListFiltersOrdersPaginatesAndJoinsSoftDeletedAccount(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "list-key")
	first := dbtest.SeedCloudAccount(t, gdb, key.ID, "first-account", "us-east-1")
	second := dbtest.SeedCloudAccount(t, gdb, key.ID, "second-account", "eu-west-1")
	base := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	rows := []models.AssetsSyncRun{
		{CloudAccountID: first.ID, Region: "us-east-1", Provider: "aws", LocationType: "region", Location: "us-east-1", ScopeRevision: 1, ResourceType: "instance", StartedAt: base, Status: "success", Trigger: "cron"},
		{CloudAccountID: first.ID, Region: "us-east-1", Provider: "aws", LocationType: "region", Location: "us-east-1", ScopeRevision: 1, ResourceType: "instance", StartedAt: base.Add(time.Minute), Status: "success", ItemsSoftDeleted: 4, Trigger: "manual"},
		{CloudAccountID: first.ID, Region: "us-east-1", Provider: "aws", LocationType: "region", Location: "us-east-1", ScopeRevision: 1, ResourceType: "network", StartedAt: base.Add(2 * time.Minute), Status: "failed", Trigger: "cron"},
		{CloudAccountID: second.ID, Region: "eu-west-1", Provider: "aws", LocationType: "region", Location: "eu-west-1", ScopeRevision: 1, ResourceType: "database", StartedAt: base.Add(3 * time.Minute), Status: "running", Trigger: "test"},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	// Same timestamp proves the ID tiebreaker is stable.
	tied := models.AssetsSyncRun{CloudAccountID: first.ID, Region: "us-east-1", Provider: "aws", LocationType: "region", Location: "us-east-1", ScopeRevision: 1, ResourceType: "instance", StartedAt: base.Add(time.Minute), Status: "success", Trigger: "cron"}
	require.NoError(t, gdb.Create(&tied).Error)
	require.NoError(t, gdb.Delete(first).Error)

	repo := NewRepo(gdb)
	startedAfter := base.Add(time.Minute)
	items, total, err := repo.List(context.Background(), ListFilter{
		AccountID: first.ID, ResourceType: "instance", Status: "success", StartedAfter: &startedAfter, Page: 1, Size: 1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 1)
	require.Equal(t, tied.ID, items[0].ID)
	require.Equal(t, "first-account", items[0].CloudAccountName)

	filterCases := []struct {
		name   string
		filter ListFilter
		wantID uint64
	}{
		{name: "account", filter: ListFilter{AccountID: second.ID, Page: 1, Size: 20}, wantID: rows[3].ID},
		{name: "resource type", filter: ListFilter{ResourceType: "network", Page: 1, Size: 20}, wantID: rows[2].ID},
		{name: "status", filter: ListFilter{Status: "failed", Page: 1, Size: 20}, wantID: rows[2].ID},
		{name: "started after", filter: ListFilter{StartedAfter: &rows[3].StartedAt, Page: 1, Size: 20}, wantID: rows[3].ID},
	}
	for _, test := range filterCases {
		t.Run(test.name, func(t *testing.T) {
			filtered, filteredTotal, listErr := repo.List(context.Background(), test.filter)
			require.NoError(t, listErr)
			require.EqualValues(t, 1, filteredTotal)
			require.Len(t, filtered, 1)
			require.Equal(t, test.wantID, filtered[0].ID)
		})
	}

	items, total, err = repo.List(context.Background(), ListFilter{AccountID: first.ID, Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
	require.Len(t, items, 4)
	require.Equal(t, []uint64{rows[2].ID, tied.ID, rows[1].ID, rows[0].ID}, []uint64{
		items[0].ID, items[1].ID, items[2].ID, items[3].ID,
	})
	require.EqualValues(t, 4, items[2].ItemsSoftDeleted)
}
