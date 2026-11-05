package runs

import "time"

type Summary struct {
	ID                uint64     `json:"id"`
	CloudAccountID    uint64     `json:"cloud_account_id"`
	CloudAccountName  string     `json:"cloud_account_name,omitempty"`
	Provider          string     `json:"provider"`
	LocationType      string     `json:"location_type"`
	Location          string     `json:"location"`
	ScopeRevision     int64      `json:"scope_revision"`
	Region            string     `json:"region"`
	ResourceType      string     `json:"resource_type"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	Status            string     `json:"status"`
	ItemsSeen         int32      `json:"items_seen"`
	ItemsSoftDeleted  int32      `json:"items_softdeleted"`
	Error             string     `json:"error,omitempty"`
	ErrorCode         int32      `json:"error_code,omitempty"`
	Trigger           string     `json:"trigger"`
	TriggeredByUserID *uint64    `json:"triggered_by_user_id,omitempty"`
}

type ListQuery struct {
	Provider     string `form:"provider"`
	LocationType string `form:"location_type"`
	Location     string `form:"location"`
	AccountID    uint64 `form:"account_id"`
	ResourceType string `form:"resource_type"`
	Status       string `form:"status"`
	StartedAfter string `form:"started_after"`
	Page         int    `form:"page,default=1" binding:"min=1"`
	Size         int    `form:"size,default=20" binding:"min=1,max=200"`
}

// ListFilter is the validated repository input. StartedAfter is parsed once by
// the service so persistence cannot silently ignore an invalid timestamp.
type ListFilter struct {
	Provider     string
	LocationType string
	Location     string
	AccountID    uint64
	ResourceType string
	Status       string
	StartedAfter *time.Time
	Page         int
	Size         int
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
