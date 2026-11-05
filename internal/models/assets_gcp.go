package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GCPInstance struct {
	ID                uint64         `gorm:"primaryKey"`
	CloudAccountID    uint64         `gorm:"column:cloud_account_id;not null"`
	InstanceID        string         `gorm:"column:instance_id;type:text;not null"`
	Name              string         `gorm:"type:text;not null;default:''"`
	InstanceType      string         `gorm:"type:text;not null;default:''"`
	NativeStatus      string         `gorm:"type:text;not null;default:''"`
	Region            string         `gorm:"type:text;not null"`
	Zone              string         `gorm:"type:text;not null;default:''"`
	NetworkInterfaces datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Tags              datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	NativeDetails     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt        time.Time      `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

func (GCPInstance) TableName() string { return "gcp_instances" }

type GCPNetwork struct {
	ID             uint64         `gorm:"primaryKey"`
	CloudAccountID uint64         `gorm:"column:cloud_account_id;not null"`
	NetworkID      string         `gorm:"column:network_id;type:text;not null"`
	NativeRef      string         `gorm:"type:text;not null;default:''"`
	Name           string         `gorm:"type:text;not null;default:''"`
	Region         string         `gorm:"type:text;not null;default:''"`
	CIDRBlocks     datatypes.JSON `gorm:"column:cidr_blocks;type:jsonb;not null;default:'[]'"`
	NativeStatus   string         `gorm:"type:text;not null;default:''"`
	Tags           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	NativeDetails  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt     time.Time      `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (GCPNetwork) TableName() string { return "gcp_networks" }

type GCPSubnet struct {
	ID             uint64         `gorm:"primaryKey"`
	CloudAccountID uint64         `gorm:"column:cloud_account_id;not null"`
	SubnetID       string         `gorm:"column:subnet_id;type:text;not null"`
	NetworkRef     string         `gorm:"type:text;not null;default:''"`
	Name           string         `gorm:"type:text;not null;default:''"`
	Region         string         `gorm:"type:text;not null"`
	Zone           string         `gorm:"type:text;not null;default:''"`
	CIDRBlocks     datatypes.JSON `gorm:"column:cidr_blocks;type:jsonb;not null;default:'[]'"`
	Tags           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	NativeDetails  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt     time.Time      `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (GCPSubnet) TableName() string { return "gcp_subnets" }

type GCPDatabase struct {
	ID                 uint64         `gorm:"primaryKey"`
	CloudAccountID     uint64         `gorm:"column:cloud_account_id;not null"`
	DatabaseID         string         `gorm:"column:database_id;type:text;not null"`
	Name               string         `gorm:"type:text;not null;default:''"`
	Product            string         `gorm:"type:text;not null;default:'cloud_sql'"`
	Engine             string         `gorm:"type:text;not null;default:''"`
	EngineVersion      string         `gorm:"type:text;not null;default:''"`
	InstanceClass      string         `gorm:"type:text;not null;default:''"`
	NativeStatus       string         `gorm:"type:text;not null;default:''"`
	Region             string         `gorm:"type:text;not null"`
	Zone               string         `gorm:"type:text;not null;default:''"`
	Endpoints          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	StorageGB          *int64         `gorm:"column:storage_gb"`
	HAMode             *string        `gorm:"column:ha_mode;type:text"`
	PubliclyAccessible *bool
	Tags               datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	NativeDetails      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt         time.Time      `gorm:"not null"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (GCPDatabase) TableName() string { return "gcp_databases" }

type GCPFirewall struct {
	ID             uint64         `gorm:"primaryKey"`
	CloudAccountID uint64         `gorm:"column:cloud_account_id;not null"`
	FirewallID     string         `gorm:"column:firewall_id;type:text;not null"`
	Name           string         `gorm:"type:text;not null;default:''"`
	NetworkRef     string         `gorm:"type:text;not null;default:''"`
	Region         string         `gorm:"type:text;not null;default:''"`
	Description    string         `gorm:"type:text;not null;default:''"`
	InboundRules   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	OutboundRules  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Tags           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	NativeDetails  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt     time.Time      `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (GCPFirewall) TableName() string { return "gcp_firewalls" }
