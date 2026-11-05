package cloudkey_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
)

func TestAuthTypeProviderMatrix(t *testing.T) {
	for _, provider := range []string{"aws", "aliyun", "gcp", "azure", ""} {
		for _, auth := range []string{"", "access_key", "service_account_json", "legacy_invalid", "external_account"} {
			t.Run(provider+"/"+auth, func(t *testing.T) {
				want := "access_key"
				if provider == "gcp" {
					want = "service_account_json"
				}
				valid := (provider == "aws" || provider == "aliyun" || provider == "gcp") && (auth == "" || auth == want)
				got, err := cloudkey.ResolveAuthType(provider, auth)
				if valid {
					require.NoError(t, err)
					require.Equal(t, want, got)
				} else {
					be, ok := apperr.AsBiz(err)
					require.True(t, ok)
					require.Equal(t, "common.validation", be.MessageKey)
				}
			})
		}
	}
}
