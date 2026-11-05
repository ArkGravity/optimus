package database

import (
	"encoding/json"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type Summary struct {
	Name               string                      `json:"name"`
	Product            string                      `json:"product"`
	Endpoints          []resource.DatabaseEndpoint `json:"endpoints"`
	HAMode             *string                     `json:"ha_mode" extensions:"x-nullable"`
	NativeDetails      query.DatabaseNativeDetails `json:"native_details"`
	multiAZ            *bool
	publiclyAccessible *bool
	storageGB          *int64
	AssetKey           string `json:"asset_key"`
	Provider           string `json:"provider"`
	ProjectID          string `json:"project_id"`
	LocationType       string `json:"location_type"`
	Location           string `json:"location"`
	Zone               string `json:"zone"`
	NativeID           string `json:"native_id"`
	NativeStatus       string `json:"native_status"`

	ID                 uint64            `json:"id"`
	CloudAccountID     uint64            `json:"cloud_account_id"`
	CloudAccountName   string            `json:"cloud_account_name,omitempty"`
	Region             string            `json:"region"`
	DBInstanceID       string            `json:"db_instance_id"`
	Engine             string            `json:"engine"`
	EngineVersion      string            `json:"engine_version"`
	InstanceClass      string            `json:"instance_class"`
	Status             string            `json:"status"`
	Endpoint           string            `json:"endpoint"`
	Port               *int32            `json:"port,omitempty"`
	MultiAZ            bool              `json:"multi_az" extensions:"x-nullable"`
	PubliclyAccessible bool              `json:"publicly_accessible" extensions:"x-nullable"`
	StorageGB          *int32            `json:"storage_gb,omitempty" extensions:"x-nullable" format:"int64"`
	Tags               map[string]string `json:"tags"`
	LastSeenAt         time.Time         `json:"last_seen_at"`
	Deleted            bool              `json:"deleted"`
}

type ListQuery struct {
	Provider  string `form:"provider"`
	ProjectID string `form:"project_id"`
	Zone      string `form:"zone"`

	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	Engine         string `form:"engine"`
	Status         string `form:"status"`
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
	Engine         string
	Status         string
	Q              string
	IncludeDeleted bool
	Page           int
	Size           int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}

// MarshalJSON retains source-compatible AWS aliases without inventing native facts.
func (s Summary) MarshalJSON() ([]byte, error) {
	type plain Summary
	multi, public, storage := s.multiAZ, s.publiclyAccessible, s.storageGB
	if s.Provider == "" || s.Provider == "aws" {
		multi = &s.MultiAZ
		public = &s.PubliclyAccessible
	}
	if storage == nil && s.StorageGB != nil {
		value := int64(*s.StorageGB)
		storage = &value
	}
	return json.Marshal(struct {
		plain
		MultiAZ            *bool  `json:"multi_az" extensions:"x-nullable"`
		PubliclyAccessible *bool  `json:"publicly_accessible" extensions:"x-nullable"`
		StorageGB          *int64 `json:"storage_gb" extensions:"x-nullable" format:"int64"`
	}{plain(s), multi, public, storage})
}
