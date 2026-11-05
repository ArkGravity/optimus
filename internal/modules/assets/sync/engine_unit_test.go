package sync

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/errs"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

type recordingConsumer struct {
	key      *credentials.CloudKey
	purposes []string
}

func (c *recordingConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	c.purposes = append(c.purposes, purpose)
	return c.key, nil
}

type recordingProvider struct{ accessKey, secret string }

func (*recordingProvider) Name() string { return "aws" }
func (*recordingProvider) Plan(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error) {
	return Plan{}, nil
}
func (p *recordingProvider) Fetch(_ context.Context, _ Unit, key *credentials.CloudKey) (Snapshot, error) {
	p.accessKey, p.secret = key.AccessKeyID, key.SecretAccessKey
	return Snapshot{}, nil
}

func TestProviderFetchUsesDetailedPurposeAndWipesKey(t *testing.T) {
	tests := []struct {
		trigger      string
		systemCaller bool
		prefix       string
	}{
		{trigger: "cron", systemCaller: true, prefix: "system:assets.sync.cron."},
		{trigger: "manual", systemCaller: false, prefix: "assets.sync.manual."},
	}
	for _, tt := range tests {
		t.Run(tt.trigger, func(t *testing.T) {
			key := &credentials.CloudKey{Provider: "aws", AuthType: "access_key", AccessKeyID: "AKIA-sensitive", SecretAccessKey: "secret-sensitive"}
			consumer := &recordingConsumer{key: key}
			provider := &recordingProvider{}
			engine := NewProviderEngine(nil, nil, nil, consumer, time.Second, time.Minute)

			_, err := engine.providerFetch(context.Background(), provider, Unit{Key: "instance/us-east-1", Scope: Scope{AccountID: 42, CloudKeyID: 7, Provider: "aws"}}, tt.trigger, tt.systemCaller)
			require.NoError(t, err)
			require.Equal(t, "AKIA-sensitive", provider.accessKey)
			require.Equal(t, "secret-sensitive", provider.secret)
			require.Len(t, consumer.purposes, 1)
			require.True(t, strings.HasPrefix(consumer.purposes[0], tt.prefix), consumer.purposes[0])
			require.Contains(t, consumer.purposes[0], "account-42.aws.instance/us-east-1")
			require.Empty(t, key.Provider)
			require.Empty(t, key.AccessKeyID)
			require.Empty(t, key.SecretAccessKey)
		})
	}
}

func TestSaturatingCountsNeverWrap(t *testing.T) {
	require.Equal(t, int32(0), saturatingInt32(-1))
	require.Equal(t, int32(math.MaxInt32), saturatingInt32(math.MaxInt64))
	require.Equal(t, int64(math.MaxInt64), saturatingAdd(math.MaxInt64-2, 10))
}

func TestSafeSyncRunErrorPreservesWhitelistedCodeWithoutCauseText(t *testing.T) {
	cause := apperr.Wrap(errors.New("secret-config-cause"), errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "unsafe caller message")
	code, message := safeSyncRunError(cause)
	require.Equal(t, errs.CodeAssetsAWSConfig, code)
	require.Equal(t, "AWS SDK configuration failed", message)
	require.NotContains(t, message, "secret-config-cause")
	require.NotContains(t, message, "unsafe caller message")
}

func TestEngineTryLockGatesConcurrentRuns(t *testing.T) {
	engine := &Engine{}
	require.True(t, engine.TryLock(9))
	require.False(t, engine.TryLock(9))
	engine.Unlock(9)
	require.True(t, engine.TryLock(9))
}

func TestSafeProviderErrorUsesFixedMessagesAndKeys(t *testing.T) {
	for _, test := range []struct {
		code apperr.Code
		key  string
	}{
		{errs.CodeAssetsConfig, errs.KeyConfig},
		{errs.CodeAssetsPartial, errs.KeyPartial},
		{errs.CodeAssetsOther, errs.KeyOther},
		{errs.CodeAssetsIdentityMismatch, errs.KeyIdentityMismatch},
		{errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden},
	} {
		cause := apperr.Wrap(errors.New("private-json-token"), test.code, "secret-key", "secret-message")
		safe := safeProviderError(cause)
		require.Equal(t, test.code, safe.Code)
		require.Equal(t, test.key, safe.MessageKey)
		require.NotContains(t, safe.Error(), "secret")
		require.NotContains(t, safe.Error(), "private-json-token")
	}
}

func TestNeutralProviderErrorsRetainCodesWithFixedSafeRunMessages(t *testing.T) {
	for _, code := range []apperr.Code{43201, 43202, 43203, 43204, 43207} {
		original := apperr.Wrap(errors.New("signed URL"), code, "untrusted", "synthetic-secret token")
		safe := safeProviderError(original)
		require.Equal(t, code, safe.Code)
		require.NotContains(t, safe.Message, "synthetic")
		require.NotContains(t, safe.Message, "signed URL")
		require.Nil(t, safe.Cause)
	}
}
