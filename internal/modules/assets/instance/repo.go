package instance

import (
	"context"
	"encoding/json"
	"net"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

type Repo struct {
	query *query.Repo
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{query: query.NewRepo(db)} }
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Summary, int64, error) {
	rows, total, err := r.query.ListInstances(ctx, f.queryFilter())
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
	row, err := r.query.FindInstance(ctx, key, includeDeleted)
	if err != nil {
		return nil, err
	}
	item := fromProjection(*row)
	return &item, nil
}
func fromProjection(row query.Instance) Summary {
	item := Summary{
		NetworkInterfaces: row.NetworkInterfaces,
		NativeDetails:     row.NativeDetails,
		AssetKey:          row.AssetKey,
		Provider:          row.Provider,
		ProjectID:         row.ProjectID,
		LocationType:      row.LocationType,
		Location:          row.Location,
		Zone:              row.Zone,
		NativeID:          row.NativeID,
		NativeStatus:      row.NativeStatus,
		ID:                row.ID,
		CloudAccountID:    row.CloudAccountID,
		CloudAccountName:  row.CloudAccountName,
		Region:            row.Region,
		InstanceID:        row.InstanceID,
		Name:              row.Name,
		InstanceType:      row.InstanceType,
		State:             row.State,
		PrivateIP:         row.PrivateIP,
		PublicIP:          row.PublicIP,
		VPCID:             row.VPCID,
		SubnetID:          row.SubnetID,
		AvailabilityZone:  row.AvailabilityZone,
		LaunchTime:        row.LaunchTime,
		Tags:              row.Tags,
		LastSeenAt:        row.LastSeenAt,
		Deleted:           row.Deleted,
	}
	item.PrivateIP = normalizedIP(item.PrivateIP)
	item.PublicIP = normalizedIP(item.PublicIP)
	return item
}
func normalizedIP(value string) string {
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return value
}

func toSummary(model models.AWSInstance, accountName string) Summary {
	result := Summary{
		ID: model.ID, CloudAccountID: model.CloudAccountID, CloudAccountName: accountName,
		Region: model.Region, InstanceID: model.InstanceID, Name: model.Name,
		InstanceType: model.InstanceType, State: model.State, VPCID: model.VPCID,
		SubnetID: model.SubnetID, AvailabilityZone: model.AvailabilityZone,
		LaunchTime: model.LaunchTime, Tags: map[string]string{}, LastSeenAt: model.LastSeenAt,
		Deleted: model.DeletedAt.Valid,
	}
	if model.PrivateIP != nil {
		result.PrivateIP = model.PrivateIP.String()
	}
	if model.PublicIP != nil {
		result.PublicIP = model.PublicIP.String()
	}
	_ = json.Unmarshal(model.Tags, &result.Tags)
	if result.Tags == nil {
		result.Tags = map[string]string{}
	}
	return result
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
		State:          f.State,
		IP:             f.IP,
		TagKey:         f.TagKey,
		TagValue:       f.TagValue,
		NetworkRef:     f.VPCID,
	}
}
