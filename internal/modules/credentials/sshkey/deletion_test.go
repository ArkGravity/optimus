//go:build dbtest

package sshkey_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/credentials/sshkey"
)

func dockerHostFor(name string, keyID uint64) *models.DockerHost {
	return &models.DockerHost{Name: name, Host: "10.0.0.9", Port: 22, SocketPath: "/var/run/docker.sock", AuthType: "ssh_key", SSHKeyID: &keyID, Username: "ops"}
}

func TestDeleteRefusedByLiveDockerHostAndDetachesHistory(t *testing.T) {
	svc, _, td := newSvc(t)
	t.Cleanup(td)
	ctx := t.Context()
	key, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "docker-key", Username: "ops", PrivateKey: genTestSSHKey(t)})
	require.NoError(t, err)
	db := svc.Repo().DB()
	historical := dockerHostFor("old", key.ID)
	live := dockerHostFor("live", key.ID)
	require.NoError(t, db.Create(historical).Error)
	require.NoError(t, db.Create(live).Error)
	require.NoError(t, db.Delete(historical).Error)

	err = svc.Delete(ctx, 0, "127.0.0.1", "ua", key.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDockerHostInUse, be.Code)
	require.Equal(t, "credentials.ssh_key.in_use_by_docker_hosts", be.MessageKey)
	require.Equal(t, 409, apperr.HTTPStatus(be.Code))
	_, err = svc.Get(ctx, key.ID)
	require.NoError(t, err)
	var kept models.DockerHost
	require.NoError(t, db.Unscoped().First(&kept, historical.ID).Error)
	require.Equal(t, key.ID, *kept.SSHKeyID, "refusal must not detach history")

	require.NoError(t, db.Delete(live).Error)
	require.NoError(t, svc.Delete(ctx, 0, "127.0.0.1", "ua", key.ID))
	var remaining int64
	require.NoError(t, db.Model(&models.CredentialSSHKey{}).Where("id = ?", key.ID).Count(&remaining).Error)
	require.Zero(t, remaining)
	var history []models.DockerHost
	require.NoError(t, db.Unscoped().Where("id IN ?", []uint64{historical.ID, live.ID}).Find(&history).Error)
	require.Len(t, history, 2)
	for _, row := range history {
		require.True(t, row.DeletedAt.Valid)
		require.Nil(t, row.SSHKeyID)
	}
	require.Error(t, db.Unscoped().Model(&models.DockerHost{}).Where("id = ?", historical.ID).Update("deleted_at", nil).Error,
		"a detached host must not become live without its key")
	var events int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ? AND target_type = ? AND target_id = ?", "credentials.delete", "credentials.ssh_key", strconv.FormatUint(key.ID, 10)).Count(&events).Error)
	require.EqualValues(t, 1, events)
}

func TestDeleteRollsBackDockerDetachmentWhenAuditFails(t *testing.T) {
	svc, _, td := newSvc(t)
	t.Cleanup(td)
	ctx := t.Context()
	key, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "rollback", Username: "ops", PrivateKey: genTestSSHKey(t)})
	require.NoError(t, err)
	db := svc.Repo().DB()
	historical := dockerHostFor("old", key.ID)
	require.NoError(t, db.Create(historical).Error)
	require.NoError(t, db.Delete(historical).Error)
	require.NoError(t, db.Exec(`CREATE FUNCTION reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_audit()`).Error)

	err = svc.Delete(ctx, 0, "", "", key.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDBError, be.Code)
	require.Equal(t, "database operation failed", be.Message)
	_, err = svc.Get(ctx, key.ID)
	require.NoError(t, err)
	var kept models.DockerHost
	require.NoError(t, db.Unscoped().First(&kept, historical.ID).Error)
	require.Equal(t, key.ID, *kept.SSHKeyID)
}
