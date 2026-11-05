package instance

import (
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type Summary struct {
	NetworkInterfaces []resource.NetworkInterface `json:"network_interfaces"`
	NativeDetails     query.InstanceNativeDetails `json:"native_details"`
	AssetKey          string                      `json:"asset_key"`
	Provider          string                      `json:"provider"`
	ProjectID         string                      `json:"project_id"`
	LocationType      string                      `json:"location_type"`
	Location          string                      `json:"location"`
	Zone              string                      `json:"zone"`
	NativeID          string                      `json:"native_id"`
	NativeStatus      string                      `json:"native_status"`

	ID               uint64            `json:"id"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	CloudAccountName string            `json:"cloud_account_name,omitempty"`
	Region           string            `json:"region"`
	InstanceID       string            `json:"instance_id"`
	Name             string            `json:"name"`
	InstanceType     string            `json:"instance_type"`
	State            string            `json:"state"`
	PrivateIP        string            `json:"private_ip,omitempty"`
	PublicIP         string            `json:"public_ip,omitempty"`
	VPCID            string            `json:"vpc_id,omitempty"`
	SubnetID         string            `json:"subnet_id,omitempty"`
	AvailabilityZone string            `json:"availability_zone,omitempty"`
	LaunchTime       *time.Time        `json:"launch_time,omitempty"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type ListQuery struct {
	IP        string `form:"ip"`
	TagKey    string `form:"tag_key"`
	TagValue  string `form:"tag_value"`
	Provider  string `form:"provider"`
	ProjectID string `form:"project_id"`
	Zone      string `form:"zone"`

	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	State          string `form:"state"`
	VPCID          string `form:"vpc_id"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1"  binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

// ListFilter is validated before it reaches persistence.
type ListFilter struct {
	IP        string
	TagKey    string
	TagValue  string
	Provider  string
	ProjectID string
	Zone      string

	AccountID      uint64
	Region         string
	State          string
	VPCID          string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
	Offset         int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
