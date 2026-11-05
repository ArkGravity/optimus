//go:build dbtest

package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	assetsmodule "github.com/ArkGravity/optimus/internal/modules/assets/module"
	aliyun "github.com/ArkGravity/optimus/internal/modules/assets/providers/aliyun"
	aws "github.com/ArkGravity/optimus/internal/modules/assets/providers/aws"
	gcp "github.com/ArkGravity/optimus/internal/modules/assets/providers/gcp"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	assetsync "github.com/ArkGravity/optimus/internal/modules/assets/sync"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

// These fixtures replace cloud I/O only. HTTP credentials, the real vault
// consumer, engine scope gates, transactions and provider publishers stay wired.
type multicloudCatalog struct{ regions []string }

func (c multicloudCatalog) Regions(_ context.Context, key *credentials.CloudKey, project string) ([]account.RegionOption, error) {
	if err := checkMulticloudKey(key); err != nil {
		return nil, err
	}
	if key.Provider == "gcp" && project != "target-project" {
		return nil, errors.New("wrong target project")
	}
	out := make([]account.RegionOption, 0, len(c.regions))
	for _, r := range c.regions {
		out = append(out, account.RegionOption{ID: r, Name: r})
	}
	return out, nil
}
func checkMulticloudKey(key *credentials.CloudKey) error {
	if key == nil {
		return errors.New("missing real vault key")
	}
	if key.Provider == "gcp" {
		if key.AuthType != "service_account_json" || !strings.Contains(string(key.ServiceAccountJSON), "fixture@credential-home.iam.gserviceaccount.com") {
			return errors.New("missing decrypted SA JSON")
		}
		return nil
	}
	if key.AuthType != "access_key" || key.AccessKeyID != "fixture-access-"+key.Provider || key.SecretAccessKey != "fixture-secret-"+key.Provider {
		return errors.New("missing decrypted access key")
	}
	return nil
}

type multicloudAWSFactory struct{}

func (multicloudAWSFactory) For(_ context.Context, key *credentials.CloudKey, _ string, _ time.Duration) (*assetsync.Clients, error) {
	return &assetsync.Clients{}, checkMulticloudKey(key)
}

type multicloudAWSFetcher struct{}

func (multicloudAWSFetcher) FetchInstances(context.Context, *assetsync.Clients) ([]models.AWSInstance, error) {
	return []models.AWSInstance{{InstanceID: "i-fixture", State: "running", VPCID: "vpc-fixture", SubnetID: "subnet-fixture"}}, nil
}
func (multicloudAWSFetcher) FetchVPCsAndSubnets(context.Context, *assetsync.Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	return []models.AWSVPC{{VPCID: "vpc-fixture"}}, []models.AWSSubnet{{SubnetID: "subnet-fixture", VPCID: "vpc-fixture"}}, nil
}
func (multicloudAWSFetcher) FetchDatabases(context.Context, *assetsync.Clients) ([]models.AWSDatabase, error) {
	return []models.AWSDatabase{{DBInstanceID: "db-fixture", Engine: "postgres"}}, nil
}
func (multicloudAWSFetcher) FetchSecurityGroups(context.Context, *assetsync.Clients) ([]models.AWSSecurityGroup, error) {
	return []models.AWSSecurityGroup{{GroupID: "sg-fixture", VPCID: "vpc-fixture"}}, nil
}

type multicloudNativeProvider struct{ provider string }

func (p multicloudNativeProvider) Name() string { return p.provider }
func (p multicloudNativeProvider) Plan(_ context.Context, row models.CloudAccount, key *credentials.CloudKey) (assetsync.Plan, error) {
	if err := checkMulticloudKey(key); err != nil {
		return assetsync.Plan{}, err
	}
	plan := assetsync.Plan{ExternalAccountID: "fixture-aliyun-account"}
	add := func(family, kind, location, region, zone string) {
		plan.Units = append(plan.Units, assetsync.Unit{Key: family + "/" + location, ResourceType: family, LocationType: kind, Location: location, Region: region, Zone: zone, Budget: time.Second})
	}
	if p.provider == "gcp" {
		if row.ProjectID != "target-project" {
			return assetsync.Plan{}, errors.New("wrong target")
		}
		plan.ExternalAccountID = ""
		add("network", "global", "global", "", "")
		add("firewall", "global", "global", "", "")
	}
	for _, region := range row.EnabledRegions {
		if p.provider == "gcp" {
			add("subnet", "region", region, region, "")
			add("instance", "zone", region+"-a", region, region+"-a")
			add("database", "region", region, region, "")
		} else {
			for _, family := range []string{"instance", "network", "database", "security_group"} {
				add(family, "region", region, region, "")
			}
		}
	}
	return plan, nil
}
func (p multicloudNativeProvider) Fetch(_ context.Context, u assetsync.Unit, key *credentials.CloudKey) (assetsync.Snapshot, error) {
	if err := checkMulticloudKey(key); err != nil {
		return assetsync.Snapshot{}, err
	}
	started := time.Now()
	loc := resource.Location{Type: u.LocationType, Name: u.Location, Region: u.Region, Zone: u.Zone}
	return assetsync.Snapshot{Complete: true, Publish: func(ctx context.Context, tx *gorm.DB) (assetsync.Stats, error) {
		var seen, deleted int64
		var err error
		if p.provider == "aliyun" {
			switch u.ResourceType {
			case "instance":
				seen, deleted, err = aliyun.PublishInstances(ctx, tx, u.Scope, loc, started, []models.AliyunInstance{{InstanceID: "i-fixture", NativeStatus: "Running", Zone: u.Region + "-a"}})
			case "network":
				seen, deleted, err = aliyun.PublishNetwork(ctx, tx, u.Scope, loc, started, []models.AliyunVPC{{NetworkID: "vpc-fixture", NativeRef: "vpc-fixture"}}, []models.AliyunVSwitch{{SubnetID: "vsw-fixture", NetworkRef: "vpc-fixture"}})
			case "database":
				seen, deleted, err = aliyun.PublishDatabases(ctx, tx, u.Scope, loc, started, []models.AliyunDatabase{{DatabaseID: "db-fixture", Engine: "MySQL"}})
			case "security_group":
				seen, deleted, err = aliyun.PublishSecurityGroups(ctx, tx, u.Scope, loc, started, []models.AliyunSecurityGroup{{FirewallID: "sg-fixture", NetworkRef: "vpc-fixture"}})
			}
		} else {
			ref := "projects/target-project/global/networks/fixture"
			switch u.ResourceType {
			case "instance":
				seen, deleted, err = gcp.PublishInstances(ctx, tx, u.Scope, loc, started, []models.GCPInstance{{InstanceID: "9007199254740993999", NativeStatus: "RUNNING"}})
			case "network":
				seen, deleted, err = gcp.PublishNetworks(ctx, tx, u.Scope, loc, started, []models.GCPNetwork{{NetworkID: "9007199254740993998", NativeRef: ref}})
			case "subnet":
				seen, deleted, err = gcp.PublishSubnets(ctx, tx, u.Scope, loc, started, []models.GCPSubnet{{SubnetID: "subnet-fixture", NetworkRef: ref}})
			case "database":
				seen, deleted, err = gcp.PublishDatabases(ctx, tx, u.Scope, loc, started, []models.GCPDatabase{{DatabaseID: "sql-fixture"}})
			case "firewall":
				seen, deleted, err = gcp.PublishFirewalls(ctx, tx, u.Scope, loc, started, []models.GCPFirewall{{FirewallID: "firewall-fixture", NetworkRef: ref}})
			}
		}
		return assetsync.Stats{Seen: seen, SoftDeleted: deleted}, err
	}}, nil
}

func multicloudSA(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]string{"type": "service_account", "project_id": "credential-home", "client_email": "fixture@credential-home.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))})
	require.NoError(t, err)
	return string(raw)
}

func TestAssetsMulticloudHTTPLifecycle(t *testing.T) {
	registry, err := assetsync.NewRegistry(aws.New(aws.Options{ClientFactory: multicloudAWSFactory{}, Fetcher: multicloudAWSFetcher{}, Identity: integrationAWSIdentity{}}), multicloudNativeProvider{"aliyun"}, multicloudNativeProvider{"gcp"})
	require.NoError(t, err)
	catalogs := map[string]account.RegionCatalog{"aws": multicloudCatalog{[]string{"us-east-1", "eu-west-1"}}, "aliyun": multicloudCatalog{[]string{"cn-hangzhou", "cn-shanghai"}}, "gcp": multicloudCatalog{[]string{"us-central1", "us-east1"}}}
	var module *assetsmodule.Module
	r, gdb := setupServerWithAssetsWiring(t, &assetsTestWiring{Registry: registry, Catalogs: catalogs, AfterWire: func(m *assetsmodule.Module) { module = m }})
	token := login(t, r, "admin", "S3cret-Pass!")
	createViewerUser(t, gdb, "assets-viewer", "Viewer-Pass-1!")
	viewer := login(t, r, "assets-viewer", "Viewer-Pass-1!")
	for _, path := range []string{"instances", "vpcs", "databases", "firewalls", "sync-runs", "cloud-accounts"} {
		require.Equal(t, http.StatusUnauthorized, assetsRequest(t, r, "", http.MethodGet, "/api/v1/assets/"+path, nil).Code)
	}
	for _, tc := range []struct{ provider, first, second string }{{"aws", "us-east-1", "eu-west-1"}, {"aliyun", "cn-hangzhou", "cn-shanghai"}, {"gcp", "us-central1", "us-east1"}} {
		t.Run(tc.provider, func(t *testing.T) {
			keyBody := map[string]any{"name": "http-" + tc.provider, "provider": tc.provider, "access_key_id": "fixture-access-" + tc.provider, "secret_access_key": "fixture-secret-" + tc.provider}
			if tc.provider == "gcp" {
				delete(keyBody, "access_key_id")
				delete(keyBody, "secret_access_key")
				keyBody["service_account_json"] = multicloudSA(t)
			}
			request := func(method, path string, body any) map[string]any {
				rec := assetsRequest(t, r, token, method, path, body)
				require.Equalf(t, http.StatusOK, rec.Code, "%s %s: %s", method, path, rec.Body.String())
				return bodyMap(t, rec)
			}
			keyData := request(http.MethodPost, "/api/v1/credentials/cloud-keys", keyBody)["data"].(map[string]any)
			keyID := uint64(keyData["id"].(float64))
			keyPath := fmt.Sprintf("/api/v1/credentials/cloud-keys/%d", keyID)
			keyJSON, err := json.Marshal(keyData)
			require.NoError(t, err)
			require.NotContains(t, string(keyJSON), "fixture-secret")
			require.NotContains(t, string(keyJSON), "PRIVATE KEY")
			var stored models.CredentialCloudKey
			require.NoError(t, gdb.First(&stored, keyID).Error)
			if tc.provider == "gcp" {
				require.NotEmpty(t, stored.ServiceAccountJSONEnc)
				require.NotContains(t, string(stored.ServiceAccountJSONEnc), "PRIVATE KEY")
			} else {
				require.NotEmpty(t, stored.AccessKeyIDEnc)
				require.NotEqual(t, []byte(keyBody["secret_access_key"].(string)), stored.SecretAccessKeyEnc)
			}
			project := ""
			if tc.provider == "gcp" {
				project = "target-project"
			}
			catalogPath := fmt.Sprintf("/api/v1/assets/cloud-accounts/region-options?provider=%s&cloudkey_id=%d&project_id=%s", tc.provider, keyID, project)
			require.Len(t, request(http.MethodGet, catalogPath, nil)["data"].(map[string]any)["items"], 2)
			require.Equal(t, http.StatusForbidden, assetsRequest(t, r, viewer, http.MethodGet, catalogPath, nil).Code)
			accountData := request(http.MethodPost, "/api/v1/assets/cloud-accounts", map[string]any{"name": "http-" + tc.provider, "provider": tc.provider, "project_id": project, "cloudkey_id": keyID, "enabled_regions": []string{tc.first, tc.second}})["data"].(map[string]any)
			accountID := uint64(accountData["id"].(float64))
			accountPath := fmt.Sprintf("/api/v1/assets/cloud-accounts/%d", accountID)
			require.Equal(t, http.StatusForbidden, assetsRequest(t, r, viewer, http.MethodPost, accountPath+"/sync", nil).Code)
			blocked := assetsRequest(t, r, token, http.MethodDelete, keyPath, nil)
			require.Equal(t, http.StatusConflict, blocked.Code)
			require.EqualValues(t, 43001, bodyMap(t, blocked)["code"])
			expected := 9
			syncAndWait := func(total int) {
				request(http.MethodPost, accountPath+"/sync", nil)
				require.Eventually(t, func() bool {
					var rows []models.AssetsSyncRun
					if gdb.Where("cloud_account_id = ?", accountID).Find(&rows).Error != nil || len(rows) != total {
						return false
					}
					for _, row := range rows {
						if row.Status != "success" || row.FinishedAt == nil {
							return false
						}
					}
					return true
				}, 5*time.Second, 10*time.Millisecond)
			}
			syncAndWait(expected)
			for _, family := range []string{"instances", "vpcs", "databases", "firewalls"} {
				path := fmt.Sprintf("/api/v1/assets/%s?provider=%s&account_id=%d", family, tc.provider, accountID)
				data := request(http.MethodGet, path, nil)["data"].(map[string]any)
				items := data["items"].([]any)
				require.NotEmpty(t, items)
				for _, item := range items {
					row := item.(map[string]any)
					require.Equal(t, tc.provider, row["provider"])
					assetKey := row["asset_key"].(string)
					require.True(t, strings.HasPrefix(assetKey, tc.provider+":"))
					request(http.MethodGet, "/api/v1/assets/"+family+"/"+assetKey, nil)
					if family == "vpcs" {
						subnets := request(http.MethodGet, "/api/v1/assets/vpcs/"+assetKey+"/subnets", nil)["data"].(map[string]any)
						require.NotEmpty(t, subnets["items"])
					}
				}
			}
			var mutationCount int64
			countMutations := func() int64 {
				require.NoError(t, gdb.Model(&models.AuditLog{}).Where("action LIKE ?", "assets.%").Count(&mutationCount).Error)
				return mutationCount
			}
			beforeCron := countMutations()
			require.NoError(t, module.Engine.RunAccount(context.Background(), accountID, "cron", nil))
			require.Equal(t, beforeCron, countMutations())
			var consumeCount int64
			require.NoError(t, gdb.Model(&models.AuditLog{}).Where("action = ?", "credentials.consume").Count(&consumeCount).Error)
			require.Positive(t, consumeCount)
			request(http.MethodPut, accountPath, map[string]any{"enabled_regions": []string{tc.first}})
			for _, family := range []string{"instances", "databases"} {
				data := request(http.MethodGet, fmt.Sprintf("/api/v1/assets/%s?account_id=%d&region=%s", family, accountID, tc.second), nil)["data"].(map[string]any)
				require.EqualValues(t, 0, data["total"])
			}
			next := 5
			if tc.provider == "gcp" {
				next = 6
			}
			syncAndWait(expected*2 + next)
			var triggerAudits int64
			require.NoError(t, gdb.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", "assets.cloud_account.sync_trigger", fmt.Sprint(accountID)).Count(&triggerAudits).Error)
			require.EqualValues(t, 2, triggerAudits)
			request(http.MethodPut, accountPath, map[string]any{"enabled": false})
			blocked = assetsRequest(t, r, token, http.MethodDelete, keyPath, nil)
			require.Equal(t, http.StatusConflict, blocked.Code)
			require.EqualValues(t, 43001, bodyMap(t, blocked)["code"])
			var beforeHistory int64
			require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("cloud_account_id = ?", accountID).Count(&beforeHistory).Error)
			request(http.MethodDelete, accountPath, nil)
			request(http.MethodDelete, keyPath, nil)
			require.ErrorIs(t, gdb.Unscoped().First(&models.CredentialCloudKey{}, keyID).Error, gorm.ErrRecordNotFound)
			var detached models.CloudAccount
			require.NoError(t, gdb.Unscoped().First(&detached, accountID).Error)
			require.True(t, detached.DeletedAt.Valid)
			require.Zero(t, detached.CloudKeyID)
			history := request(http.MethodGet, fmt.Sprintf("/api/v1/assets/sync-runs?account_id=%d&size=200", accountID), nil)["data"].(map[string]any)
			require.EqualValues(t, beforeHistory, history["total"])
			for _, family := range []string{"instances", "vpcs", "databases", "firewalls"} {
				data := request(http.MethodGet, fmt.Sprintf("/api/v1/assets/%s?account_id=%d", family, accountID), nil)["data"].(map[string]any)
				require.EqualValues(t, 0, data["total"])
			}
		})
	}
}
