package gcp

import (
	"context"
	"errors"
	"net"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func configError() *apperr.BizError { return errs.ProviderError(errs.CodeAssetsConfig) }

// SDK errors may contain private material or API bodies. Keep only fixed text.
func safeError(err error) error {
	code := errs.CodeAssetsOther
	if biz, ok := apperr.AsBiz(err); ok {
		code = biz.Code
	} else {
		var api *googleapi.Error
		var token *oauth2.RetrieveError
		var network net.Error
		switch {
		case errors.As(err, &token):
			code = errs.CodeAssetsUnauthorized
			if token.Response != nil && token.Response.StatusCode == 429 {
				code = errs.CodeAssetsThrottled
			}
			if token.Response != nil && token.Response.StatusCode >= 500 {
				code = errs.CodeAssetsUnreachable
			}
		case errors.As(err, &api):
			switch api.Code {
			case 401:
				code = errs.CodeAssetsUnauthorized
			case 403:
				code = errs.CodeAssetsForbidden
			case 429:
				code = errs.CodeAssetsThrottled
			case 400, 404:
				code = errs.CodeAssetsConfig
			case 500, 502, 503, 504:
				code = errs.CodeAssetsUnreachable
			}
			for _, detail := range api.Errors {
				switch detail.Reason {
				case "accessNotConfigured", "serviceDisabled":
					code = errs.CodeAssetsAPIDisabled
				case "rateLimitExceeded", "userRateLimitExceeded", "quotaExceeded":
					code = errs.CodeAssetsThrottled
				}
			}
			for _, detail := range api.Details {
				if info, ok := detail.(map[string]interface{}); ok && info["@type"] == "type.googleapis.com/google.rpc.ErrorInfo" {
					switch info["reason"] {
					case "SERVICE_DISABLED":
						code = errs.CodeAssetsAPIDisabled
					case "RATE_LIMIT_EXCEEDED", "QUOTA_EXCEEDED":
						code = errs.CodeAssetsThrottled
					}
				}
			}
		case errors.As(err, &network):
			code = errs.CodeAssetsUnreachable
		}
		if errors.Is(err, context.DeadlineExceeded) {
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
func requestError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return safeError(ctx.Err())
	}
	return safeError(err)
}
