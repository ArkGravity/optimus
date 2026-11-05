-- +goose Up
CREATE TABLE cloud_accounts (
  id              BIGSERIAL PRIMARY KEY,
  name            VARCHAR(128) NOT NULL,
  provider        VARCHAR(16)  NOT NULL,
  cloudkey_id     BIGINT       REFERENCES credentials_cloud_keys(id),
  enabled_regions TEXT[]       NOT NULL,
  enabled         BOOLEAN      NOT NULL DEFAULT true,
  description     TEXT         NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at      TIMESTAMPTZ,
  project_id TEXT NOT NULL DEFAULT '',
  external_account_id TEXT NOT NULL DEFAULT '',
  scope_revision BIGINT NOT NULL DEFAULT 1,
  CONSTRAINT cloud_accounts_live_cloudkey_required CHECK (deleted_at IS NOT NULL OR cloudkey_id IS NOT NULL),
  CONSTRAINT ck_cloud_account_target CHECK (
      (provider = 'gcp' AND project_id <> '') OR
      (provider IN ('aws', 'aliyun') AND project_id = '')
  ),
  CONSTRAINT ck_cloud_account_scope_revision CHECK (scope_revision > 0)
);
CREATE UNIQUE INDEX uq_cloud_accounts_name_alive
    ON cloud_accounts(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_cloud_accounts_cloudkey ON cloud_accounts(cloudkey_id);

-- +goose Down
DROP TABLE IF EXISTS cloud_accounts;
