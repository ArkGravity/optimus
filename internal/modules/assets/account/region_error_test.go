package account

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegionCatalogErrorsPreserveSafeCategoriesAndCancellation(t *testing.T) {
	for _, code := range []apperr.Code{errs.CodeAssetsUnauthorized, errs.CodeAssetsForbidden, errs.CodeAssetsUnreachable, errs.CodeAssetsThrottled, errs.CodeAssetsConfig, errs.CodeAssetsPartial, errs.CodeAssetsAPIDisabled, errs.CodeAssetsOther, errs.CodeAssetsIdentityMismatch, errs.CodeAssetsAWSUnauthorized, errs.CodeAssetsAWSForbidden, errs.CodeAssetsAWSUnreachable, errs.CodeAssetsAWSThrottled, errs.CodeAssetsAWSConfig, errs.CodeAssetsAWSOther} {
		original := apperr.Wrap(errors.New(`{"private_key":"SA-JSON"} signed-JWT AK-ID SK-value raw-provider-error`), code, "private-key", "raw-provider-error")
		safe := safeRegionCatalogError(original)
		biz, ok := apperr.AsBiz(safe)
		require.True(t, ok)
		require.Equal(t, code, biz.Code)
		require.NotContains(t, safe.Error(), "raw-provider-error")
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		response.Error(c, safe)
		require.NotEqual(t, 401, rec.Code, "provider auth failure must not initiate browser JWT refresh")
		for _, secret := range strings.Fields("SA-JSON signed-JWT AK-ID SK-value raw-provider-error") {
			require.NotContains(t, rec.Body.String(), secret)
		}
	}
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		err := safeRegionCatalogError(errors.Join(errors.New("raw-provider-error"), sentinel))
		require.ErrorIs(t, err, sentinel)
		require.NotContains(t, err.Error(), "raw-provider-error")
	}
}
