package securitygroup

import (
	"context"
	"encoding/json"

	"github.com/ArkGravity/optimus/internal/models"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

const projection = `aws_security_groups.id, aws_security_groups.cloud_account_id, cloud_accounts.name AS cloud_account_name,
aws_security_groups.region, group_id, group_name, aws_security_groups.name, aws_security_groups.description,
vpc_id, owner_id, jsonb_array_length(inbound_rules) AS inbound_rule_count,
jsonb_array_length(outbound_rules) AS outbound_rule_count, last_seen_at, aws_security_groups.deleted_at IS NOT NULL AS deleted`

func (r *Repo) List(ctx context.Context, q ListQuery) ([]Summary, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.AWSSecurityGroup{})
	if q.IncludeDeleted {
		query = query.Unscoped()
	}
	if q.AccountID != 0 {
		query = query.Where("aws_security_groups.cloud_account_id = ?", q.AccountID)
	}
	if q.Region != "" {
		query = query.Where("aws_security_groups.region = ?", q.Region)
	}
	if q.VPCID != "" {
		query = query.Where("vpc_id = ?", q.VPCID)
	}
	if q.Q != "" {
		term := "%" + q.Q + "%"
		query = query.Where("aws_security_groups.name ILIKE ? OR group_name ILIKE ? OR group_id ILIKE ? OR aws_security_groups.description ILIKE ?", term, term, term, term)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Summary, 0)
	err := query.Select(projection).
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_security_groups.cloud_account_id").
		Order("aws_security_groups.deleted_at IS NULL DESC, last_seen_at DESC, aws_security_groups.id DESC").
		Offset((q.Page - 1) * q.Size).Limit(q.Size).Scan(&items).Error
	return items, total, err
}

func (r *Repo) Get(ctx context.Context, id uint64) (*Detail, error) {
	var row struct {
		Summary
		RawTags     []byte `gorm:"column:tags"`
		RawInbound  []byte `gorm:"column:inbound_rules"`
		RawOutbound []byte `gorm:"column:outbound_rules"`
	}
	result := r.db.WithContext(ctx).Unscoped().Model(&models.AWSSecurityGroup{}).
		Select(projection+", tags, inbound_rules, outbound_rules").Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_security_groups.cloud_account_id").
		Where("aws_security_groups.id = ?", id).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	detail := &Detail{Summary: row.Summary, Tags: map[string]string{}, InboundRules: []Rule{}, OutboundRules: []Rule{}}
	for _, field := range []struct {
		raw []byte
		dst any
	}{
		{row.RawTags, &detail.Tags}, {row.RawInbound, &detail.InboundRules}, {row.RawOutbound, &detail.OutboundRules},
	} {
		if err := json.Unmarshal(field.raw, field.dst); err != nil {
			return nil, err
		}
	}
	return detail, nil
}
