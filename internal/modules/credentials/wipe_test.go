package credentials

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWipeCloudKey(t *testing.T) {
	require.NotPanics(t, func() { Wipe(nil) })
	key := &CloudKey{
		Name: "production", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIATEST", SecretAccessKey: "secret",
	}
	Wipe(key)
	require.Empty(t, key.Provider)
	require.Empty(t, key.AccessKeyID)
	require.Empty(t, key.SecretAccessKey)
	require.Equal(t, "production", key.Name)
	require.Equal(t, "us-east-1", key.Region)
}

func TestWipeServiceAccountOriginalSlice(t *testing.T) {
	original := []byte("private service account JSON")
	key := &CloudKey{Provider: "gcp", AuthType: "service_account_json", ServiceAccountJSON: original}
	Wipe(key)
	require.Equal(t, make([]byte, len(original)), original)
	require.Nil(t, key.ServiceAccountJSON)
	require.Empty(t, key.AuthType)
	Wipe(key)
}

func TestValidateServiceAccountJSONRejectsExternalSource(t *testing.T) {
	_, err := ValidateServiceAccountJSON([]byte(`{"type":"external_account","credential_source":{"url":"http://metadata"}}`))
	if err == nil {
		t.Fatal("unsafe credential source accepted")
	}
}
