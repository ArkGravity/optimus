package vpc

import (
	"encoding/json"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
)

type Summary struct {
	NetworkRef    string                 `json:"network_ref"`
	CIDRBlocks    []string               `json:"cidr_blocks"`
	NativeDetails query.VPCNativeDetails `json:"native_details"`
	isDefault     *bool
	AssetKey      string `json:"asset_key"`
	Provider      string `json:"provider"`
	ProjectID     string `json:"project_id"`
	LocationType  string `json:"location_type"`
	Location      string `json:"location"`
	Zone          string `json:"zone"`
	NativeID      string `json:"native_id"`
	NativeStatus  string `json:"native_status"`

	ID               uint64            `json:"id"`
	CloudAccountID   uint64            `json:"cloud_account_id"`
	CloudAccountName string            `json:"cloud_account_name,omitempty"`
	Region           string            `json:"region"`
	VPCID            string            `json:"vpc_id"`
	Name             string            `json:"name"`
	CIDRBlock        string            `json:"cidr_block,omitempty"`
	IsDefault        bool              `json:"is_default" extensions:"x-nullable"`
	State            string            `json:"state"`
	Tags             map[string]string `json:"tags"`
	LastSeenAt       time.Time         `json:"last_seen_at"`
	Deleted          bool              `json:"deleted"`
}

type SubnetSummary struct {
	CloudAccountName string                    `json:"cloud_account_name,omitempty"`
	AssetKey         string                    `json:"asset_key"`
	Provider         string                    `json:"provider"`
	ProjectID        string                    `json:"project_id"`
	LocationType     string                    `json:"location_type"`
	Location         string                    `json:"location"`
	Zone             string                    `json:"zone"`
	NativeID         string                    `json:"native_id"`
	NativeStatus     string                    `json:"native_status"`
	NetworkRef       string                    `json:"network_ref"`
	CIDRBlocks       []string                  `json:"cidr_blocks"`
	NativeDetails    query.SubnetNativeDetails `json:"native_details"`
	ID               uint64                    `json:"id"`
	CloudAccountID   uint64                    `json:"cloud_account_id"`
	Region           string                    `json:"region"`
	SubnetID         string                    `json:"subnet_id"`
	VPCID            string                    `json:"vpc_id"`
	Name             string                    `json:"name"`
	CIDRBlock        string                    `json:"cidr_block,omitempty"`
	AvailabilityZone string                    `json:"availability_zone"`
	Tags             map[string]string         `json:"tags"`
	LastSeenAt       time.Time                 `json:"last_seen_at"`
	Deleted          bool                      `json:"deleted"`
}

type ListQuery struct {
	Provider  string `form:"provider"`
	ProjectID string `form:"project_id"`
	Zone      string `form:"zone"`

	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type SubnetListQuery struct {
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type ListFilter struct {
	Provider  string
	ProjectID string
	Zone      string

	AccountID      uint64
	Region         string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type SubnetListFilter struct {
	CloudAccountID uint64
	Region         string
	VPCID          string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}

type SubnetListResponse struct {
	Items []SubnetSummary `json:"items"`
	Total int64           `json:"total"`
}

// MarshalJSON keeps the legacy Go bool while preserving unknown provider facts.
func (s Summary) MarshalJSON() ([]byte, error) {
	type plain Summary
	value := s.isDefault
	if s.Provider == "" || s.Provider == "aws" {
		value = &s.IsDefault
	}
	return json.Marshal(struct {
		plain
		IsDefault *bool `json:"is_default" extensions:"x-nullable"`
	}{plain(s), value})
}
