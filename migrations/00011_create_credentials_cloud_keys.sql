-- +goose Up
CREATE TABLE credentials_cloud_keys (
    id                     BIGSERIAL    PRIMARY KEY,
    name                   VARCHAR(128) NOT NULL UNIQUE,
    description            TEXT         NOT NULL DEFAULT '',
    provider               VARCHAR(16)  NOT NULL CHECK (provider IN ('aliyun','aws','gcp')),
    region                 VARCHAR(32)  NOT NULL DEFAULT '',
    access_key_id_enc      BYTEA,
    secret_access_key_enc  BYTEA,
    auth_type              VARCHAR(32) NOT NULL DEFAULT 'access_key',
    service_account_json_enc BYTEA,
    service_account_email  TEXT NOT NULL DEFAULT '',
    credential_project_id  TEXT NOT NULL DEFAULT '',
    created_by_user_id     BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_cloud_key_auth_shape CHECK (
    (provider IN ('aws', 'aliyun') AND auth_type = 'access_key'
        AND access_key_id_enc IS NOT NULL AND secret_access_key_enc IS NOT NULL
        AND service_account_json_enc IS NULL)
    OR (provider = 'gcp' AND auth_type = 'service_account_json'
        AND access_key_id_enc IS NULL AND secret_access_key_enc IS NULL
        AND service_account_json_enc IS NOT NULL)
    OR (provider = 'gcp' AND auth_type = 'legacy_invalid'
        AND access_key_id_enc IS NOT NULL AND secret_access_key_enc IS NOT NULL
        AND service_account_json_enc IS NULL)
)
);
CREATE INDEX idx_credentials_cloud_keys_provider   ON credentials_cloud_keys(provider);
CREATE INDEX idx_credentials_cloud_keys_created_by ON credentials_cloud_keys(created_by_user_id);

-- +goose Down
DROP TABLE credentials_cloud_keys;
