package firewall

import "github.com/ArkGravity/optimus/internal/modules/assets/query"

// Detail preserves common displayed entries and allowlisted provider facts.
type Detail = query.Firewall

// Summary omits rule arrays and their per-entry native facts from list responses.
type Summary struct {
	query.Base
	NetworkRef        string        `json:"network_ref"`
	Description       string        `json:"description"`
	GroupID           string        `json:"group_id,omitempty"`
	GroupName         string        `json:"group_name,omitempty"`
	VPCID             string        `json:"vpc_id,omitempty"`
	OwnerID           string        `json:"owner_id,omitempty"`
	InboundRuleCount  int           `json:"inbound_rule_count"`
	OutboundRuleCount int           `json:"outbound_rule_count"`
	NativeDetails     NativeSummary `json:"native_details"`
}
type NativeSummary struct {
	Priority  *int32 `json:"priority,omitempty"`
	Disabled  *bool  `json:"disabled,omitempty"`
	Direction string `json:"direction,omitempty"`
}
type ListQuery struct {
	Provider       string `form:"provider"`
	AccountID      uint64 `form:"account_id"`
	ProjectID      string `form:"project_id"`
	Region         string `form:"region"`
	Zone           string `form:"zone"`
	NetworkRef     string `form:"network_ref"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}
type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}

func summarize(row query.Firewall) Summary {
	return Summary{
		Base: row.Base, NetworkRef: row.NetworkRef, Description: row.Description,
		GroupID: row.GroupID, GroupName: row.GroupName, VPCID: row.VPCID, OwnerID: row.OwnerID,
		InboundRuleCount: row.InboundRuleCount, OutboundRuleCount: row.OutboundRuleCount,
		NativeDetails: NativeSummary{Priority: row.NativeDetails.Priority, Disabled: row.NativeDetails.Disabled, Direction: row.NativeDetails.Direction},
	}
}
