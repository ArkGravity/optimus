-- +goose Up
ALTER TABLE cloud_accounts ALTER COLUMN cloudkey_id DROP NOT NULL;
ALTER TABLE cloud_accounts ADD CONSTRAINT cloud_accounts_live_cloudkey_required
    CHECK (deleted_at IS NOT NULL OR cloudkey_id IS NOT NULL);

-- +goose Down
-- A downgrade cannot reconstruct deleted credentials. Refuse it if historical
-- references have already been detached; Goose rolls back the whole migration.
ALTER TABLE cloud_accounts ALTER COLUMN cloudkey_id SET NOT NULL;
ALTER TABLE cloud_accounts DROP CONSTRAINT cloud_accounts_live_cloudkey_required;
