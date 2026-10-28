package sync

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/sync/runs"
)

func (e *Engine) sweepSecurityGroups(ctx context.Context, account models.CloudAccount, region, trigger string, triggeredBy *uint64) error {
	runID, err := e.insertRun(ctx, account.ID, region, "security_group", trigger, triggeredBy)
	if err != nil {
		return err
	}
	clients, err := e.clientsForSweep(ctx, account.ID, account.CloudKeyID, region, "security_group", trigger, triggeredBy == nil)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	started := time.Now()
	items, err := e.fetcher.FetchSecurityGroups(ctx, clients)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	deleted, err := UpsertSecurityGroups(ctx, e.db, account.ID, region, started, items)
	if err != nil {
		return e.finishPersistence(ctx, runID, err)
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "success", ItemsSeen: saturatingInt32(int64(len(items))), ItemsSoftDeleted: saturatingInt32(deleted)})
}

func UpsertSecurityGroups(ctx context.Context, db *gorm.DB, accountID uint64, region string, started time.Time, items []models.AWSSecurityGroup) (int64, error) {
	var deleted int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEligibleAccount(tx, accountID, region); err != nil {
			return err
		}
		for i := range items {
			items[i].ID = 0
			items[i].CloudAccountID, items[i].Region, items[i].LastSeenAt = accountID, region, started
			items[i].DeletedAt = gorm.DeletedAt{}
		}
		if len(items) > 0 {
			conflict := clause.OnConflict{
				Columns:   []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "group_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"group_name", "name", "description", "vpc_id", "owner_id", "inbound_rules", "outbound_rules", "tags", "last_seen_at", "updated_at", "deleted_at"}),
			}
			if err := tx.Clauses(conflict).CreateInBatches(&items, sweepBatchSize).Error; err != nil {
				return err
			}
		}
		result := tx.Where("cloud_account_id = ? AND region = ? AND last_seen_at < ?", accountID, region, started).Delete(&models.AWSSecurityGroup{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}
