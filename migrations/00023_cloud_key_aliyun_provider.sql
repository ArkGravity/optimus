-- +goose Up
ALTER TABLE credentials_cloud_keys DROP CONSTRAINT credentials_cloud_keys_provider_check;
-- Keep existing Azure credentials readable and editable without changing their
-- identity. API validation permits only Aliyun, AWS and GCP for new bindings.
ALTER TABLE credentials_cloud_keys ADD CONSTRAINT credentials_cloud_keys_provider_check
    CHECK (provider IN ('aliyun', 'aws', 'gcp', 'azure'));

-- +goose Down
-- Refuse a downgrade while Aliyun credentials exist rather than relabel or
-- delete them. Goose rolls back both statements if validation fails.
ALTER TABLE credentials_cloud_keys DROP CONSTRAINT credentials_cloud_keys_provider_check;
ALTER TABLE credentials_cloud_keys ADD CONSTRAINT credentials_cloud_keys_provider_check
    CHECK (provider IN ('aws', 'gcp', 'azure'));
