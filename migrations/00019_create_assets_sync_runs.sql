-- +goose Up
CREATE TABLE assets_sync_runs (
  id                   BIGSERIAL PRIMARY KEY,
  cloud_account_id     BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region               VARCHAR(32)  NOT NULL,
  resource_type        VARCHAR(16)  NOT NULL,
  started_at           TIMESTAMPTZ  NOT NULL,
  finished_at          TIMESTAMPTZ,
  status               VARCHAR(16)  NOT NULL,
  items_seen           INTEGER      NOT NULL DEFAULT 0,
  items_softdeleted    INTEGER      NOT NULL DEFAULT 0,
  error                TEXT         NOT NULL DEFAULT '',
  error_code           INTEGER      NOT NULL DEFAULT 0,
  trigger              VARCHAR(16)  NOT NULL,
  triggered_by_user_id BIGINT       REFERENCES users(id),
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
  provider VARCHAR(16) NOT NULL,
  location_type VARCHAR(16) NOT NULL,
  location TEXT NOT NULL,
  scope_revision BIGINT NOT NULL,
  CONSTRAINT ck_assets_sync_run_provider CHECK (provider IN ('aws', 'aliyun', 'gcp')),
  CONSTRAINT ck_assets_sync_run_location_type CHECK (location_type IN ('global', 'region', 'zone')),
  CONSTRAINT ck_assets_sync_run_scope_revision CHECK (scope_revision > 0),
  CONSTRAINT ck_assets_sync_run_resource_type CHECK (resource_type IN
      ('instance', 'network', 'database', 'security_group', 'subnet', 'firewall', 'planning'))
);
CREATE INDEX idx_assets_sync_runs_account_type
    ON assets_sync_runs(cloud_account_id, resource_type, started_at DESC);
CREATE INDEX idx_assets_sync_runs_status
    ON assets_sync_runs(status, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS assets_sync_runs;
