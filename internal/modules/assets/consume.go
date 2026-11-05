// Package assets exposes a read-only Consumer seam so downstream modules can
// look up discovered resource snapshots without going through HTTP. Resource
// lookups intentionally have no purpose parameter and produce no audit rows.
package assets

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/netip"
	"strconv"
	"strings"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

// Instance is the read-side view of a live instance snapshot.
type Instance struct {
	Provider     string
	AssetKey     string
	ProjectID    string
	LocationType string
	Location     string
	Zone         string
	NativeID     string
	NetworkRef   string
	AccountID    int64
	AccountName  string
	Region       string
	InstanceID   string
	Name         string
	InstanceType string
	State        string
	PrivateIP    netip.Addr
	PublicIP     netip.Addr
	VPCID        string
	SubnetID     string
}

// ErrAssetsInstanceNotFound is returned when a single-instance lookup has no
// live match. Callers should test it with errors.Is.
var ErrAssetsInstanceNotFound = errors.New("assets: instance not found")

// InstanceScope narrows a private address lookup using provider-native identity.
type InstanceScope struct {
	Provider       string
	CloudAccountID *uint64
	NetworkRef     string
}

// ErrAssetsInstanceAmbiguous identifies multiple distinct live instance matches.
var ErrAssetsInstanceAmbiguous = errors.New("multiple assets match instance lookup")

// Consumer is the stable read-only assets seam used by downstream modules.
type Consumer interface {
	LookupInstanceByPrivateIP(ctx context.Context, ip netip.Addr) (*Instance, error)
	LookupInstanceByPrivateIPScoped(ctx context.Context, ip netip.Addr, scope InstanceScope) (*Instance, error)
	LookupInstanceByID(ctx context.Context, accountID int64, region, instanceID string) (*Instance, error)
	ListInstancesByVPC(ctx context.Context, accountID int64, region, vpcID string) ([]Instance, error)
}

type consumer struct{ db *gorm.DB }

// NewConsumer constructs a Consumer backed by the assets database tables.
func NewConsumer(db *gorm.DB) Consumer { return &consumer{db: db} }

func (c *consumer) LookupInstanceByPrivateIP(ctx context.Context, ip netip.Addr) (*Instance, error) {
	return c.LookupInstanceByPrivateIPScoped(ctx, ip, InstanceScope{})
}

func (c *consumer) LookupInstanceByPrivateIPScoped(ctx context.Context, ip netip.Addr, scope InstanceScope) (*Instance, error) {
	if !ip.IsValid() || ip.Zone() != "" {
		return nil, validationError("IP address must be valid and unzoned")
	}
	f := query.Filter{
		Provider: scope.Provider, NetworkRef: scope.NetworkRef,
		IP: ip.Unmap().String(), PrivateIPOnly: true, Page: 1, Size: 2,
	}
	if scope.CloudAccountID != nil {
		if *scope.CloudAccountID == 0 || *scope.CloudAccountID > math.MaxInt64 {
			return nil, validationError("account ID must be positive and within database range")
		}
		f.AccountID = *scope.CloudAccountID
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	rows, _, err := query.NewRepo(c.db).ListInstances(ctx, f)
	if err != nil {
		return nil, databaseError(err)
	}
	if len(rows) == 0 {
		return nil, ErrAssetsInstanceNotFound
	}
	if len(rows) > 1 {
		return nil, ErrAssetsInstanceAmbiguous
	}
	row := rows[0]
	nic, ok := matchedInterface(row.NetworkInterfaces, ip.Unmap(), scope.NetworkRef)
	if !ok {
		return nil, ErrAssetsInstanceNotFound
	}
	return &Instance{
		AccountID: int64(row.CloudAccountID), AccountName: row.CloudAccountName,
		Provider: row.Provider, AssetKey: row.AssetKey, ProjectID: row.ProjectID,
		LocationType: row.LocationType, Location: row.Location, Region: row.Region, Zone: row.Zone,
		NativeID: row.NativeID, InstanceID: row.InstanceID, Name: row.Name,
		InstanceType: row.InstanceType, State: row.State,
		PrivateIP: ip.Unmap(), PublicIP: firstAddress(nic.PublicIPv4, nic.PublicIPv6),
		VPCID: nic.NetworkRef, NetworkRef: nic.NetworkRef, SubnetID: nic.SubnetRef,
	}, nil
}

// Select metadata from the same NIC that satisfied the private address and scope.
func matchedInterface(nics []resource.NetworkInterface, ip netip.Addr, network string) (resource.NetworkInterface, bool) {
	for _, nic := range nics {
		if network != "" && nic.NetworkRef != network {
			continue
		}
		for _, addresses := range [][]string{nic.PrivateIPv4, nic.PrivateIPv6} {
			for _, value := range addresses {
				address, err := netip.ParseAddr(value)
				if err == nil && address.Zone() == "" && address.Unmap() == ip {
					return nic, true
				}
			}
		}
	}
	return resource.NetworkInterface{}, false
}

func firstAddress(groups ...[]string) netip.Addr {
	for _, addresses := range groups {
		for _, value := range addresses {
			address, err := netip.ParseAddr(value)
			if err == nil && address.Zone() == "" {
				return address.Unmap()
			}
		}
	}
	return netip.Addr{}
}

func (c *consumer) LookupInstanceByID(ctx context.Context, accountID int64, region, instanceID string) (*Instance, error) {
	region = strings.TrimSpace(region)
	instanceID = strings.TrimSpace(instanceID)
	if err := validateTuple(accountID, region, instanceID, "instance ID"); err != nil {
		return nil, err
	}
	return c.findOne(ctx,
		"aws_instances.cloud_account_id = ? AND aws_instances.region = ? AND aws_instances.instance_id = ?",
		accountID, region, instanceID,
	)
}

func (c *consumer) ListInstancesByVPC(ctx context.Context, accountID int64, region, vpcID string) ([]Instance, error) {
	region = strings.TrimSpace(region)
	vpcID = strings.TrimSpace(vpcID)
	if err := validateTuple(accountID, region, vpcID, "VPC ID"); err != nil {
		return nil, err
	}

	rows := make([]consumerRow, 0)
	err := c.baseQuery(ctx).
		Where("aws_instances.cloud_account_id = ?", accountID).
		Where("aws_instances.region = ?", region).
		Where("aws_instances.vpc_id = ?", vpcID).
		Order("aws_instances.last_seen_at DESC, aws_instances.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, databaseError(err)
	}

	items := make([]Instance, len(rows))
	for i := range rows {
		items[i] = rows[i].instance()
	}
	return items, nil
}

func (c *consumer) findOne(ctx context.Context, where string, args ...any) (*Instance, error) {
	var row consumerRow
	err := c.baseQuery(ctx).
		Where(where, args...).
		Order("aws_instances.last_seen_at DESC, aws_instances.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, databaseError(err)
	}
	if row.ID == 0 {
		return nil, ErrAssetsInstanceNotFound
	}
	result := row.instance()
	return &result, nil
}

// consumerRow deliberately projects inet columns through host() into strings.
// Scanning PostgreSQL inet directly into models.AWSInstance's *net.IP fields is
// driver-dependent and fails with the pgx/GORM combination used by Optimus.
type consumerRow struct {
	ID           int64          `gorm:"column:id"`
	AccountID    int64          `gorm:"column:cloud_account_id"`
	AccountName  string         `gorm:"column:account_name"`
	ProjectID    string         `gorm:"column:project_id"`
	Zone         string         `gorm:"column:availability_zone"`
	Region       string         `gorm:"column:region"`
	InstanceID   string         `gorm:"column:instance_id"`
	Name         string         `gorm:"column:name"`
	InstanceType string         `gorm:"column:instance_type"`
	State        string         `gorm:"column:state"`
	PrivateIP    sql.NullString `gorm:"column:private_ip_text"`
	PublicIP     sql.NullString `gorm:"column:public_ip_text"`
	VPCID        string         `gorm:"column:vpc_id"`
	SubnetID     string         `gorm:"column:subnet_id"`
}

func (c *consumer) baseQuery(ctx context.Context) *gorm.DB {
	return c.db.WithContext(ctx).
		Table("aws_instances").
		Select(`
			aws_instances.id,
			aws_instances.cloud_account_id,
			cloud_accounts.name AS account_name,
			COALESCE(cloud_accounts.project_id,'') AS project_id,
			aws_instances.availability_zone,
			aws_instances.region,
			aws_instances.instance_id,
			aws_instances.name,
			aws_instances.instance_type,
			aws_instances.state,
			host(aws_instances.private_ip) AS private_ip_text,
			host(aws_instances.public_ip) AS public_ip_text,
			aws_instances.vpc_id,
			aws_instances.subnet_id`).
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_instances.cloud_account_id").
		Where("aws_instances.deleted_at IS NULL")
}

func (r consumerRow) instance() Instance {
	return Instance{
		Provider: "aws", AssetKey: "aws:instance:" + strconv.FormatInt(r.ID, 10),
		NativeID: r.InstanceID, ProjectID: r.ProjectID, Zone: r.Zone,
		LocationType: "region", Location: r.Region, NetworkRef: r.VPCID,
		AccountID: r.AccountID, AccountName: r.AccountName,
		Region: r.Region, InstanceID: r.InstanceID, Name: r.Name,
		InstanceType: r.InstanceType, State: r.State,
		PrivateIP: parseProjectedIP(r.PrivateIP.String, r.PrivateIP.Valid),
		PublicIP:  parseProjectedIP(r.PublicIP.String, r.PublicIP.Valid),
		VPCID:     r.VPCID, SubnetID: r.SubnetID,
	}
}

func parseProjectedIP(value string, valid bool) netip.Addr {
	if !valid {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func validateTuple(accountID int64, region, resourceID, resourceName string) error {
	if accountID <= 0 {
		return validationError("account ID must be positive")
	}
	if region == "" {
		return validationError("region is required")
	}
	if resourceID == "" {
		return validationError(resourceName + " is required")
	}
	return nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}
