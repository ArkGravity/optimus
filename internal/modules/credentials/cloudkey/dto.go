package cloudkey

import "time"

type Summary struct {
	ID                  uint64    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	Provider            string    `json:"provider"`
	Region              string    `json:"region"`
	AuthType            string    `json:"auth_type"`
	ServiceAccountEmail string    `json:"service_account_email"`
	CredentialProjectID string    `json:"credential_project_id"`
	NeedsConfiguration  bool      `json:"needs_configuration"`
	CreatedBy           *Actor    `json:"created_by,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Detail = Summary

type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type CreateRequest struct {
	Name               string `json:"name"              binding:"required,max=128"`
	Description        string `json:"description"       binding:"max=4096"`
	Provider           string `json:"provider"          binding:"required,oneof=aliyun aws gcp"`
	Region             string `json:"region"            binding:"max=32"`
	AuthType           string `json:"auth_type"`
	AccessKeyID        string `json:"access_key_id" binding:"max=256"`
	SecretAccessKey    string `json:"secret_access_key"`
	ServiceAccountJSON string `json:"service_account_json"`
}

type UpdateRequest struct {
	Name               *string `json:"name,omitempty"              binding:"omitempty,max=128"`
	Description        *string `json:"description,omitempty"       binding:"omitempty,max=4096"`
	Provider           *string `json:"provider,omitempty"          binding:"omitempty,oneof=aliyun aws gcp"`
	Region             *string `json:"region,omitempty"            binding:"omitempty,max=32"`
	AccessKeyID        *string `json:"access_key_id,omitempty"     binding:"omitempty,max=256"`
	SecretAccessKey    *string `json:"secret_access_key,omitempty"`
	AuthType           *string `json:"auth_type,omitempty"`
	ServiceAccountJSON *string `json:"service_account_json,omitempty"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
	Provider string `form:"provider"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
