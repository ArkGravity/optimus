package query

import (
	"context"
	"encoding/json"
)

type SubnetNativeDetails struct {
	AvailableIPCount      *int64 `json:"available_ip_count,omitempty"`
	Purpose               string `json:"purpose,omitempty"`
	PrivateIPGoogleAccess *bool  `json:"private_ip_google_access,omitempty"`
}
type Subnet struct {
	Base
	NetworkRef       string              `json:"network_ref"`
	CIDRBlocks       []string            `json:"cidr_blocks"`
	SubnetID         string              `json:"subnet_id"`
	VPCID            string              `json:"vpc_id"`
	CIDRBlock        string              `json:"cidr_block,omitempty"`
	AvailabilityZone string              `json:"availability_zone"`
	NativeDetails    SubnetNativeDetails `json:"native_details"`
}

var subnetSources = map[string]string{
	"aws":    projection("aws", "aws_subnets", "r.subnet_id", "r.name", "''", "r.availability_zone", "region", `jsonb_build_object('network_ref',r.vpc_id,'vpc_id',r.vpc_id,'cidr_blocks',CASE WHEN r.cidr_block IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(r.cidr_block::text) END,'cidr_block',COALESCE(r.cidr_block::text,''),'availability_zone',r.availability_zone)`, `'{}'::jsonb`),
	"aliyun": projection("aliyun", "aliyun_vswitches", "r.subnet_id", "r.name", "''", "r.zone", "region", `jsonb_build_object('network_ref',r.network_ref,'cidr_blocks',r.cidr_blocks)`, "r.native_details"),
	"gcp":    projection("gcp", "gcp_subnets", "r.subnet_id", "r.name", "''", "r.zone", "region", `jsonb_build_object('network_ref',r.network_ref,'cidr_blocks',r.cidr_blocks)`, "r.native_details"),
}

func subnetFrom(row projectionRow) (Subnet, error) {
	item := Subnet{}
	if err := json.Unmarshal(row.Facts, &item); err != nil {
		return item, err
	}
	if err := json.Unmarshal(row.NativeDetails, &item.NativeDetails); err != nil {
		return item, err
	}
	item.Base = row.base("subnet")
	item.SubnetID = item.NativeID
	if item.CIDRBlocks == nil {
		item.CIDRBlocks = []string{}
	}
	return item, nil
}
func (r *Repo) ListSubnets(ctx context.Context, f Filter) ([]Subnet, int64, error) {
	extra := ""
	var args []any
	if f.NetworkRef != "" {
		extra = "facts->>'network_ref' = ?"
		args = []any{f.NetworkRef}
	}
	rows, total, err := r.page(ctx, subnetSources, f, extra, args)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Subnet, len(rows))
	for i, row := range rows {
		items[i], err = subnetFrom(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

// ListSubnetsByVPC scopes the child relation using the selected parent's full identity.
// Global GCP networks retain all regional children, including Shared VPC host references.
func (r *Repo) ListSubnetsByVPC(ctx context.Context, key string, f Filter) ([]Subnet, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	parent, err := r.FindVPC(ctx, key, f.IncludeDeleted)
	if err != nil {
		return nil, 0, err
	}
	if parent.NetworkRef == "" {
		return []Subnet{}, 0, nil
	}
	if (f.Provider != "" && f.Provider != parent.Provider) || (f.AccountID != 0 && f.AccountID != parent.CloudAccountID) || (f.ProjectID != "" && f.ProjectID != parent.ProjectID) || (parent.LocationType != "global" && f.Region != "" && f.Region != parent.Region) {
		return []Subnet{}, 0, nil
	}
	f.Provider = parent.Provider
	f.AccountID = parent.CloudAccountID
	f.ProjectID = parent.ProjectID
	f.NetworkRef = parent.NetworkRef
	if parent.LocationType != "global" {
		f.Region = parent.Region
	}
	return r.ListSubnets(ctx, f)
}
func (r *Repo) FindSubnet(ctx context.Context, key string, includeDeleted bool) (*Subnet, error) {
	row, err := r.find(ctx, subnetSources, key, "subnet", includeDeleted)
	if err != nil {
		return nil, err
	}
	item, err := subnetFrom(*row)
	return &item, err
}
