package models

import "time"

type CredentialCloudKey struct {
	ID                    uint64  `gorm:"primaryKey"`
	Name                  string  `gorm:"size:128;not null;uniqueIndex"`
	Description           string  `gorm:"type:text;not null;default:''"`
	Provider              string  `gorm:"size:16;not null"`
	Region                string  `gorm:"size:32;not null;default:''"`
	AuthType              string  `gorm:"size:32;not null;default:access_key"`
	AccessKeyIDEnc        []byte  `gorm:"column:access_key_id_enc"`
	SecretAccessKeyEnc    []byte  `gorm:"column:secret_access_key_enc"`
	ServiceAccountJSONEnc []byte  `gorm:"column:service_account_json_enc"`
	ServiceAccountEmail   string  `gorm:"type:text;not null;default:''"`
	CredentialProjectID   string  `gorm:"type:text;not null;default:''"`
	CreatedByUserID       *uint64 `gorm:"column:created_by_user_id"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (CredentialCloudKey) TableName() string { return "credentials_cloud_keys" }
