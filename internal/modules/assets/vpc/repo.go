package vpc

import (
	"context"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

type Repo struct {
	db    *gorm.DB
	query *query.Repo
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db, query: query.NewRepo(db)} }
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Summary, int64, error) {
	rows, total, err := r.query.ListVPCs(ctx, f.queryFilter())
	if err != nil {
		return nil, 0, err
	}
	items := make([]Summary, len(rows))
	for i, row := range rows {
		items[i] = fromProjection(row)
	}
	return items, total, nil
}
func (r *Repo) FindByKey(ctx context.Context, key string, includeDeleted bool) (*Summary, error) {
	row, err := r.query.FindVPC(ctx, key, includeDeleted)
	if err != nil {
		return nil, err
	}
	item := fromProjection(*row)
	return &item, nil
}
func fromProjection(row query.VPC) Summary {
	item := Summary{
		NetworkRef:       row.NetworkRef,
		CIDRBlocks:       row.CIDRBlocks,
		NativeDetails:    row.NativeDetails,
		AssetKey:         row.AssetKey,
		Provider:         row.Provider,
		ProjectID:        row.ProjectID,
		LocationType:     row.LocationType,
		Location:         row.Location,
		Zone:             row.Zone,
		NativeID:         row.NativeID,
		NativeStatus:     row.NativeStatus,
		ID:               row.ID,
		CloudAccountID:   row.CloudAccountID,
		CloudAccountName: row.CloudAccountName,
		Region:           row.Region,
		VPCID:            row.VPCID,
		Name:             row.Name,
		CIDRBlock:        row.CIDRBlock,
		State:            row.State,
		Tags:             row.Tags,
		LastSeenAt:       row.LastSeenAt,
		Deleted:          row.Deleted,
	}
	item.isDefault = row.IsDefault
	if row.IsDefault != nil {
		item.IsDefault = *row.IsDefault
	}
	return item
}
func (r *Repo) FindByID(ctx context.Context, id uint64) (*models.AWSVPC, error) {
	var row models.AWSVPC
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
func (r *Repo) ListSubnets(ctx context.Context, f SubnetListFilter) ([]SubnetSummary, int64, error) {
	rows, total, err := r.query.ListSubnets(ctx, query.Filter{
		Provider:       "aws",
		AccountID:      f.CloudAccountID,
		Region:         f.Region,
		NetworkRef:     f.VPCID,
		Q:              f.Q,
		IncludeDeleted: f.IncludeDeleted,
		Page:           f.Page,
		Size:           f.Size,
	})
	return subnetResults(rows, total, err)
}
func (r *Repo) ListSubnetsByVPC(ctx context.Context, key string, f ListFilter) ([]SubnetSummary, int64, error) {
	rows, total, err := r.query.ListSubnetsByVPC(ctx, key, f.queryFilter())
	return subnetResults(rows, total, err)
}
func subnetResults(rows []query.Subnet, total int64, err error) ([]SubnetSummary, int64, error) {
	if err != nil {
		return nil, 0, err
	}
	items := make([]SubnetSummary, len(rows))
	for i, row := range rows {
		items[i] = subnetFromProjection(row)
	}
	return items, total, nil
}
func subnetFromProjection(row query.Subnet) SubnetSummary {
	return SubnetSummary{
		CloudAccountName: row.CloudAccountName,
		AssetKey:         row.AssetKey,
		Provider:         row.Provider,
		ProjectID:        row.ProjectID,
		LocationType:     row.LocationType,
		Location:         row.Location,
		Zone:             row.Zone,
		NativeID:         row.NativeID,
		NativeStatus:     row.NativeStatus,
		NetworkRef:       row.NetworkRef,
		CIDRBlocks:       row.CIDRBlocks,
		NativeDetails:    row.NativeDetails,
		ID:               row.ID,
		CloudAccountID:   row.CloudAccountID,
		Region:           row.Region,
		SubnetID:         row.SubnetID,
		VPCID:            row.VPCID,
		Name:             row.Name,
		CIDRBlock:        row.CIDRBlock,
		AvailabilityZone: row.AvailabilityZone,
		Tags:             row.Tags,
		LastSeenAt:       row.LastSeenAt,
		Deleted:          row.Deleted,
	}
}
func (f ListFilter) queryFilter() query.Filter {
	return query.Filter{
		Provider:       f.Provider,
		AccountID:      f.AccountID,
		ProjectID:      f.ProjectID,
		Region:         f.Region,
		Zone:           f.Zone,
		Q:              f.Q,
		IncludeDeleted: f.IncludeDeleted,
		Page:           f.Page,
		Size:           f.Size,
	}
}
