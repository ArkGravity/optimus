package account_test

import (
	"github.com/logic3579/optimus/internal/modules/assets/account"
	"github.com/logic3579/optimus/internal/modules/credentials/cloudkey"
)

var _ account.CloudKeyExistenceChecker = (*cloudkey.Service)(nil)
