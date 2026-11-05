package query

import (
	"context"
	"encoding/json"
)

type VPCNativeDetails struct {
	MTU                   *int64 `json:"mtu,omitempty"`
	RoutingMode           string `json:"routing_mode,omitempty"`
	AutoCreateSubnetworks *bool  `json:"auto_create_subnetworks,omitempty"`
}
type VPC struct {
	Base
	NetworkRef    string           `json:"network_ref"`
	CIDRBlocks    []string         `json:"cidr_blocks"`
	VPCID         string           `json:"vpc_id"`
	CIDRBlock     string           `json:"cidr_block,omitempty"`
	IsDefault     *bool            `json:"is_default,omitempty"`
	State         string           `json:"state"`
	NativeDetails VPCNativeDetails `json:"native_details"`
}

var vpcSources = map[string]string{
	"aws":    projection("aws", "aws_vpcs", "r.vpc_id", "r.name", "r.state", "''", "region", `jsonb_build_object('network_ref',r.vpc_id,'cidr_blocks',CASE WHEN r.cidr_block IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(r.cidr_block::text) END,'cidr_block',COALESCE(r.cidr_block::text,''),'is_default',r.is_default)`, `'{}'::jsonb`),
	"aliyun": projection("aliyun", "aliyun_vpcs", "r.network_id", "r.name", "r.native_status", "''", "region", `jsonb_build_object('network_ref',r.native_ref,'cidr_blocks',r.cidr_blocks)`, "r.native_details"),
	"gcp":    projection("gcp", "gcp_networks", "r.network_id", "r.name", "r.native_status", "''", "global", `jsonb_build_object('network_ref',r.native_ref,'cidr_blocks',r.cidr_blocks)`, "r.native_details"),
}

func vpcFrom(row projectionRow) (VPC, error) {
	item := VPC{}
	if err := json.Unmarshal(row.Facts, &item); err != nil {
		return item, err
	}
	if err := json.Unmarshal(row.NativeDetails, &item.NativeDetails); err != nil {
		return item, err
	}
	item.Base = row.base("vpc")
	item.VPCID = item.NativeID
	item.State = item.NativeStatus
	if item.CIDRBlocks == nil {
		item.CIDRBlocks = []string{}
	}
	return item, nil
}
func (r *Repo) ListVPCs(ctx context.Context, f Filter) ([]VPC, int64, error) {
	extra := ""
	var args []any
	if f.NetworkRef != "" {
		extra = "facts->>'network_ref' = ?"
		args = []any{f.NetworkRef}
	}
	rows, total, err := r.page(ctx, vpcSources, f, extra, args)
	if err != nil {
		return nil, 0, err
	}
	items := make([]VPC, len(rows))
	for i, row := range rows {
		items[i], err = vpcFrom(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (r *Repo) FindVPC(ctx context.Context, key string, includeDeleted bool) (*VPC, error) {
	row, err := r.find(ctx, vpcSources, key, "vpc", includeDeleted)
	if err != nil {
		return nil, err
	}
	item, err := vpcFrom(*row)
	return &item, err
}
