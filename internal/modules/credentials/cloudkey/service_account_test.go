package cloudkey_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
)

func serviceAccountFields(t *testing.T) map[string]string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return map[string]string{
		"type": "service_account", "project_id": "test-project",
		"client_email": "collector@test-project.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
}

func serviceAccountFixture(t *testing.T) []byte {
	t.Helper()
	return marshalServiceAccount(t, serviceAccountFields(t))
}

func marshalServiceAccount(t *testing.T, fields map[string]string) []byte {
	t.Helper()
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	return data
}

func TestServiceAccountCanonicalDownloadedKey(t *testing.T) {
	fields := serviceAccountFields(t)
	fields["private_key_id"] = "key-id"
	fields["client_id"] = "123456"
	fields["universe_domain"] = "googleapis.com"
	for _, field := range []string{"auth_uri", "auth_provider_x509_cert_url", "client_x509_cert_url"} {
		fields[field] = "https://untrusted.invalid/never-fetch"
	}
	canonical, metadata, err := cloudkey.ValidateServiceAccountJSON(marshalServiceAccount(t, fields))
	require.NoError(t, err)
	require.Equal(t, fields["client_email"], metadata.Email)
	require.Equal(t, fields["project_id"], metadata.ProjectID)
	var got map[string]string
	require.NoError(t, json.Unmarshal(canonical, &got))
	require.Len(t, got, 8)
	require.NotContains(t, string(canonical), "untrusted.invalid")
	delete(fields, "auth_uri")
	delete(fields, "auth_provider_x509_cert_url")
	delete(fields, "client_x509_cert_url")
	require.Equal(t, fields, got)
}

func TestServiceAccountOptionalAbsenceAndPKCS1(t *testing.T) {
	var fields map[string]string
	require.NoError(t, json.Unmarshal(serviceAccountFixture(t), &fields))
	delete(fields, "token_uri")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	fields["private_key"] = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	canonical, _, err := cloudkey.ValidateServiceAccountJSON(marshalServiceAccount(t, fields))
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(canonical, &got))
	require.Equal(t, fields, got)
}

func TestServiceAccountRejectsUnsafeOrInvalidInput(t *testing.T) {
	base := serviceAccountFields(t)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	for _, tc := range []struct{ name, field, value string }{
		{"external account", "type", "external_account"},
		{"authorized user", "type", "authorized_user"},
		{"no project", "project_id", " "},
		{"bad email", "client_email", "not-an-email"},
		{"display name email", "client_email", "Agent <agent@example.com>"},
		{"malformed RSA", "private_key", "DO-NOT-ECHO-THIS-SECRET"},
		{"non RSA", "private_key", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}))},
		{"hostile token", "token_uri", "http://127.0.0.1/DO-NOT-ECHO-THIS-SECRET"},
		{"empty token", "token_uri", ""},
		{"token suffix", "token_uri", "https://oauth2.googleapis.com/token?x=1"},
		{"hostile universe", "universe_domain", "attacker.invalid"},
		{"executable source", "credential_source", "DO-NOT-ECHO-THIS-SECRET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := make(map[string]string, len(base)+1)
			for k, v := range base {
				fields[k] = v
			}
			fields[tc.field] = tc.value
			assertServiceAccountInvalid(t, marshalServiceAccount(t, fields))
		})
	}
	valid := string(marshalServiceAccount(t, base))
	for name, data := range map[string]string{
		"duplicate":                  strings.TrimSuffix(valid, "}") + `,"type":"service_account"}`,
		"discarded duplicate":        strings.TrimSuffix(valid, "}") + `,"auth_uri":"a","auth_uri":"b"}`,
		"nested discarded duplicate": strings.TrimSuffix(valid, "}") + `,"auth_uri":{"a":1,"a":2}}`,
		"trailing document":          valid + ` {}`,
		"trailing junk":              valid + ` DO-NOT-ECHO-THIS-SECRET`,
		"too large":                  valid + strings.Repeat(" ", 64*1024),
		"array":                      `[]`,
		"null":                       `null`,
		"non string":                 strings.TrimSuffix(valid, "}") + `,"client_id":1}`,
	} {
		t.Run(name, func(t *testing.T) { assertServiceAccountInvalid(t, []byte(data)) })
	}
}

func assertServiceAccountInvalid(t *testing.T, data []byte) {
	t.Helper()
	canonical, _, err := cloudkey.ValidateServiceAccountJSON(data)
	require.Nil(t, canonical)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected safe validation error, got %v", err)
	require.Equal(t, apperr.CodeValidation, be.Code)
	require.Equal(t, "common.validation", be.MessageKey)
	require.NotContains(t, err.Error(), "DO-NOT-ECHO-THIS-SECRET")
	require.NotContains(t, err.Error(), "PRIVATE KEY")
}
