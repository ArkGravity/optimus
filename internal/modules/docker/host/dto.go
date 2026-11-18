package host

import "time"

// Summary is the public host shape. Password material is never included.
type Summary struct {
	ID             uint64     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	SocketPath     string     `json:"socket_path"`
	AuthType       string     `json:"auth_type"`
	SSHKeyID       *uint64    `json:"ssh_key_id,omitempty"`
	SSHKeyName     string     `json:"ssh_key_name,omitempty"`
	Username       string     `json:"username"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastCheckOK    *bool      `json:"last_check_ok,omitempty"`
	LastCheckError string     `json:"last_check_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Detail = Summary

// CreateRequest creates a host. Port defaults to 22 and SocketPath to
// /var/run/docker.sock. For ssh_key auth an empty Username defaults to the
// key's username; Password is required for password auth and write-only.
type CreateRequest struct {
	Name        string  `json:"name"        binding:"required,max=128"`
	Description string  `json:"description" binding:"max=4096"`
	Host        string  `json:"host"        binding:"required,max=255"`
	Port        int     `json:"port"        binding:"omitempty,min=1,max=65535"`
	SocketPath  string  `json:"socket_path" binding:"max=255"`
	AuthType    string  `json:"auth_type"   binding:"required,oneof=ssh_key password"`
	SSHKeyID    *uint64 `json:"ssh_key_id"`
	Username    string  `json:"username"    binding:"max=64"`
	Password    string  `json:"password"    binding:"max=1024"`
}

// UpdateRequest patches a host. Omitted fields are unchanged. An omitted or
// empty Password keeps the stored password; switching to password auth
// requires a new Password.
type UpdateRequest struct {
	Name        *string `json:"name,omitempty"        binding:"omitempty,max=128"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
	Host        *string `json:"host,omitempty"        binding:"omitempty,max=255"`
	Port        *int    `json:"port,omitempty"        binding:"omitempty,min=1,max=65535"`
	SocketPath  *string `json:"socket_path,omitempty" binding:"omitempty,max=255"`
	AuthType    *string `json:"auth_type,omitempty"   binding:"omitempty,oneof=ssh_key password"`
	SSHKeyID    *uint64 `json:"ssh_key_id,omitempty"`
	Username    *string `json:"username,omitempty"    binding:"omitempty,max=64"`
	Password    *string `json:"password,omitempty"    binding:"omitempty,max=1024"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Search   string `form:"search"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// ProbeRequest dials with the submitted connection parameters without creating
// or updating a host row. HostID is optional: when editing a password host and
// Password is empty, the sealed password from that host is reused.
type ProbeRequest struct {
	HostID     *uint64 `json:"host_id"`
	Host       string  `json:"host"        binding:"required,max=255"`
	Port       int     `json:"port"        binding:"omitempty,min=1,max=65535"`
	SocketPath string  `json:"socket_path" binding:"max=255"`
	AuthType   string  `json:"auth_type"   binding:"required,oneof=ssh_key password"`
	SSHKeyID   *uint64 `json:"ssh_key_id"`
	Username   string  `json:"username"    binding:"max=64"`
	Password   string  `json:"password"    binding:"max=1024"`
}

// TestResult reports one connection probe. Message is a fixed safe text.
type TestResult struct {
	OK            bool      `json:"ok"`
	ServerVersion string    `json:"server_version,omitempty"`
	APIVersion    string    `json:"api_version,omitempty"`
	Message       string    `json:"message,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}
