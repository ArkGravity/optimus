package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SecurityGroupRule preserves the protocol and one source/destination target.
// For ICMP the port fields represent type and code, including the -1 wildcard.
type SecurityGroupRule struct {
	Protocol    string `json:"protocol"`
	FromPort    *int32 `json:"from_port,omitempty"`
	ToPort      *int32 `json:"to_port,omitempty"`
	TargetType  string `json:"target_type"`
	Target      string `json:"target"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id,omitempty"`
	VPCID       string `json:"vpc_id,omitempty"`
	PeeringID   string `json:"peering_id,omitempty"`
}

type AWSSecurityGroup struct {
	ID             uint64         `gorm:"primaryKey"`
	CloudAccountID uint64         `gorm:"column:cloud_account_id;not null"`
	Region         string         `gorm:"size:32;not null"`
	GroupID        string         `gorm:"column:group_id;size:64;not null"`
	GroupName      string         `gorm:"type:text;not null;default:''"`
	Name           string         `gorm:"type:text;not null;default:''"`
	Description    string         `gorm:"type:text;not null;default:''"`
	VPCID          string         `gorm:"column:vpc_id;size:64;not null;default:''"`
	OwnerID        string         `gorm:"column:owner_id;size:32;not null;default:''"`
	InboundRules   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	OutboundRules  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Tags           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	LastSeenAt     time.Time      `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (AWSSecurityGroup) TableName() string { return "aws_security_groups" }
