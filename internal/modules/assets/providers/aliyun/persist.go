// Package aliyun persists provider-native complete asset snapshots.
package aliyun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const batchSize = 500

// All publishers require the caller's active transaction. Only complete,
// successful snapshots may be passed here; the caller must roll back on error.
// Models contain mapped, allowlisted native details, never raw SDK metadata.
// A fresh session retains the transaction while discarding inherited clauses;
// its cloned Statement also clears Unscoped even when propagation is enabled.
// PublishInstances publishes and prunes one complete region snapshot in tx.
func PublishInstances(ctx context.Context, tx *gorm.DB, scope account.Scope, location resource.Location, sweepStart time.Time, items []models.AliyunInstance) (seen, softDeleted int64, err error) {
	if scope.Provider != "aliyun" || location.Type != "region" || sweepStart.IsZero() {
		return 0, 0, account.ErrScopeIneligible
	}
	if _, err := account.LockScopeTx(ctx, tx, scope, location); err != nil {
		return 0, 0, err
	}
	tx = tx.Session(&gorm.Session{NewDB: true, Context: ctx, SkipDefaultTransaction: true})
	tx.Statement.Unscoped = false
	items = append([]models.AliyunInstance(nil), items...)
	for i := range items {
		row := &items[i]
		row.ID = 0
		row.CloudAccountID = scope.AccountID
		row.Region = location.Region
		row.LastSeenAt = sweepStart
		row.CreatedAt = sweepStart
		row.UpdatedAt = sweepStart
		row.DeletedAt = gorm.DeletedAt{}
		if err := normalizeJSON(&row.NetworkInterfaces, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.Tags, '{'); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.NativeDetails, '{'); err != nil {
			return 0, 0, err
		}
	}
	if len(items) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "instance_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "instance_type", "native_status", "zone", "network_interfaces", "tags", "native_details", "last_seen_at", "updated_at", "deleted_at"})}).CreateInBatches(&items, batchSize).Error; err != nil {
			return 0, 0, err
		}
	}
	result := tx.Where("cloud_account_id = ? AND last_seen_at < ?", scope.AccountID, sweepStart)
	result = result.Where("region = ?", location.Region)
	result = result.Delete(&models.AliyunInstance{})
	if result.Error != nil {
		return 0, 0, result.Error
	}
	return int64(len(items)), result.RowsAffected, nil
}

// PublishNetwork publishes and prunes one complete region snapshot in tx.
// Counts include both VPCs and vSwitches.
func PublishNetwork(ctx context.Context, tx *gorm.DB, scope account.Scope, location resource.Location, sweepStart time.Time, items []models.AliyunVPC, switches []models.AliyunVSwitch) (seen, softDeleted int64, err error) {
	if scope.Provider != "aliyun" || location.Type != "region" || sweepStart.IsZero() {
		return 0, 0, account.ErrScopeIneligible
	}
	if _, err := account.LockScopeTx(ctx, tx, scope, location); err != nil {
		return 0, 0, err
	}
	tx = tx.Session(&gorm.Session{NewDB: true, Context: ctx, SkipDefaultTransaction: true})
	tx.Statement.Unscoped = false
	items = append([]models.AliyunVPC(nil), items...)
	for i := range items {
		row := &items[i]
		row.ID = 0
		row.CloudAccountID = scope.AccountID
		row.Region = location.Region
		row.LastSeenAt = sweepStart
		row.CreatedAt = sweepStart
		row.UpdatedAt = sweepStart
		row.DeletedAt = gorm.DeletedAt{}
		if err := normalizeJSON(&row.CIDRBlocks, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.Tags, '{'); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.NativeDetails, '{'); err != nil {
			return 0, 0, err
		}
	}
	switches = append([]models.AliyunVSwitch(nil), switches...)
	for i := range switches {
		row := &switches[i]
		row.ID = 0
		row.CloudAccountID = scope.AccountID
		row.Region = location.Region
		row.LastSeenAt = sweepStart
		row.CreatedAt = sweepStart
		row.UpdatedAt = sweepStart
		row.DeletedAt = gorm.DeletedAt{}
		if err := normalizeJSON(&row.CIDRBlocks, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.Tags, '{'); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.NativeDetails, '{'); err != nil {
			return 0, 0, err
		}
	}
	if len(items) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "network_id"}}, DoUpdates: clause.AssignmentColumns([]string{"native_ref", "name", "cidr_blocks", "native_status", "tags", "native_details", "last_seen_at", "updated_at", "deleted_at"})}).CreateInBatches(&items, batchSize).Error; err != nil {
			return 0, 0, err
		}
	}
	if len(switches) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "subnet_id"}}, DoUpdates: clause.AssignmentColumns([]string{"network_ref", "name", "zone", "cidr_blocks", "tags", "native_details", "last_seen_at", "updated_at", "deleted_at"})}).CreateInBatches(&switches, batchSize).Error; err != nil {
			return 0, 0, err
		}
	}
	result := tx.Where("cloud_account_id = ? AND last_seen_at < ?", scope.AccountID, sweepStart)
	result = result.Where("region = ?", location.Region)
	result = result.Delete(&models.AliyunVPC{})
	if result.Error != nil {
		return 0, 0, result.Error
	}
	childResult := tx.Where("cloud_account_id = ? AND last_seen_at < ?", scope.AccountID, sweepStart)
	childResult = childResult.Where("region = ?", location.Region)
	childResult = childResult.Delete(&models.AliyunVSwitch{})
	if childResult.Error != nil {
		return 0, 0, childResult.Error
	}
	return int64(len(items) + len(switches)), result.RowsAffected + childResult.RowsAffected, nil
}

// PublishDatabases publishes and prunes one complete region snapshot in tx.
func PublishDatabases(ctx context.Context, tx *gorm.DB, scope account.Scope, location resource.Location, sweepStart time.Time, items []models.AliyunDatabase) (seen, softDeleted int64, err error) {
	if scope.Provider != "aliyun" || location.Type != "region" || sweepStart.IsZero() {
		return 0, 0, account.ErrScopeIneligible
	}
	if _, err := account.LockScopeTx(ctx, tx, scope, location); err != nil {
		return 0, 0, err
	}
	tx = tx.Session(&gorm.Session{NewDB: true, Context: ctx, SkipDefaultTransaction: true})
	tx.Statement.Unscoped = false
	items = append([]models.AliyunDatabase(nil), items...)
	for i := range items {
		row := &items[i]
		row.ID = 0
		row.CloudAccountID = scope.AccountID
		row.Region = location.Region
		row.LastSeenAt = sweepStart
		row.CreatedAt = sweepStart
		row.UpdatedAt = sweepStart
		row.DeletedAt = gorm.DeletedAt{}
		if err := normalizeJSON(&row.Endpoints, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.Tags, '{'); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.NativeDetails, '{'); err != nil {
			return 0, 0, err
		}
	}
	if len(items) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "database_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "product", "engine", "engine_version", "instance_class", "native_status", "zone", "endpoints", "storage_gb", "ha_mode", "publicly_accessible", "tags", "native_details", "last_seen_at", "updated_at", "deleted_at"})}).CreateInBatches(&items, batchSize).Error; err != nil {
			return 0, 0, err
		}
	}
	result := tx.Where("cloud_account_id = ? AND last_seen_at < ?", scope.AccountID, sweepStart)
	result = result.Where("region = ?", location.Region)
	result = result.Delete(&models.AliyunDatabase{})
	if result.Error != nil {
		return 0, 0, result.Error
	}
	return int64(len(items)), result.RowsAffected, nil
}

// PublishSecurityGroups publishes and prunes one complete region snapshot in tx.
func PublishSecurityGroups(ctx context.Context, tx *gorm.DB, scope account.Scope, location resource.Location, sweepStart time.Time, items []models.AliyunSecurityGroup) (seen, softDeleted int64, err error) {
	if scope.Provider != "aliyun" || location.Type != "region" || sweepStart.IsZero() {
		return 0, 0, account.ErrScopeIneligible
	}
	if _, err := account.LockScopeTx(ctx, tx, scope, location); err != nil {
		return 0, 0, err
	}
	tx = tx.Session(&gorm.Session{NewDB: true, Context: ctx, SkipDefaultTransaction: true})
	tx.Statement.Unscoped = false
	items = append([]models.AliyunSecurityGroup(nil), items...)
	for i := range items {
		row := &items[i]
		row.ID = 0
		row.CloudAccountID = scope.AccountID
		row.Region = location.Region
		row.LastSeenAt = sweepStart
		row.CreatedAt = sweepStart
		row.UpdatedAt = sweepStart
		row.DeletedAt = gorm.DeletedAt{}
		if err := normalizeJSON(&row.InboundRules, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.OutboundRules, '['); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.Tags, '{'); err != nil {
			return 0, 0, err
		}
		if err := normalizeJSON(&row.NativeDetails, '{'); err != nil {
			return 0, 0, err
		}
	}
	if len(items) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "firewall_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "network_ref", "description", "inbound_rules", "outbound_rules", "tags", "native_details", "last_seen_at", "updated_at", "deleted_at"})}).CreateInBatches(&items, batchSize).Error; err != nil {
			return 0, 0, err
		}
	}
	result := tx.Where("cloud_account_id = ? AND last_seen_at < ?", scope.AccountID, sweepStart)
	result = result.Where("region = ?", location.Region)
	result = result.Delete(&models.AliyunSecurityGroup{})
	if result.Error != nil {
		return 0, 0, result.Error
	}
	return int64(len(items)), result.RowsAffected, nil
}

// normalizeJSON preserves SQL collection shapes and never includes payloads in errors.
func normalizeJSON(value *datatypes.JSON, shape byte) error {
	raw := bytes.TrimSpace(*value)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		if shape == '[' {
			*value = datatypes.JSON("[]")
		} else {
			*value = datatypes.JSON("{}")
		}
		return nil
	}
	if !json.Valid(raw) || raw[0] != shape {
		return errors.New("invalid native asset JSON collection")
	}
	return nil
}
