package aliyun

import (
	"context"
	"strconv"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	rds "github.com/alibabacloud-go/rds-20140815/v17/client"
	"github.com/alibabacloud-go/tea/dara"
	"golang.org/x/sync/errgroup"
)

// The pinned RDS SDK has no WithContext API; bind each call through HttpClient.
func rdsRequest[T any](ctx context.Context, p *Provider, key *credentials.CloudKey, region string, call func(*rds.Client) (*T, error)) (*T, error) {
	ctx, cancel := context.WithTimeout(ctx, p.options.RequestTimeout)
	defer cancel()
	cfg, err := p.config(ctx, key, region)
	if err != nil {
		return nil, err
	}
	client, err := rds.NewClient(cfg)
	if err != nil {
		return nil, safeError(err)
	}
	result, err := call(client)
	if ctx.Err() != nil {
		return nil, safeError(ctx.Err())
	}
	if err != nil {
		return nil, safeError(err)
	}
	return result, nil
}

// Workers share the unit cancellation budget and finish before returning any error.
func collectDetails(ctx context.Context, count int, call func(context.Context, int) error) error {
	group, ctx := errgroup.WithContext(ctx)
	for worker := 0; worker < 4 && worker < count; worker++ {
		group.Go(func() error {
			for index := worker; index < count; index += 4 {
				if err := ctx.Err(); err != nil {
					return safeError(err)
				}
				if err := call(ctx, index); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return group.Wait()
}

func (p *Provider) databases(ctx context.Context, key *credentials.CloudKey, region string) ([]models.AliyunDatabase, error) {
	ids := []string{}
	seen := map[string]bool{}
	var total int32
	for page := int32(1); ; page++ {
		response, err := rdsRequest(ctx, p, key, region, func(client *rds.Client) (*rds.DescribeDBInstancesResponse, error) {
			return client.DescribeDBInstancesWithOptions(&rds.DescribeDBInstancesRequest{RegionId: dara.String(region), PageNumber: dara.Int32(page), PageSize: dara.Int32(100)}, runtimeOptions())
		})
		if err != nil {
			return nil, err
		}
		if response == nil || response.Body == nil || response.Body.Items == nil {
			return nil, configError()
		}
		body := response.Body
		count := len(body.Items.DBInstance)
		if body.PageNumber == nil || *body.PageNumber != page || body.PageRecordCount == nil || int(*body.PageRecordCount) != count || count > 100 || body.TotalRecordCount == nil || *body.TotalRecordCount < 0 || dara.StringValue(body.NextToken) != "" || page > 10000 {
			return nil, configError()
		}
		if page == 1 {
			total = *body.TotalRecordCount
		} else if total != *body.TotalRecordCount {
			return nil, configError()
		}
		if len(ids)+count > int(total) || (count == 0 && len(ids) < int(total)) {
			return nil, configError()
		}
		for _, item := range body.Items.DBInstance {
			if item == nil {
				return nil, configError()
			}
			id := dara.StringValue(item.DBInstanceId)
			if id == "" || seen[id] || (item.RegionId != nil && *item.RegionId != region) {
				return nil, configError()
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) == int(total) {
			break
		}
	}
	rows := make([]models.AliyunDatabase, len(ids))
	err := collectDetails(ctx, len(ids), func(ctx context.Context, index int) error {
		row, err := p.databaseDetail(ctx, key, region, ids[index])
		if err != nil {
			return err
		}
		rows[index] = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (p *Provider) databaseDetail(ctx context.Context, key *credentials.CloudKey, region, id string) (models.AliyunDatabase, error) {
	row := models.AliyunDatabase{}
	response, err := rdsRequest(ctx, p, key, region, func(client *rds.Client) (*rds.DescribeDBInstanceAttributeResponse, error) {
		return client.DescribeDBInstanceAttributeWithOptions(&rds.DescribeDBInstanceAttributeRequest{DBInstanceId: dara.String(id)}, runtimeOptions())
	})
	if err != nil {
		return row, err
	}
	if response == nil || response.Body == nil || response.Body.Items == nil || len(response.Body.Items.DBInstanceAttribute) != 1 {
		return row, configError()
	}
	item := response.Body.Items.DBInstanceAttribute[0]
	if item == nil || dara.StringValue(item.DBInstanceId) != id || (item.RegionId != nil && *item.RegionId != region) {
		return row, configError()
	}
	details := query.DatabaseNativeDetails{Category: dara.StringValue(item.Category), InstanceType: dara.StringValue(item.DBInstanceType), StorageType: dara.StringValue(item.DBInstanceStorageType), MemoryMB: item.DBInstanceMemory}
	if item.DBInstanceCPU != nil && *item.DBInstanceCPU != "" {
		n, err := strconv.ParseInt(*item.DBInstanceCPU, 10, 64)
		if err != nil || n < 0 {
			return row, configError()
		}
		details.CPUCount = &n
	}
	if details.MemoryMB != nil && *details.MemoryMB < 0 {
		return row, configError()
	}
	row = models.AliyunDatabase{DatabaseID: id, Name: dara.StringValue(item.DBInstanceDescription), Product: "rds", Engine: dara.StringValue(item.Engine), EngineVersion: dara.StringValue(item.EngineVersion), InstanceClass: dara.StringValue(item.DBInstanceClass), NativeStatus: dara.StringValue(item.DBInstanceStatus), Region: region, Zone: dara.StringValue(item.ZoneId), Tags: jsonValue(map[string]string{}), NativeDetails: jsonValue(details)}
	if item.DBInstanceStorage != nil {
		n := int64(*item.DBInstanceStorage)
		if n < 0 {
			return row, configError()
		}
		row.StorageGB = &n
	}
	if item.Category != nil && *item.Category != "" {
		category := *item.Category
		row.HAMode = &category
	}
	net, err := rdsRequest(ctx, p, key, region, func(client *rds.Client) (*rds.DescribeDBInstanceNetInfoResponse, error) {
		return client.DescribeDBInstanceNetInfoWithOptions(&rds.DescribeDBInstanceNetInfoRequest{DBInstanceId: dara.String(id)}, runtimeOptions())
	})
	if err != nil {
		return row, err
	}
	if net == nil || net.Body == nil || net.Body.DBInstanceNetInfos == nil {
		return row, configError()
	}
	endpoints := []resource.DatabaseEndpoint{}
	seen := map[string]bool{}
	knownPublic := true
	public := false
	for _, endpoint := range net.Body.DBInstanceNetInfos.DBInstanceNetInfo {
		if endpoint == nil {
			return row, configError()
		}
		address := dara.StringValue(endpoint.ConnectionString)
		if address == "" {
			address = dara.StringValue(endpoint.IPAddress)
		}
		if address == "" {
			return row, configError()
		}
		port, err := endpointPort(endpoint.Port)
		if err != nil {
			return row, err
		}
		nativeType := dara.StringValue(endpoint.IPType)
		identity := address + "/" + dara.StringValue(endpoint.Port) + "/" + nativeType
		if seen[identity] {
			return row, configError()
		}
		seen[identity] = true
		endpoints = append(endpoints, resource.DatabaseEndpoint{Address: address, Port: port, Type: nativeType})
		switch strings.ToLower(nativeType) {
		case "public":
			public = true
		case "private", "inner":
		default:
			knownPublic = false
		}
	}
	if public || knownPublic && len(endpoints) > 0 {
		row.PubliclyAccessible = &public
	}
	row.Endpoints = jsonValue(endpoints)
	return row, nil
}

func endpointPort(raw *string) (*int32, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(*raw, 10, 32)
	if err != nil || n < 1 || n > 65535 {
		return nil, configError()
	}
	v := int32(n)
	return &v, nil
}
