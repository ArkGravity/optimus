//go:build dbtest

package cloudkey_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArkGravity/optimus/internal/infra/advisorylock"
	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/account/inuse"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
)

func gcpRequest(t *testing.T) cloudkey.CreateRequest {
	t.Helper()
	return cloudkey.CreateRequest{Name: "gcp", Provider: "gcp", ServiceAccountJSON: string(serviceAccountFixture(t))}
}

func TestServiceAccountEncryptedCanonicalRoundTrip(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	fields := serviceAccountFields(t)
	fields["auth_uri"] = "https://discard.invalid"
	req := cloudkey.CreateRequest{Name: "gcp", Provider: "gcp", ServiceAccountJSON: string(marshalServiceAccount(t, fields))}
	detail, err := svc.Create(t.Context(), 0, "ip", "agent", req)
	require.NoError(t, err)
	require.Equal(t, "service_account_json", detail.AuthType)
	require.Equal(t, fields["client_email"], detail.ServiceAccountEmail)
	require.Equal(t, "test-project", detail.CredentialProjectID)
	require.False(t, detail.NeedsConfiguration)
	row, err := svc.Repo().Get(t.Context(), detail.ID)
	require.NoError(t, err)
	require.Nil(t, row.AccessKeyIDEnc)
	require.Nil(t, row.SecretAccessKeyEnc)
	require.Contains(t, string(row.ServiceAccountJSONEnc), "SEAL")
	canonical, _, err := cloudkey.ValidateServiceAccountJSON([]byte(req.ServiceAccountJSON))
	require.NoError(t, err)
	require.Equal(t, append([]byte("SEAL"), canonical...), row.ServiceAccountJSONEnc)
	consumed, err := svc.Consume(t.Context(), nil, detail.ID, "system:roundtrip")
	require.NoError(t, err)
	require.Equal(t, canonical, consumed.ServiceAccountJSON)
	require.Equal(t, "service_account_json", consumed.AuthType)
	require.Empty(t, consumed.AccessKeyID)
	require.Empty(t, consumed.SecretAccessKey)
	list, err := svc.List(t.Context(), cloudkey.ListQuery{})
	require.NoError(t, err)
	summary, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(summary), "PRIVATE KEY")
	require.NotContains(t, string(summary), "service_account_json_enc")
	var audits []models.AuditLog
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 2)
	for _, entry := range audits {
		require.NotContains(t, string(entry.Payload), "PRIVATE KEY")
		require.NotContains(t, string(entry.Payload), "discard.invalid")
	}
}

func TestServiceAccountAuthTypeSecretMatrix(t *testing.T) {
	svc, _, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	jsonSecret := string(serviceAccountFixture(t))
	for _, tc := range []struct {
		name  string
		req   cloudkey.CreateRequest
		valid bool
	}{
		{"gcp keys", cloudkey.CreateRequest{Provider: "gcp", AccessKeyID: "ak", SecretAccessKey: "sk"}, false},
		{"gcp json", cloudkey.CreateRequest{Provider: "gcp", ServiceAccountJSON: jsonSecret}, true},
		{"gcp mixed", cloudkey.CreateRequest{Provider: "gcp", ServiceAccountJSON: jsonSecret, AccessKeyID: "ak"}, false},
		{"gcp legacy", cloudkey.CreateRequest{Provider: "gcp", AuthType: "legacy_invalid", AccessKeyID: "ak", SecretAccessKey: "sk"}, false},
		{"aws missing", cloudkey.CreateRequest{Provider: "aws", AccessKeyID: "ak"}, false},
		{"aws mixed", cloudkey.CreateRequest{Provider: "aws", AccessKeyID: "ak", SecretAccessKey: "sk", ServiceAccountJSON: jsonSecret}, false},
		{"aws wrong type", cloudkey.CreateRequest{Provider: "aws", AuthType: "service_account_json", AccessKeyID: "ak", SecretAccessKey: "sk"}, false},
		{"aliyun keys", cloudkey.CreateRequest{Provider: "aliyun", AuthType: "access_key", AccessKeyID: "ak", SecretAccessKey: "sk"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Name = tc.name
			_, err := svc.Create(t.Context(), 0, "", "", tc.req)
			if tc.valid {
				require.NoError(t, err)
			} else {
				requireValidation(t, err)
			}
		})
	}
}

func TestServiceAccountLegacyMetadataRepairAndDelete(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	legacy := &models.CredentialCloudKey{Name: "legacy", Provider: "gcp", AuthType: "legacy_invalid", AccessKeyIDEnc: []byte("invalid ciphertext"), SecretAccessKeyEnc: []byte("invalid ciphertext")}
	require.NoError(t, db.Create(legacy).Error)
	detail, err := svc.Get(t.Context(), legacy.ID)
	require.NoError(t, err)
	require.True(t, detail.NeedsConfiguration)
	_, err = svc.Consume(t.Context(), nil, legacy.ID, "system:legacy")
	requireValidation(t, err)
	description := "metadata edit"
	detail, err = svc.Update(t.Context(), 0, "", "", legacy.ID, cloudkey.UpdateRequest{Description: &description})
	require.NoError(t, err)
	require.True(t, detail.NeedsConfiguration)
	require.Equal(t, description, detail.Description)
	valid := string(serviceAccountFixture(t))
	detail, err = svc.Update(t.Context(), 0, "", "", legacy.ID, cloudkey.UpdateRequest{ServiceAccountJSON: &valid})
	require.NoError(t, err)
	require.False(t, detail.NeedsConfiguration)
	require.Equal(t, "service_account_json", detail.AuthType)
	row, err := svc.Repo().Get(t.Context(), legacy.ID)
	require.NoError(t, err)
	require.Nil(t, row.AccessKeyIDEnc)
	require.Nil(t, row.SecretAccessKeyEnc)
	require.NoError(t, svc.Delete(t.Context(), 0, "", "", legacy.ID))
}

func TestServiceAccountRotationAndConversion(t *testing.T) {
	svc, _, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	detail, err := svc.Create(t.Context(), 0, "", "", gcpRequest(t))
	require.NoError(t, err)
	fields := serviceAccountFields(t)
	fields["project_id"] = "rotated-project"
	rotated := string(marshalServiceAccount(t, fields))
	_, err = svc.Update(t.Context(), 0, "", "", detail.ID, cloudkey.UpdateRequest{ServiceAccountJSON: &rotated})
	require.NoError(t, err)
	provider, ak, sk := "aws", "new-ak", "new-sk"
	_, err = svc.Update(t.Context(), 0, "", "", detail.ID, cloudkey.UpdateRequest{Provider: &provider})
	requireValidation(t, err)
	_, err = svc.Update(t.Context(), 0, "", "", detail.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak})
	requireValidation(t, err)
	_, err = svc.Update(t.Context(), 0, "", "", detail.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak, SecretAccessKey: &sk})
	require.NoError(t, err)
	row, err := svc.Repo().Get(t.Context(), detail.ID)
	require.NoError(t, err)
	require.Equal(t, "access_key", row.AuthType)
	require.Nil(t, row.ServiceAccountJSONEnc)
	require.Empty(t, row.ServiceAccountEmail)
	require.Empty(t, row.CredentialProjectID)
	provider = "gcp"
	_, err = svc.Update(t.Context(), 0, "", "", detail.ID, cloudkey.UpdateRequest{Provider: &provider, ServiceAccountJSON: &rotated})
	require.NoError(t, err)
}

func TestAuthTypeConversionRejectsLiveDisabledReferencesAndAllowsRotation(t *testing.T) {
	for _, wired := range []bool{false, true} {
		t.Run(strconv.FormatBool(wired), func(t *testing.T) {
			svc, db, cleanup := newSvcWithDB(t)
			t.Cleanup(cleanup)
			if wired {
				svc.SetAccountsInUseCounter(inuse.New(db))
			}
			key, err := svc.Create(t.Context(), 0, "", "", newReq())
			require.NoError(t, err)
			row := &models.CloudAccount{Name: "disabled", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
			require.NoError(t, db.Create(row).Error)
			require.NoError(t, db.Model(row).Update("enabled", false).Error)
			provider := "aliyun"
			ak, sk := "replacement-ak", "replacement-sk"
			_, err = svc.Update(t.Context(), 0, "", "", key.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak, SecretAccessKey: &sk})
			be, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
			_, err = svc.Update(t.Context(), 0, "", "", key.ID, cloudkey.UpdateRequest{SecretAccessKey: &sk})
			require.NoError(t, err)
		})
	}
}

func TestAuthTypeCreateAndRotationAuditFailureRollBack(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	key, err := svc.Create(t.Context(), 0, "", "", gcpRequest(t))
	require.NoError(t, err)
	before, err := svc.Repo().Get(t.Context(), key.ID)
	require.NoError(t, err)
	legacy := &models.CredentialCloudKey{Name: "legacy-rollback", Provider: "gcp", AuthType: "legacy_invalid", AccessKeyIDEnc: []byte("old-ak-ciphertext"), SecretAccessKeyEnc: []byte("old-sk-ciphertext")}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Exec(`CREATE FUNCTION reject_auth_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_auth_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_auth_audit()`).Error)
	req := gcpRequest(t)
	req.Name = "must-rollback"
	_, err = svc.Create(t.Context(), 0, "", "", req)
	requireDatabaseError(t, err)
	var count int64
	require.NoError(t, db.Model(&models.CredentialCloudKey{}).Where("name = ?", req.Name).Count(&count).Error)
	require.Zero(t, count)
	secret := string(serviceAccountFixture(t))
	_, err = svc.Update(t.Context(), 0, "", "", key.ID, cloudkey.UpdateRequest{ServiceAccountJSON: &secret})
	requireDatabaseError(t, err)
	after, err := svc.Repo().Get(t.Context(), key.ID)
	require.NoError(t, err)
	require.Equal(t, before.ServiceAccountJSONEnc, after.ServiceAccountJSONEnc)
	_, err = svc.Update(t.Context(), 0, "", "", legacy.ID, cloudkey.UpdateRequest{ServiceAccountJSON: &secret})
	requireDatabaseError(t, err)
	legacyAfter, err := svc.Repo().Get(t.Context(), legacy.ID)
	require.NoError(t, err)
	require.Equal(t, "legacy_invalid", legacyAfter.AuthType)
	require.Equal(t, legacy.AccessKeyIDEnc, legacyAfter.AccessKeyIDEnc)
	require.Equal(t, legacy.SecretAccessKeyEnc, legacyAfter.SecretAccessKeyEnc)
	require.Nil(t, legacyAfter.ServiceAccountJSONEnc)
	provider, ak, sk := "aws", "ak", "sk"
	_, err = svc.Update(t.Context(), 0, "", "", key.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak, SecretAccessKey: &sk})
	requireDatabaseError(t, err)
	after, err = svc.Repo().Get(t.Context(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "gcp", after.Provider)
	require.Equal(t, before.ServiceAccountJSONEnc, after.ServiceAccountJSONEnc)
}

func TestAuthTypeUpdatesRejectMixedAndIncompleteSecrets(t *testing.T) {
	svc, _, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	aws, err := svc.Create(t.Context(), 0, "", "", newReq())
	require.NoError(t, err)
	gcp, err := svc.Create(t.Context(), 0, "", "", gcpRequest(t))
	require.NoError(t, err)
	provider, legacyType, wrongType, jsonSecret, ak, empty := "aliyun", "legacy_invalid", "service_account_json", string(serviceAccountFixture(t)), "ak", ""
	for _, tc := range []struct {
		name string
		id   uint64
		req  cloudkey.UpdateRequest
	}{
		{"aws json", aws.ID, cloudkey.UpdateRequest{ServiceAccountJSON: &jsonSecret}},
		{"aws wrong auth", aws.ID, cloudkey.UpdateRequest{AuthType: &wrongType}},
		{"new legacy", gcp.ID, cloudkey.UpdateRequest{AuthType: &legacyType}},
		{"gcp keys", gcp.ID, cloudkey.UpdateRequest{AccessKeyID: &ak}},
		{"gcp empty keys", gcp.ID, cloudkey.UpdateRequest{AccessKeyID: &empty}},
		{"gcp mixed", gcp.ID, cloudkey.UpdateRequest{AccessKeyID: &ak, ServiceAccountJSON: &jsonSecret}},
		{"conversion needs full keys", aws.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Update(t.Context(), 0, "", "", tc.id, tc.req)
			requireValidation(t, err)
		})
	}
}

func TestConcurrentBindingPreventsAuthTypeConversion(t *testing.T) {
	svc, db, cleanup := newSvcWithDB(t)
	t.Cleanup(cleanup)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	key, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, advisorylock.LockCloudKey(ctx, tx, key.ID))
	require.NoError(t, tx.Create(&models.CloudAccount{Name: "racing", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}).Error)
	done := make(chan error, 1)
	provider, ak, sk := "aliyun", "ak", "sk"
	go func() {
		_, err := svc.Update(ctx, 0, "", "", key.ID, cloudkey.UpdateRequest{Provider: &provider, AccessKeyID: &ak, SecretAccessKey: &sk})
		done <- err
	}()
	require.Eventually(t, func() bool {
		var waiting int64
		err := db.WithContext(ctx).Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit().Error)
	be, ok := apperr.AsBiz(<-done)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
}

func requireValidation(t *testing.T, err error) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected validation error, got %v", err)
	require.Equal(t, apperr.CodeValidation, be.Code)
	require.Equal(t, "common.validation", be.MessageKey)
}

func requireDatabaseError(t *testing.T, err error) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected safe database error, got %v", err)
	require.Equal(t, apperr.CodeDBError, be.Code)
	require.Equal(t, "database operation failed", be.Message)
}
