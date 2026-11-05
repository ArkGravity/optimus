package query

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

type DatabaseNativeDetails struct {
	PrimaryDNSName           string                 `json:"primary_dns_name,omitempty"`
	WriteEndpoint            string                 `json:"write_endpoint,omitempty"`
	IPv6Address              string                 `json:"ipv6_address,omitempty"`
	SecondaryGCEZone         string                 `json:"secondary_gce_zone,omitempty"`
	GCPIPAddresses           []GCPDatabaseIPAddress `json:"gcp_ip_addresses,omitempty"`
	GCPDNSNames              []GCPDatabaseDNSName   `json:"gcp_dns_names,omitempty"`
	DNSName                  string                 `json:"dns_name,omitempty"`
	PSCServiceAttachmentLink string                 `json:"psc_service_attachment_link,omitempty"`
	SQLNetworkArchitecture   string                 `json:"sql_network_architecture,omitempty"`
	DatabaseInstalledVersion string                 `json:"database_installed_version,omitempty"`
	CPUCount                 *int64                 `json:"cpu_count,omitempty"`
	MemoryMB                 *int64                 `json:"memory_mb,omitempty"`
	InstanceType             string                 `json:"instance_type,omitempty"`
	StorageType              string                 `json:"storage_type,omitempty"`
	ConnectionName           string                 `json:"connection_name,omitempty"`
	AvailabilityType         string                 `json:"availability_type,omitempty"`
	Category                 string                 `json:"category,omitempty"`
}

// GCPDatabaseIPAddress retains the API type, including egress-only OUTGOING IPs.
type GCPDatabaseIPAddress struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

// GCPDatabaseDNSName preserves observed DNS provenance without inferring reachability.
type GCPDatabaseDNSName struct {
	Name           string `json:"name"`
	ConnectionType string `json:"connection_type,omitempty"`
	DNSScope       string `json:"dns_scope,omitempty"`
	RecordManager  string `json:"record_manager,omitempty"`
}
type Database struct {
	Base
	DBInstanceID       string                      `json:"db_instance_id"`
	Product            string                      `json:"product"`
	Engine             string                      `json:"engine"`
	EngineVersion      string                      `json:"engine_version"`
	InstanceClass      string                      `json:"instance_class"`
	Status             string                      `json:"status"`
	Endpoints          []resource.DatabaseEndpoint `json:"endpoints"`
	Endpoint           string                      `json:"endpoint"`
	Port               *int32                      `json:"port,omitempty"`
	MultiAZ            *bool                       `json:"multi_az,omitempty"`
	HAMode             *string                     `json:"ha_mode"`
	PubliclyAccessible *bool                       `json:"publicly_accessible"`
	StorageGB          *int64                      `json:"storage_gb"`
	NativeDetails      DatabaseNativeDetails       `json:"native_details"`
}

var databaseSources = map[string]string{
	"aws":    projection("aws", "aws_databases", "r.db_instance_id", "r.db_instance_id", "r.status", "''", "region", `jsonb_build_object('product','rds','engine',r.engine,'engine_version',r.engine_version,'instance_class',r.instance_class,'endpoints',CASE WHEN r.endpoint='' THEN '[]'::jsonb ELSE jsonb_build_array(jsonb_build_object('address',r.endpoint,'port',r.port,'type','endpoint')) END,'endpoint',r.endpoint,'port',r.port,'multi_az',r.multi_az,'ha_mode',CASE WHEN r.multi_az THEN 'multi_az' ELSE 'single_az' END,'publicly_accessible',r.publicly_accessible,'storage_gb',r.storage_gb)`, `'{}'::jsonb`),
	"aliyun": projection("aliyun", "aliyun_databases", "r.database_id", "r.name", "r.native_status", "r.zone", "region", `jsonb_build_object('product',r.product,'engine',r.engine,'engine_version',r.engine_version,'instance_class',r.instance_class,'endpoints',r.endpoints,'ha_mode',r.ha_mode,'publicly_accessible',r.publicly_accessible,'storage_gb',r.storage_gb)`, "r.native_details"),
	"gcp":    projection("gcp", "gcp_databases", "r.database_id", "r.name", "r.native_status", "r.zone", "region", `jsonb_build_object('product',r.product,'engine',r.engine,'engine_version',r.engine_version,'instance_class',r.instance_class,'endpoints',r.endpoints,'ha_mode',r.ha_mode,'publicly_accessible',r.publicly_accessible,'storage_gb',r.storage_gb)`, "r.native_details"),
}

func databaseFrom(row projectionRow) (Database, error) {
	item := Database{}
	if err := json.Unmarshal(row.Facts, &item); err != nil {
		return item, err
	}
	if err := json.Unmarshal(row.NativeDetails, &item.NativeDetails); err != nil {
		return item, err
	}
	item.Base = row.base("database")
	item.DBInstanceID = item.NativeID
	item.Status = item.NativeStatus
	if item.Endpoints == nil {
		item.Endpoints = []resource.DatabaseEndpoint{}
	}
	return item, nil
}
func (r *Repo) ListDatabases(ctx context.Context, f Filter) ([]Database, int64, error) {
	clauses := []string{}
	args := []any{}
	if f.Engine != "" {
		clauses = append(clauses, "facts->>'engine' = ?")
		args = append(args, f.Engine)
	}
	if f.Status != "" {
		clauses = append(clauses, "native_status = ?")
		args = append(args, f.Status)
	}
	rows, total, err := r.page(ctx, databaseSources, f, strings.Join(clauses, " AND "), args)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Database, len(rows))
	for i, row := range rows {
		items[i], err = databaseFrom(row)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}
func (r *Repo) FindDatabase(ctx context.Context, key string, includeDeleted bool) (*Database, error) {
	row, err := r.find(ctx, databaseSources, key, "database", includeDeleted)
	if err != nil {
		return nil, err
	}
	item, err := databaseFrom(*row)
	return &item, err
}
