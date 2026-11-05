package aliyun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/stretchr/testify/require"
)

func TestOfficialSDKErrorsUseNeutralCodesAndNeverRetainSecrets(t *testing.T) {
	require.Equal(t, 500, apperr.HTTPStatus(43201), "cloud authentication errors must not trigger JWT refresh")
	for _, tc := range []struct {
		code   string
		status int
		want   apperr.Code
	}{
		{"InvalidAccessKeyId.NotFound", 400, 43201}, {"SignatureDoesNotMatch", 403, 43201}, {"Forbidden.RAM", 403, 43202}, {"Throttling.User", 400, 43204}, {"ServiceUnavailable", 503, 43203}, {"InvalidParameter", 400, 43205}, {"ServiceNotEnabled", 403, 43207}, {"Unknown", 500, 43208}, {"Unknown", 401, 43201}, {"Unknown", 403, 43202}, {"Unknown", 429, 43204},
	} {
		t.Run(tc.code+fmt.Sprint(tc.status), func(t *testing.T) {
			p := New(Options{Transport: databaseFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("x-acs-action") != "DescribeDBInstances" {
					return false
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"Code":%q,"Message":"synthetic-secret signed URL token"}`, tc.code)
				return true
			})})
			_, err := p.Fetch(context.Background(), fixtureUnit("database"), fixtureKey())
			biz, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, tc.want, biz.Code)
			require.NotContains(t, err.Error(), "synthetic")
			require.Nil(t, biz.Cause)
		})
	}
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		require.ErrorIs(t, safeError(fmt.Errorf("wrapped synthetic-secret: %w", sentinel)), sentinel)
	}
	raw := dara.NewSDKError(map[string]interface{}{"code": "Forbidden", "statusCode": 403, "message": "synthetic-secret"})
	require.NotContains(t, safeError(errors.Join(errors.New("token"), raw)).Error(), "token")
	require.False(t, strings.Contains(safeError(raw).Error(), "synthetic"))
}
