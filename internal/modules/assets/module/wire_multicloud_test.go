//go:build dbtest

package module

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/ArkGravity/optimus/internal/infra/crypto"
	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
	"github.com/ArkGravity/optimus/tests/dbtest"
)

func TestModuleCanonicalReadsUseJWTAndResourcePermission(t *testing.T) {
	gdb, closeDB := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(closeDB)
	key := dbtest.SeedCloudKey(t, gdb, "canonical-http")
	aws := dbtest.SeedCloudAccount(t, gdb, key.ID, "aws", "us-east-1")
	gcp := dbtest.SeedCloudAccount(t, gdb, key.ID, "gcp", "us-central1")
	require.NoError(t, gdb.Model(gcp).Updates(map[string]any{"provider": "gcp", "project_id": "project"}).Error)
	now := time.Now().UTC()
	require.NoError(t, gdb.Create(&models.AWSInstance{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", InstanceID: "i-aws", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPInstance{ID: 42, CloudAccountID: gcp.ID, Region: "us-central1", Zone: "us-central1-a", InstanceID: "123", LastSeenAt: now, NativeDetails: datatypes.JSON(`{"startup_script":"secret-must-not-leak"}`)}).Error)
	require.NoError(t, gdb.Create(&models.AWSVPC{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", VPCID: "vpc-aws", LastSeenAt: now}).Error)
	network := "projects/host/global/networks/main"
	require.NoError(t, gdb.Create(&models.GCPNetwork{ID: 42, CloudAccountID: gcp.ID, NetworkID: "123", NativeRef: network, LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AWSSubnet{CloudAccountID: aws.ID, Region: "us-east-1", VPCID: "vpc-aws", SubnetID: "subnet-aws", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPSubnet{CloudAccountID: gcp.ID, Region: "us-central1", NetworkRef: network, SubnetID: "subnet-gcp", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPSubnet{CloudAccountID: gcp.ID, Region: "us-central1", NetworkRef: "123", SubnetID: "wrong-network", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AWSDatabase{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", DBInstanceID: "db-aws", LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPDatabase{ID: 42, CloudAccountID: gcp.ID, Region: "us-central1", DatabaseID: "db-gcp", NativeDetails: datatypes.JSON(`{"password":"secret-must-not-leak"}`), LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AWSSecurityGroup{ID: 42, CloudAccountID: aws.ID, Region: "us-east-1", GroupID: "sg-aws", InboundRules: datatypes.JSON(`[{"protocol":"icmp","from_port":0,"to_port":0,"target_type":"security_group","target":"sg-peer","owner_id":"owner","vpc_id":"vpc-peer","peering_id":"pcx-peer"}]`), LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.GCPFirewall{ID: 42, CloudAccountID: gcp.ID, FirewallID: "123", NetworkRef: network, InboundRules: datatypes.JSON(`[{"direction":"inbound","protocol":"tcp","action":"deny","priority":1000,"enabled":false,"source_ranges":["10.0.0.0/8"],"target_network_tags":["web"]}]`), LastSeenAt: now}).Error)

	allowed := models.User{Username: "reader", Email: "reader@example.test", PasswordHash: "unused", Status: "enabled"}
	denied := models.User{Username: "denied", Email: "denied@example.test", PasswordHash: "unused", Status: "enabled"}
	require.NoError(t, gdb.Create(&allowed).Error)
	require.NoError(t, gdb.Create(&denied).Error)
	role := models.Role{Code: "asset-reader", Name: "reader"}
	require.NoError(t, gdb.Create(&role).Error)
	permission := models.Permission{Code: permissions.PermAssetsResourceRead, Name: "read", Category: "assets"}
	require.NoError(t, gdb.Create(&permission).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: allowed.ID, RoleID: role.ID}).Error)
	require.NoError(t, gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error)
	signer := crypto.NewJWTSigner("canonical-route-test-secret-at-least-32")
	allowedToken, err := signer.Sign(crypto.JWTClaims{UserID: allowed.ID}, time.Hour)
	require.NoError(t, err)
	deniedToken, err := signer.Sign(crypto.JWTClaims{UserID: denied.ID}, time.Hour)
	require.NoError(t, err)
	m := Wire(context.Background(), Input{DB: gdb})
	t.Cleanup(func() { require.NoError(t, m.Shutdown(context.Background())) })
	router := gin.New()
	m.MountRoutes(router.Group("/api/v1", middleware.JWTAuth(signer)), rbac.NewPermissionCache(gdb, time.Minute))
	request := func(path, token string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/assets"+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(rec, req)
		return rec
	}
	data := func(rec *httptest.ResponseRecorder) map[string]any {
		var body struct {
			Code int            `json:"code"`
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Zero(t, body.Code)
		return body.Data
	}
	for _, family := range []struct{ plural, singular string }{{"instances", "instance"}, {"vpcs", "vpc"}, {"databases", "database"}, {"firewalls", "firewall"}} {
		for _, suffix := range []string{"", "/gcp:" + family.singular + ":42"} {
			path := "/" + family.plural + suffix
			require.Equal(t, 401, request(path, "").Code, path)
			require.Equal(t, 403, request(path, deniedToken).Code, path)
			rec := request(path, allowedToken)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "secret-must-not-leak")
		}
		for _, asset := range []struct{ key, provider string }{{"gcp:" + family.singular + ":42", "gcp"}, {"aws:" + family.singular + ":42", "aws"}, {"42", "aws"}} {
			rec := request("/"+family.plural+"/"+asset.key, allowedToken)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			body := data(rec)
			require.Equal(t, asset.provider, body["provider"])
			require.Equal(t, asset.provider+":"+family.singular+":42", body["asset_key"])
		}
		rec := request("/"+family.plural+"/gcp:subnet:42", allowedToken)
		require.Equal(t, 400, rec.Code, rec.Body.String())
	}
	for _, parent := range []struct{ key, child string }{{"42", "subnet-aws"}, {"gcp:vpc:42", "subnet-gcp"}} {
		path := "/vpcs/" + parent.key + "/subnets"
		require.Equal(t, 401, request(path, "").Code)
		require.Equal(t, 403, request(path, deniedToken).Code)
		rec := request(path, allowedToken)
		require.Equal(t, 200, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), parent.child)
		require.NotContains(t, rec.Body.String(), "wrong-network")
	}
	rec := request("/firewalls/aws:firewall:42", allowedToken)
	require.Contains(t, rec.Body.String(), `"icmp_type":0`)
	require.Contains(t, rec.Body.String(), `"icmp_code":0`)
	require.Contains(t, rec.Body.String(), `"owner_id":"owner"`)
	require.Contains(t, rec.Body.String(), `"peering_id":"pcx-peer"`)
	rec = request("/firewalls/gcp:firewall:42", allowedToken)
	require.Contains(t, rec.Body.String(), `"action":"deny"`)
	require.Contains(t, rec.Body.String(), `"enabled":false`)
	rec = request("/firewalls", allowedToken)
	require.NotContains(t, rec.Body.String(), `"inbound_rules"`)
	require.NotContains(t, rec.Body.String(), `"aws_inbound_rule_facts"`)
	rec = request("/security-groups", allowedToken)
	require.Equal(t, 200, rec.Code)
	require.NotContains(t, rec.Body.String(), "gcp:firewall")
	require.Equal(t, 200, request("/security-groups/42", allowedToken).Code)
	require.Equal(t, 400, request("/security-groups/gcp:firewall:42", allowedToken).Code)
	require.NoError(t, gdb.Delete(&models.GCPDatabase{}, 42).Error)
	require.Equal(t, 404, request("/databases/gcp:database:42", allowedToken).Code)
	rec = request("/databases/gcp:database:42?include_deleted=true", allowedToken)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, true, data(rec)["deleted"])
}
