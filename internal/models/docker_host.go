package models

import (
	"time"

	"gorm.io/gorm"
)

// DockerHost stores connection metadata for a remote Docker Engine over SSH.
type DockerHost struct {
	ID              uint64 `gorm:"primaryKey"`
	Name            string `gorm:"size:128;not null"`
	Description     string `gorm:"type:text;not null;default:''"`
	Host            string `gorm:"size:255;not null"`
	Port            int    `gorm:"not null;default:22"`
	SocketPath      string `gorm:"column:socket_path;size:255;not null;default:'/var/run/docker.sock'"`
	AuthType        string `gorm:"column:auth_type;size:16;not null"`
	SSHKeyID        *uint64
	Username        string `gorm:"size:64;not null"`
	PasswordEnc     []byte `gorm:"column:password_enc"`
	LastCheckedAt   *time.Time
	LastCheckOK     *bool  `gorm:"column:last_check_ok"`
	LastCheckError  string `gorm:"column:last_check_error;type:text;not null;default:''"`
	CreatedByUserID *uint64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	SSHKey *CredentialSSHKey `gorm:"foreignKey:SSHKeyID;references:ID"`
}

func (DockerHost) TableName() string { return "docker_hosts" }
