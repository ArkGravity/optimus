package advisorylock

import (
	"context"

	"gorm.io/gorm"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
)

const applicationLockNamespace = "optimus:application-lifecycle:"

func applicationLockKey(id uint64) int64 {
	return lifecycleLockKey(applicationLockNamespace, id)
}

// LockApplication acquires the shared application lifecycle lock or reports a
// conflict. Contending requests must not occupy the connection pool while an
// upstream Helm operation needs connections for lookups and credential audits.
func LockApplication(ctx context.Context, tx *gorm.DB, id uint64) error {
	if tx == nil {
		return apperr.New(apperr.CodeInternal, "common.internal_error", "application lock requires a transaction")
	}
	var acquired bool
	if err := tx.WithContext(ctx).Raw("SELECT pg_try_advisory_xact_lock(?)", applicationLockKey(id)).Scan(&acquired).Error; err != nil {
		return err
	}
	if !acquired {
		return apperr.New(apperr.CodeConflict, "apps.application.busy", "another application operation is in progress")
	}
	return nil
}
