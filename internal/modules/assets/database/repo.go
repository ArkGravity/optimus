package database

import (
	"context"
	"math"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

type Repo struct {
	query *query.Repo
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{query: query.NewRepo(db)} }
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Summary, int64, error) {
	rows, total, err := r.query.ListDatabases(ctx, f.queryFilter())
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
	row, err := r.query.FindDatabase(ctx, key, includeDeleted)
	if err != nil {
		return nil, err
	}
	item := fromProjection(*row)
	return &item, nil
}
func fromProjection(row query.Database) Summary {
	item := Summary{
		Name:             row.Name,
		Product:          row.Product,
		Endpoints:        row.Endpoints,
		HAMode:           row.HAMode,
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
		DBInstanceID:     row.DBInstanceID,
		Engine:           row.Engine,
		EngineVersion:    row.EngineVersion,
		InstanceClass:    row.InstanceClass,
		Status:           row.Status,
		Endpoint:         row.Endpoint,
		Port:             row.Port,
		Tags:             row.Tags,
		LastSeenAt:       row.LastSeenAt,
		Deleted:          row.Deleted,
	}
	item.multiAZ = row.MultiAZ
	item.publiclyAccessible = row.PubliclyAccessible
	item.storageGB = row.StorageGB
	if row.MultiAZ != nil {
		item.MultiAZ = *row.MultiAZ
	}
	if row.PubliclyAccessible != nil {
		item.PubliclyAccessible = *row.PubliclyAccessible
	}
	if row.StorageGB != nil && *row.StorageGB >= math.MinInt32 && *row.StorageGB <= math.MaxInt32 {
		v := int32(*row.StorageGB)
		item.StorageGB = &v
	}
	return item
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
		Engine:         f.Engine,
		Status:         f.Status,
	}
}
