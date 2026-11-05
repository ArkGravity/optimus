package gcp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func TestSafeErrorClassifiesStructuredFailuresAndDiscardsSecrets(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		code apperr.Code
	}{
		{"unauthorized", &googleapi.Error{Code: 401, Message: "private-key JWT access-token body"}, errs.CodeAssetsUnauthorized},
		{"forbidden", &googleapi.Error{Code: 403, Message: "secret"}, errs.CodeAssetsForbidden},
		{"disabled", &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "accessNotConfigured"}}}, errs.CodeAssetsAPIDisabled},
		{"disabled-error-info", &googleapi.Error{Code: 403, Details: []interface{}{map[string]interface{}{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "SERVICE_DISABLED"}}}, errs.CodeAssetsAPIDisabled},
		{"quota", &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "quotaExceeded"}}}, errs.CodeAssetsThrottled},
		{"429", &googleapi.Error{Code: 429}, errs.CodeAssetsThrottled},
		{"unavailable", &googleapi.Error{Code: 503}, errs.CodeAssetsUnreachable},
		{"oauth", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 400}, Body: []byte("secret JWT private-key")}, errs.CodeAssetsUnauthorized},
		{"oauth-403-is-authentication", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 403}, Body: []byte("secret JWT")}, errs.CodeAssetsUnauthorized},
		{"oauth-throttled", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 429}, Body: []byte("secret")}, errs.CodeAssetsThrottled},
		{"oauth-unreachable", &oauth2.RetrieveError{Response: &http.Response{StatusCode: 503}, Body: []byte("secret")}, errs.CodeAssetsUnreachable},
		{"disabled-service", &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "serviceDisabled"}}, Message: "secret"}, errs.CodeAssetsAPIDisabled},
		{"deadline", context.DeadlineExceeded, errs.CodeAssetsUnreachable},
		{"other", errors.New("secret"), errs.CodeAssetsOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := safeError(tt.err)
			biz, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, tt.code, biz.Code)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "private-key")
			if errors.Is(tt.err, context.DeadlineExceeded) {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
		})
	}
	require.ErrorIs(t, safeError(context.Canceled), context.Canceled)
}
