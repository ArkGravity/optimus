package sync

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ArkGravity/optimus/internal/models"
)

func UpsertSecurityGroups(ctx context.Context, db *gorm.DB, accountID uint64, region string, started time.Time, items []models.AWSSecurityGroup) (int64, error) {
	var deleted int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEligibleAccount(tx, accountID, region); err != nil {
			return err
		}
		var err error
		deleted, err = UpsertSecurityGroupsTx(ctx, tx, accountID, region, started, items)
		return err
	})
	return deleted, err
}

// UpsertSecurityGroupsTx publishes inside the caller-owned transaction. The caller must lock the collection scope first.
func UpsertSecurityGroupsTx(ctx context.Context, tx *gorm.DB, accountID uint64, region string, started time.Time, items []models.AWSSecurityGroup) (int64, error) {
	tx = tx.WithContext(ctx)
	var deleted int64
	err := func() error {
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
	}()
	return deleted, err
}
