// Package inuse is the one-way dependency surface from credentials/sshkey
// into Docker hosts. It lives in its own tiny package so the credentials
// module can import it without pulling in the Docker host service.
package inuse

import (
	"context"

	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/models"
)

// CountBySSHKeyID returns the number of live (non-soft-deleted) Docker hosts
// that reference the given SSH key.
func CountBySSHKeyID(ctx context.Context, db *gorm.DB, sshKeyID uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).
		Model(&models.DockerHost{}).
		Where("ssh_key_id = ?", sshKeyID).
		Count(&n).Error
	return n, err
}

// DetachDeletedBySSHKeyID clears only historical references. Call after
// locking the credential and checking live references in the same transaction.
func DetachDeletedBySSHKeyID(ctx context.Context, tx *gorm.DB, sshKeyID uint64) error {
	return tx.WithContext(ctx).Unscoped().Model(&models.DockerHost{}).
		Where("ssh_key_id = ? AND deleted_at IS NOT NULL", sshKeyID).
		Update("ssh_key_id", nil).Error
}
