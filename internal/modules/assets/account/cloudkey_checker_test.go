package account_test

import (
	"github.com/ArkGravity/optimus/internal/modules/assets/account"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
)

var _ account.CloudKeyConsumer = (credentials.Consumer)(nil)
