//go:build dbtest

package securitygroup

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestSecurityGroupRepositoryFiltersPaginationAndHistoricalDetail(t *testing.T) {
	gdb, cleanup := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(cleanup)
	key := dbtest.SeedCloudKey(t, gdb, "sg-key")
	first := dbtest.SeedCloudAccount(t, gdb, key.ID, "first", "us-east-1")
	second := dbtest.SeedCloudAccount(t, gdb, key.ID, "second", "eu-west-1")
	rows := []models.AWSSecurityGroup{
		{CloudAccountID: first.ID, Region: "us-east-1", GroupID: "sg-1", GroupName: "web", VPCID: "vpc-1", Description: "HTTPS office", Tags: datatypes.JSON(`{"env":"prod"}`), InboundRules: datatypes.JSON(`[{"protocol":"tcp","from_port":443,"to_port":443,"target_type":"ipv4","target":"10.0.0.0/8","description":"office"}]`), LastSeenAt: time.Now()},
		{CloudAccountID: first.ID, Region: "us-east-1", GroupID: "sg-2", GroupName: "old", VPCID: "vpc-2", LastSeenAt: time.Now()},
		{CloudAccountID: second.ID, Region: "eu-west-1", GroupID: "sg-1", VPCID: "vpc-1", LastSeenAt: time.Now()},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	require.NoError(t, gdb.Delete(&rows[1]).Error)
	repo := NewRepo(gdb)
	items, total, err := repo.List(t.Context(), ListQuery{AccountID: first.ID, Region: "us-east-1", VPCID: "vpc-1", Q: "HTTPS", Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "first", items[0].CloudAccountName)
	require.Equal(t, 1, items[0].InboundRuleCount)
	require.Zero(t, items[0].OutboundRuleCount)
	items, total, err = repo.List(t.Context(), ListQuery{AccountID: first.ID, IncludeDeleted: true, Page: 2, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 1)
	require.Equal(t, rows[1].ID, items[0].ID)
	require.True(t, items[0].Deleted)
	detail, err := repo.Get(t.Context(), rows[0].ID)
	require.NoError(t, err)
	require.Equal(t, "prod", detail.Tags["env"])
	require.Equal(t, int32(443), *detail.InboundRules[0].FromPort)
	require.NotNil(t, detail.OutboundRules)
	require.NoError(t, gdb.Delete(first).Error)
	detail, err = repo.Get(t.Context(), rows[1].ID)
	require.NoError(t, err)
	require.True(t, detail.Deleted)
	require.Equal(t, "first", detail.CloudAccountName)
	_, err = repo.Get(t.Context(), 99999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
