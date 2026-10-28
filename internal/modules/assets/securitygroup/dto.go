package securitygroup

import (
	"time"

	"github.com/ArkGravity/optimus/internal/models"
)

type Rule = models.SecurityGroupRule

type Summary struct {
	ID                uint64    `json:"id"`
	CloudAccountID    uint64    `json:"cloud_account_id"`
	CloudAccountName  string    `json:"cloud_account_name"`
	Region            string    `json:"region"`
	GroupID           string    `json:"group_id"`
	GroupName         string    `json:"group_name"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	VPCID             string    `json:"vpc_id"`
	OwnerID           string    `json:"owner_id"`
	InboundRuleCount  int       `json:"inbound_rule_count"`
	OutboundRuleCount int       `json:"outbound_rule_count"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	Deleted           bool      `json:"deleted"`
}

type Detail struct {
	Summary
	Tags          map[string]string `json:"tags"`
	InboundRules  []Rule            `json:"inbound_rules"`
	OutboundRules []Rule            `json:"outbound_rules"`
}

type ListQuery struct {
	AccountID      uint64 `form:"account_id"`
	Region         string `form:"region"`
	VPCID          string `form:"vpc_id"`
	Q              string `form:"q"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
