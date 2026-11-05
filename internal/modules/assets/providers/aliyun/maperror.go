package aliyun

import (
	"context"
	"errors"
	"net"
	"strings"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"
)

func configError() *apperr.BizError { return errs.ProviderError(errs.CodeAssetsConfig) }

// SDK errors can contain signed URLs, keys and API bodies. Retain only fixed text.
func safeError(err error) error {
	code := errs.CodeAssetsOther
	if biz, ok := apperr.AsBiz(err); ok {
		code = biz.Code
	} else {
		native, status := "", 0
		var response dara.ResponseError
		var sdk *dara.SDKError
		var legacy *tea.SDKError
		switch {
		case errors.As(err, &response):
			native = dara.StringValue(response.GetCode())
			status = dara.IntValue(response.GetStatusCode())
		case errors.As(err, &sdk):
			native = dara.StringValue(sdk.Code)
			status = dara.IntValue(sdk.StatusCode)
		case errors.As(err, &legacy):
			native = dara.StringValue(legacy.Code)
			status = dara.IntValue(legacy.StatusCode)
		}
		switch {
		case native == "InvalidAccessKeyId.NotFound" || native == "InvalidAccessKeyId" || native == "SignatureDoesNotMatch" || native == "InvalidSecurityToken.Expired" || native == "InvalidSecurityToken" || status == 401:
			code = errs.CodeAssetsUnauthorized
		case native == "ServiceNotEnabled" || native == "ServiceNotOpen" || native == "ApiNotEnabled":
			code = errs.CodeAssetsAPIDisabled
		case strings.HasPrefix(native, "Throttling") || status == 429:
			code = errs.CodeAssetsThrottled
		case strings.HasPrefix(native, "Forbidden") || native == "UnauthorizedOperation" || status == 403:
			code = errs.CodeAssetsForbidden
		case native == "ServiceUnavailable" || status == 502 || status == 503 || status == 504:
			code = errs.CodeAssetsUnreachable
		case strings.HasPrefix(native, "InvalidParameter") || strings.HasPrefix(native, "InvalidRegion"):
			code = errs.CodeAssetsConfig
		}
		var network net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) {
			code = errs.CodeAssetsUnreachable
		}
	}
	safe := errs.ProviderError(code)
	if errors.Is(err, context.Canceled) {
		return errors.Join(safe, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(safe, context.DeadlineExceeded)
	}
	return safe
}
