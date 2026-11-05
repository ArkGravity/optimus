package query

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

// InstanceNativeDetails is an allowlist; unknown native JSON fields never leave this seam.
type InstanceNativeDetails struct {
	CPUCount             *int64                                `json:"cpu_count,omitempty"`
	MemoryMB             *int64                                `json:"memory_mb,omitempty"`
	ImageID              string                                `json:"image_id,omitempty"`
	OS                   string                                `json:"os,omitempty"`
	NetworkTags          []string                              `json:"network_tags,omitempty"`
	ResourceManagerTags  map[string]string                     `json:"resource_manager_tags,omitempty"`
	GCPNetworkInterfaces []resource.GCPNetworkInterfaceDetails `json:"gcp_network_interfaces,omitempty"`
}

type Instance struct {
	Base
	InstanceID        string                      `json:"instance_id"`
	InstanceType      string                      `json:"instance_type"`
	State             string                      `json:"state"`
	NetworkInterfaces []resource.NetworkInterface `json:"network_interfaces"`
	PrivateIP         string                      `json:"private_ip,omitempty"`
	PublicIP          string                      `json:"public_ip,omitempty"`
	VPCID             string                      `json:"vpc_id,omitempty"`
	SubnetID          string                      `json:"subnet_id,omitempty"`
	AvailabilityZone  string                      `json:"availability_zone,omitempty"`
	LaunchTime        *time.Time                  `json:"launch_time,omitempty"`
	NativeDetails     InstanceNativeDetails       `json:"native_details"`
}

const awsNICs = `jsonb_build_array(jsonb_build_object('id','','name','','network_ref',r.vpc_id,'subnet_ref',r.subnet_id,
 'private_ipv4',CASE WHEN family(r.private_ip)=4 THEN jsonb_build_array(host(r.private_ip)) ELSE '[]'::jsonb END,
 'public_ipv4',CASE WHEN family(r.public_ip)=4 THEN jsonb_build_array(host(r.public_ip)) ELSE '[]'::jsonb END,
 'private_ipv6',CASE WHEN family(r.private_ip)=6 THEN jsonb_build_array(host(r.private_ip)) ELSE '[]'::jsonb END,
 'public_ipv6',CASE WHEN family(r.public_ip)=6 THEN jsonb_build_array(host(r.public_ip)) ELSE '[]'::jsonb END))`

var instanceSources = map[string]string{
	"aws":    projection("aws", "aws_instances", "r.instance_id", "r.name", "r.state", "r.availability_zone", "region", `jsonb_build_object('instance_type',r.instance_type,'network_interfaces',`+awsNICs+`,'private_ip',COALESCE(host(r.private_ip),''),'public_ip',COALESCE(host(r.public_ip),''),'vpc_id',r.vpc_id,'subnet_id',r.subnet_id,'availability_zone',r.availability_zone,'launch_time',r.launch_time)`, `'{}'::jsonb`),
	"aliyun": projection("aliyun", "aliyun_instances", "r.instance_id", "r.name", "r.native_status", "r.zone", "region", `jsonb_build_object('instance_type',r.instance_type,'network_interfaces',r.network_interfaces)`, "r.native_details"),
	"gcp":    projection("gcp", "gcp_instances", "r.instance_id", "r.name", "r.native_status", "r.zone", "zone", `jsonb_build_object('instance_type',r.instance_type,'network_interfaces',r.network_interfaces)`, "r.native_details"),
}

// State retains native status separately and maps only established provider values.
func State(provider, native string) string {
	state := strings.ToLower(native)
	if provider == "gcp" {
		switch state {
		case "terminated":
			return "stopped"
		case "provisioning", "staging", "repairing":
			return "pending"
		case "suspending":
			return "stopping"
		}
	}
	switch state {
	case "running", "stopped", "suspended", "pending", "stopping", "error", "terminated":
		return state
	case "starting", "rebooting":
		return "pending"
	case "shutting-down":
		return "stopping"
	}
	return "unknown"
}

func instanceFrom(row projectionRow) (Instance, error) {
	item := Instance{}
	if err := json.Unmarshal(row.Facts, &item); err != nil {
		return item, err
	}
	if err := json.Unmarshal(row.NativeDetails, &item.NativeDetails); err != nil {
		return item, err
	}
	item.Base = row.base("instance")
	item.InstanceID = item.NativeID
	item.State = State(item.Provider, item.NativeStatus)
	if item.NetworkInterfaces == nil {
		item.NetworkInterfaces = []resource.NetworkInterface{}
	}
	return item, nil
}

// Safe address validation keeps malformed optional stored entries from failing a read.
// PostgreSQL inet handles equivalent IPv6 spellings. Unmap IPv4-mapped values on both sides.
const exactAddress = `CASE WHEN pg_input_is_valid(address.value, 'inet') THEN
 CASE WHEN address.value::inet <<= '::ffff:0:0/96'::inet
 THEN substring(host(address.value::inet) from 8)::inet ELSE address.value::inet END END = ?::inet`

func instanceConditions(f Filter) (string, []any) {
	clauses := []string{}
	args := []any{}
	if f.State != "" {
		clauses = append(clauses, `CASE WHEN provider='gcp' AND lower(native_status)='terminated' THEN 'stopped' WHEN provider='gcp' AND lower(native_status) IN ('provisioning','staging','repairing') THEN 'pending' WHEN provider='gcp' AND lower(native_status)='suspending' THEN 'stopping' WHEN lower(native_status) IN ('starting','rebooting') THEN 'pending' WHEN lower(native_status)='shutting-down' THEN 'stopping' WHEN lower(native_status) IN ('running','stopped','suspended','pending','stopping','error','terminated') THEN lower(native_status) ELSE 'unknown' END = ?`)
		args = append(args, f.State)
	}
	if f.IP != "" {
		address, _ := netip.ParseAddr(f.IP)
		arrays := `COALESCE(nic->'private_ipv4','[]'::jsonb) || COALESCE(nic->'private_ipv6','[]'::jsonb)`
		if !f.PrivateIPOnly {
			arrays += ` || COALESCE(nic->'public_ipv4','[]'::jsonb) || COALESCE(nic->'public_ipv6','[]'::jsonb)`
		}
		nicWhere := exactAddress
		args = append(args, address.Unmap().String())
		if f.NetworkRef != "" {
			nicWhere += ` AND nic->>'network_ref' = ?`
			args = append(args, f.NetworkRef)
		}
		clauses = append(clauses, `EXISTS (SELECT 1 FROM jsonb_array_elements(facts->'network_interfaces') nic CROSS JOIN LATERAL jsonb_array_elements_text(`+arrays+`) address(value) WHERE `+nicWhere+`)`)
	} else if f.NetworkRef != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM jsonb_array_elements(facts->'network_interfaces') nic WHERE nic->>'network_ref' = ?)`)
		args = append(args, f.NetworkRef)
	}
	return strings.Join(clauses, " AND "), args
}

func (r *Repo) ListInstances(ctx context.Context, f Filter) ([]Instance, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	extra, args := instanceConditions(f)
	rows, total, err := r.page(ctx, instanceSources, f, extra, args)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Instance, len(rows))
	for i, row := range rows {
		items[i], err = instanceFrom(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (r *Repo) FindInstance(ctx context.Context, key string, includeDeleted bool) (*Instance, error) {
	row, err := r.find(ctx, instanceSources, key, "instance", includeDeleted)
	if err != nil {
		return nil, err
	}
	item, err := instanceFrom(*row)
	return &item, err
}
