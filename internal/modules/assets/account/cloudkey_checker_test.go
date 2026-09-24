package account_test

import (
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/credentials/cloudkey"
)

var _ account.CloudKeyExistenceChecker = (*cloudkey.Service)(nil)
