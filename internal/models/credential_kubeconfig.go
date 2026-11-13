package models

import "time"

type CredentialKubeconfig struct {
	ID               uint64  `gorm:"primaryKey"`
	Name             string  `gorm:"size:128;not null;uniqueIndex"`
	Description      string  `gorm:"type:text;not null;default:''"`
	DefaultNamespace string  `gorm:"column:default_namespace;size:64;not null;default:''"`
	KubeconfigEnc    []byte  `gorm:"column:kubeconfig_enc;not null"`
	AuthType         string  `gorm:"size:32;not null;default:kubeconfig"`
	CloudKeyID       *uint64 `gorm:"column:cloud_key_id"`
	GKEProject       string  `gorm:"column:gke_project;size:128;not null;default:''"`
	GKELocation      string  `gorm:"column:gke_location;size:64;not null;default:''"`
	GKECluster       string  `gorm:"column:gke_cluster;size:64;not null;default:''"`
	GKEEndpoint      string  `gorm:"column:gke_endpoint;size:16;not null;default:''"`
	CreatedByUserID  *uint64 `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CredentialKubeconfig) TableName() string { return "credentials_kubeconfigs" }
