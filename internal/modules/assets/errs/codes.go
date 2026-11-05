package errs

import infraerrors "github.com/ArkGravity/optimus/internal/infra/errors"

const (
	CodeAssetsUnauthorized             = infraerrors.CodeAssetsUnauthorized
	CodeAssetsForbidden                = infraerrors.CodeAssetsForbidden
	CodeAssetsUnreachable              = infraerrors.CodeAssetsUnreachable
	CodeAssetsThrottled                = infraerrors.CodeAssetsThrottled
	CodeAssetsAPIDisabled              = infraerrors.CodeAssetsAPIDisabled
	CodeAssetsConfig                   = infraerrors.CodeAssetsConfig
	CodeAssetsPartial                  = infraerrors.CodeAssetsPartial
	CodeAssetsOther                    = infraerrors.CodeAssetsOther
	CodeAssetsIdentityMismatch         = infraerrors.CodeAssetsIdentityMismatch
	CodeAssetsCloudAccountInUse        = infraerrors.CodeAssetsCloudAccountInUse
	CodeAssetsCloudAccountNotFound     = infraerrors.CodeAssetsCloudAccountNotFound
	CodeAssetsCloudAccountNameConflict = infraerrors.CodeAssetsCloudAccountNameConflict
	CodeAssetsRegionInvalid            = infraerrors.CodeAssetsRegionInvalid
	CodeAssetsProviderUnsupported      = infraerrors.CodeAssetsProviderUnsupported
	CodeAssetsCloudAccountDisabled     = infraerrors.CodeAssetsCloudAccountDisabled
	CodeAssetsCloudKeyNotFound         = infraerrors.CodeAssetsCloudKeyNotFound
	CodeAssetsVPCNotFound              = infraerrors.CodeAssetsVPCNotFound
	CodeAssetsSyncBusy                 = infraerrors.CodeAssetsSyncBusy
	CodeAssetsAWSUnauthorized          = infraerrors.CodeAssetsAWSUnauthorized
	CodeAssetsAWSForbidden             = infraerrors.CodeAssetsAWSForbidden
	CodeAssetsAWSUnreachable           = infraerrors.CodeAssetsAWSUnreachable
	CodeAssetsAWSThrottled             = infraerrors.CodeAssetsAWSThrottled
	CodeAssetsAWSOther                 = infraerrors.CodeAssetsAWSOther
	CodeAssetsAWSConfig                = infraerrors.CodeAssetsAWSConfig
)

const (
	KeyUnauthorized             = "assets.provider.unauthorized"
	KeyForbidden                = "assets.provider.forbidden"
	KeyUnreachable              = "assets.provider.unreachable"
	KeyThrottled                = "assets.provider.throttled"
	KeyAPIDisabled              = "assets.provider.api_disabled"
	KeyConfig                   = "assets.provider.config"
	KeyPartial                  = "assets.provider.partial"
	KeyOther                    = "assets.provider.other"
	KeyIdentityMismatch         = "assets.provider.identity_mismatch"
	KeyCloudAccountInUse        = "assets.cloud_account.in_use"
	KeyCloudAccountNotFound     = "assets.cloud_account.not_found"
	KeyCloudAccountNameConflict = "assets.cloud_account.name_conflict"
	KeyRegionInvalid            = "assets.region.invalid"
	KeyProviderUnsupported      = "assets.provider.unsupported"
	KeyCloudAccountDisabled     = "assets.cloud_account.disabled"
	KeyCloudKeyNotFound         = "assets.cloudkey.not_found"
	KeyVPCNotFound              = "assets.vpc.not_found"
	KeySyncBusy                 = "assets.sync.busy"
	KeyAWSUnauthorized          = "assets.aws.unauthorized"
	KeyAWSForbidden             = "assets.aws.forbidden"
	KeyAWSUnreachable           = "assets.aws.unreachable"
	KeyAWSThrottled             = "assets.aws.throttled"
	KeyAWSOther                 = "assets.aws.other"
	KeyAWSConfig                = "assets.aws.config"
)

// ProviderError returns fixed public text, discarding SDK and caller error payloads.
func ProviderError(code infraerrors.Code) *infraerrors.BizError {
	switch code {
	case CodeAssetsUnauthorized:
		return infraerrors.New(code, KeyUnauthorized, "cloud credentials invalid or expired")
	case CodeAssetsForbidden:
		return infraerrors.New(code, KeyForbidden, "cloud API access denied")
	case CodeAssetsUnreachable:
		return infraerrors.New(code, KeyUnreachable, "cloud API unreachable")
	case CodeAssetsThrottled:
		return infraerrors.New(code, KeyThrottled, "cloud API request throttled")
	case CodeAssetsAPIDisabled:
		return infraerrors.New(code, KeyAPIDisabled, "cloud API is not enabled")
	case CodeAssetsConfig:
		return infraerrors.New(code, KeyConfig, "invalid asset collection configuration")
	case CodeAssetsPartial:
		return infraerrors.New(code, KeyPartial, "asset snapshot is incomplete")
	case CodeAssetsIdentityMismatch:
		return infraerrors.New(code, KeyIdentityMismatch, "verified provider identity does not match this cloud account")
	default:
		return infraerrors.New(CodeAssetsOther, KeyOther, "asset collection failed")
	}
}
