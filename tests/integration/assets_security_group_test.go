//go:build dbtest

package integration_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ArkGravity/optimus/internal/infra/crypto"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/tests/dbtest"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestAssetsSecurityGroupHTTPReadPermissionAndRules(t *testing.T) {
	router, gdb := setupServerWithAssetsWiring(t, &assetsTestWiring{
		Factory: integrationClientFactory{}, Fetcher: &integrationFetcher{}, Consumer: integrationCloudKeyConsumer{},
	})
	key := dbtest.SeedCloudKey(t, gdb, "security-group-http")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "firewall", "us-east-1")
	group := models.AWSSecurityGroup{CloudAccountID: account.ID, Region: "us-east-1", GroupID: "sg-http", GroupName: "web", VPCID: "vpc-1", LastSeenAt: time.Now(), InboundRules: datatypes.JSON(`[{"protocol":"icmp","from_port":8,"to_port":0,"target_type":"ipv4","target":"10.0.0.0/8","description":"office"}]`)}
	require.NoError(t, gdb.Create(&group).Error)
	hash, err := crypto.HashPassword("Viewer-Pass!", 4)
	require.NoError(t, err)
	user := models.User{Username: "sg-reader", Email: "sg-reader@example.com", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(&user).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: user.ID, RoleID: viewer.ID}).Error)
	token := login(t, router, user.Username, "Viewer-Pass!")
	paths := []string{"/api/v1/assets/security-groups?account_id=" + fmt.Sprint(account.ID) + "&vpc_id=vpc-1", fmt.Sprintf("/api/v1/assets/security-groups/%d", group.ID)}
	for _, path := range paths {
		rec := assetsRequest(t, router, "", http.MethodGet, path, nil)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		rec = assetsRequest(t, router, token, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "sg-http")
	}
	rec := assetsRequest(t, router, token, http.MethodGet, paths[1], nil)
	data := bodyMap(t, rec)["data"].(map[string]any)
	rule := data["inbound_rules"].([]any)[0].(map[string]any)
	require.EqualValues(t, 0, rule["to_port"])
	require.Equal(t, "office", rule["description"])
	require.Empty(t, data["outbound_rules"])
	denied := models.User{Username: "sg-denied", Email: "sg-denied@example.com", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(&denied).Error)
	token = login(t, router, denied.Username, "Viewer-Pass!")
	for _, path := range paths {
		rec = assetsRequest(t, router, token, http.MethodGet, path, nil)
		require.Equal(t, http.StatusForbidden, rec.Code)
	}
}
